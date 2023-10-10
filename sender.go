package courier

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nyaruka/gocommon/analytics"
)

// Foreman takes care of managing our set of sending workers and assigns msgs for each to send
type Foreman struct {
	server           Server
	senders          []*Sender
	availableSenders chan *Sender
	quit             chan bool
}

// NewForeman creates a new Foreman for the passed in server with the number of max senders
func NewForeman(server Server, maxSenders int) *Foreman {
	foreman := &Foreman{
		server:           server,
		senders:          make([]*Sender, maxSenders),
		availableSenders: make(chan *Sender, maxSenders),
		quit:             make(chan bool),
	}

	for i := 0; i < maxSenders; i++ {
		foreman.senders[i] = NewSender(foreman, i)
	}

	return foreman
}

// Start starts the foreman and all its senders, assigning jobs while there are some
func (f *Foreman) Start() {
	for _, sender := range f.senders {
		sender.Start()
	}
	go f.Assign()
}

// Stop stops the foreman and all its senders, the wait group of the server can be used to track progress
func (f *Foreman) Stop() {
	for _, sender := range f.senders {
		sender.Stop()
	}
	close(f.quit)
	slog.Info("foreman stopping", "comp", "foreman", "state", "stopping")
}

// Assign is our main loop for the Foreman, it takes care of popping the next outgoing messages from our
// backend and assigning them to workers
func (f *Foreman) Assign() {
	f.server.WaitGroup().Add(1)
	defer f.server.WaitGroup().Done()
	log := slog.With("comp", "foreman")

	log.Info("senders started and waiting",
		"state", "started",
		"senders", len(f.senders))

	backend := f.server.Backend()
	lastSleep := false

	for {
		select {
		// return if we have been told to stop
		case <-f.quit:
			log.Info("foreman stopped", "state", "stopped")
			return

		// otherwise, grab the next msg and assign it to a sender
		case sender := <-f.availableSenders:
			// see if we have a message to work on
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
			msg, err := backend.PopNextOutgoingMsg(ctx)
			cancel()

			if err == nil && msg != nil {
				// if so, assign it to our sender
				sender.job <- msg
				lastSleep = false
			} else {
				// we received an error getting the next message, log it
				if err != nil {
					log.Error("error popping outgoing msg", "error", err)
				}

				// add our sender back to our queue and sleep a bit
				if !lastSleep {
					log.Debug("sleeping, no messages")
					lastSleep = true
				}
				f.availableSenders <- sender
				time.Sleep(250 * time.Millisecond)
			}
		}
	}
}

// Sender is our type for a single goroutine that is sending messages
type Sender struct {
	id      int
	foreman *Foreman
	job     chan MsgOut
}

// NewSender creates a new sender responsible for sending messages
func NewSender(foreman *Foreman, id int) *Sender {
	sender := &Sender{
		id:      id,
		foreman: foreman,
		job:     make(chan MsgOut, 1),
	}
	return sender
}

// Start starts our Sender's goroutine and has it start waiting for tasks from the foreman
func (w *Sender) Start() {
	w.foreman.server.WaitGroup().Add(1)

	go func() {
		defer w.foreman.server.WaitGroup().Done()
		slog.Debug("started", "comp", "sender", "sender_id", w.id)
		for {
			// list ourselves as available for work
			w.foreman.availableSenders <- w

			// grab our next piece of work
			msg := <-w.job

			// exit if we were stopped
			if msg == nil {
				slog.Debug("stopped")
				return
			}

			w.sendMessage(msg)
		}
	}()
}

// Stop stops our senders, callers can use the server's wait group to track progress
func (w *Sender) Stop() {
	close(w.job)
}

func (w *Sender) sendMessage(msg MsgOut) {

	log := slog.With("comp", "sender", "sender_id", w.id, "channel_uuid", msg.Channel().UUID())

	server := w.foreman.server
	backend := server.Backend()

	// we don't want any individual send taking more than 35s
	sendCTX, cancel := context.WithTimeout(context.Background(), time.Second*35)
	defer cancel()

	log = log.With("msg_id", msg.ID(), "msg_text", msg.Text(), "msg_urn", msg.URN().Identity())
	if len(msg.Attachments()) > 0 {
		log = log.With("attachments", msg.Attachments())
	}
	if len(msg.QuickReplies()) > 0 {
		log = log.With("quick_replies", msg.QuickReplies())
	}

	start := time.Now()

	// if this is a resend, clear our sent status
	if msg.IsResend() {
		err := backend.ClearMsgSent(sendCTX, msg.ID())
		if err != nil {
			log.Error("error clearing sent status for msg", "error", err)
		}
	}

	// was this msg already sent? (from a double queue?)
	sent, err := backend.WasMsgSent(sendCTX, msg.ID())

	// failing on a lookup isn't a halting problem but we should log it
	if err != nil {
		log.Error("error looking up msg was sent", "error", err)
	}

	var status StatusUpdate
	var redactValues []string
	handler := server.GetHandler(msg.Channel())
	if handler != nil {
		redactValues = handler.RedactValues(msg.Channel())
	}

	clog := NewChannelLogForSend(msg, redactValues)

	if handler == nil {
		// if there's no handler, create a FAILED status for it
		status = backend.NewStatusUpdate(msg.Channel(), msg.ID(), MsgStatusFailed, clog)
		log.Error(fmt.Sprintf("unable to find handler for channel type: %s", msg.Channel().ChannelType()))

	} else if sent {
		// if this message was already sent, create a WIRED status for it
		status = backend.NewStatusUpdate(msg.Channel(), msg.ID(), MsgStatusWired, clog)
		log.Warn("duplicate send, marking as wired")

	} else {
		// send our message
		status, err = handler.Send(sendCTX, msg, clog)
		duration := time.Since(start)
		secondDuration := float64(duration) / float64(time.Second)

		if err != nil {
			log.Error("error sending message", "error", err, "elapsed", duration)

			// handlers should log errors implicitly with user friendly messages.. but if not.. add what we have
			if len(clog.Errors()) == 0 {
				clog.RawError(err)
			}

			// possible for handlers to only return an error in which case we construct an error status
			if status == nil {
				status = backend.NewStatusUpdate(msg.Channel(), msg.ID(), MsgStatusErrored, clog)
			}
		}

		// report to librato and log locally
		if status.Status() == MsgStatusErrored || status.Status() == MsgStatusFailed {
			log.Warn("msg errored", "elapsed", duration)
			analytics.Gauge(fmt.Sprintf("courier.msg_send_error_%s", msg.Channel().ChannelType()), secondDuration)
		} else {
			log.Debug("msg sent", "elapsed", duration)
			analytics.Gauge(fmt.Sprintf("courier.msg_send_%s", msg.Channel().ChannelType()), secondDuration)
		}
	}

	// we allot 10 seconds to write our status to the db
	writeCTX, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	err = backend.WriteStatusUpdate(writeCTX, status)
	if err != nil {
		log.Info("error writing msg status", "error", err)
	}

	clog.End()

	// write our logs as well
	err = backend.WriteChannelLog(writeCTX, clog)
	if err != nil {
		log.Info("error writing msg logs", "error", err)
	}

	// mark our send task as complete
	backend.MarkOutgoingMsgComplete(writeCTX, msg, status)
}
