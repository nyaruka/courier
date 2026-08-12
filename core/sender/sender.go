package sender

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/gocommon/svclogs"
	"github.com/nyaruka/gocommon/urns"
)

// Foreman takes care of managing our set of sending workers and assigns msgs for each to send
type Foreman struct {
	rt               *runtime.Runtime
	senders          []*Sender
	availableSenders chan *Sender
	quit             chan bool
	waitGroup        *sync.WaitGroup
}

// NewForeman creates a new Foreman for the passed in server with the number of max senders
func NewForeman(rt *runtime.Runtime, maxSenders int) *Foreman {
	foreman := &Foreman{
		rt:               rt,
		senders:          make([]*Sender, maxSenders),
		availableSenders: make(chan *Sender, maxSenders),
		quit:             make(chan bool),
		waitGroup:        &sync.WaitGroup{},
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

	// wait for the assign loop and all senders to finish what they're doing
	f.waitGroup.Wait()
}

// Assign is our main loop for the Foreman, it takes care of popping the next outgoing messages from our
// backend and assigning them to workers
func (f *Foreman) Assign() {
	f.waitGroup.Add(1)
	defer f.waitGroup.Done()
	log := slog.With("comp", "foreman")

	log.Info("senders started and waiting",
		"state", "started",
		"senders", len(f.senders))

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
			msg, err := models.PopNextOutgoingMsg(ctx, f.rt)
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
	job     chan *models.MsgOut
}

// NewSender creates a new sender responsible for sending messages
func NewSender(foreman *Foreman, id int) *Sender {
	sender := &Sender{
		id:      id,
		foreman: foreman,
		job:     make(chan *models.MsgOut, 1),
	}
	return sender
}

// Start starts our Sender's goroutine and has it start waiting for tasks from the foreman
func (w *Sender) Start() {
	w.foreman.waitGroup.Add(1)

	go func() {
		defer w.foreman.waitGroup.Done()
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

func (w *Sender) sendMessage(msg *models.MsgOut) {
	log := slog.With("comp", "sender", "sender_id", w.id, "channel_uuid", msg.Channel().UUID())

	rt := w.foreman.rt

	// we don't want any individual send taking more than 35s
	sendCTX, cancel := context.WithTimeout(context.Background(), time.Second*35)
	defer cancel()

	log = log.With("msg_uuid", msg.UUID(), "msg_text", msg.Text(), "msg_urn", msg.URN().Identity())
	if len(msg.Attachments()) > 0 {
		log = log.With("attachments", msg.Attachments())
	}
	if len(msg.QuickReplies()) > 0 {
		log = log.With("quick_replies", msg.QuickReplies())
	}

	// if this is a resend, clear our sent status
	if msg.IsResend() {
		err := models.ClearMsgSent(sendCTX, rt, msg.UUID())
		if err != nil {
			log.Error("error clearing sent status for msg", "error", err)
		}
	}

	// was this msg already sent? (from a double queue?)
	sent, err := models.WasMsgSent(sendCTX, rt, msg.UUID())

	// failing on a lookup isn't a halting problem but we should log it
	if err != nil {
		log.Error("error looking up msg was sent", "error", err)
	}

	var status *models.StatusUpdate
	var res *channels.SendResult
	var redactValues []string
	handler := channels.GetActiveHandler(msg.Channel().ChannelType())
	if handler != nil {
		redactValues = handler.RedactValues(msg.Channel())
	}

	clog := models.NewChannelLogForSend(msg, redactValues)

	if handler == nil {
		// if there's no handler, create a FAILED status for it
		status = models.NewStatusUpdate(msg.Channel(), msg.UUID(), models.MsgStatusFailed, clog)
		log.Error(fmt.Sprintf("unable to find handler for channel type: %s", msg.Channel().ChannelType()))

	} else if sent {
		// if this message was already sent, create a WIRED status for it
		status = models.NewStatusUpdate(msg.Channel(), msg.UUID(), models.MsgStatusWired, clog)
		log.Warn("duplicate send, marking as wired")

	} else {
		status, res = w.sendByHandler(sendCTX, handler, msg, clog, log)
	}

	// we allot 15 seconds to write our status to the db
	writeCTX, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := models.WriteStatusUpdate(writeCTX, rt, status); err != nil {
		log.Info("error writing msg status", "error", err)
	}

	clog.End()

	// write our logs as well
	models.WriteChannelLog(rt, clog)

	// mark our send task as complete
	var newURN urns.URN
	if res != nil {
		newURN = res.NewURN()
	}
	models.OnSendComplete(writeCTX, rt, msg, status, newURN, clog)
}

func (w *Sender) sendByHandler(ctx context.Context, h channels.Handler, m *models.MsgOut, clog *models.ChannelLog, log *slog.Logger) (*models.StatusUpdate, *channels.SendResult) {
	rt := w.foreman.rt
	res := &channels.SendResult{}
	err := h.Send(ctx, m, res, clog)

	status := models.NewStatusUpdate(m.Channel(), m.UUID(), models.MsgStatusWired, clog)

	// fow now we can only store one external id per message
	if len(res.ExternalIDs()) > 0 {
		status.SetExternalIdentifier(res.ExternalIDs()[0])
	}

	var serr *channels.SendError
	if errors.As(err, &serr) {
		if serr.Loggable() {
			log.Error("error sending message", "error", err)
		}
		if serr.Retryable() {
			status.SetStatus(models.MsgStatusErrored)
		} else {
			status.SetStatus(models.MsgStatusFailed)
		}

		clog.Error(serr.ClogError())

		// if handler returned ErrContactStopped need to write a stop event
		if serr == channels.ErrContactStopped {
			channelEvent := models.NewChannelEvent(m.Channel(), models.EventTypeStopContact, m.URN(), clog)
			if err = models.WriteChannelEvent(ctx, rt, channelEvent, clog); err != nil {
				log.Error("error writing stop event", "error", err)
			}
		}

	} else if err != nil {
		log.Error("error sending message", "error", err)

		status.SetStatus(models.MsgStatusErrored)

		clog.Error(&svclogs.Error{Code: "internal_error", Message: "An internal error occured."})
	}

	return status, res
}
