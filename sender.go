package courier

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nyaruka/courier/utils/clogs"
	"github.com/nyaruka/gocommon/urns"
)

type SendResult struct {
	externalIDs []string
	newURN      urns.URN
}

func (r *SendResult) AddExternalID(id string) {
	r.externalIDs = append(r.externalIDs, id)
}

func (r *SendResult) ExternalIDs() []string {
	return r.externalIDs
}

func (r *SendResult) SetNewURN(u urns.URN) {
	r.newURN = u
}

func (r *SendResult) GetNewURN() urns.URN {
	return r.newURN

}

type SendError struct {
	msg       string
	retryable bool
	loggable  bool

	clogCode    string
	clogMsg     string
	clogExtCode string
}

func (e *SendError) Error() string {
	return e.msg
}

// ErrChannelConfig should be returned by a handler send method when channel config is invalid
var ErrChannelConfig error = &SendError{
	msg:       "channel config invalid",
	retryable: false,
	loggable:  true,
	clogCode:  "channel_config",
	clogMsg:   "Channel configuration is missing required values.",
}

// ErrMessageInvalid should be returned by a handler send method when the message it has received is invalid
var ErrMessageInvalid error = &SendError{
	msg:       "message invalid",
	retryable: false,
	loggable:  true,
	clogCode:  "message_invalid",
	clogMsg:   "Message is missing required values.",
}

// ErrConnectionFailed should be returned when connection to the channel fails (timeout or 5XX response)
var ErrConnectionFailed error = &SendError{
	msg:       "channel connection failed",
	retryable: true,
	loggable:  false,
	clogCode:  "connection_failed",
	clogMsg:   "Connection to server failed.",
}

// ErrConnectionThrottled should be returned when channel tells us we're rate limited
var ErrConnectionThrottled error = &SendError{
	msg:       "channel rate limited",
	retryable: true,
	loggable:  false,
	clogCode:  "connection_throttled",
	clogMsg:   "Connection to server has been rate limited.",
}

// ErrResponseStatus should be returned when the response from the channel has a non-success status code
var ErrResponseStatus error = &SendError{
	msg:       "response status code",
	retryable: false,
	loggable:  false,
	clogCode:  "response_status",
	clogMsg:   "Response has non-success status code.",
}

// ErrResponseContent should be returned when the response content from the channel indicates non-succeess
var ErrResponseContent error = &SendError{
	msg:       "response content",
	retryable: false,
	loggable:  false,
	clogCode:  "response_content",
	clogMsg:   "Response content indicates non-success.",
}

// ErrResponseUnparseable should be returned when channel response can't be parsed in expected format
var ErrResponseUnparseable error = &SendError{
	msg:       "response couldn't be parsed",
	retryable: false,
	loggable:  true,
	clogCode:  "response_unparseable",
	clogMsg:   "Response could not be parsed in the expected format.",
}

// ErrResponseUnexpected should be returned when channel response doesn't match what we expect
var ErrResponseUnexpected error = &SendError{
	msg:       "response not expected values",
	retryable: false,
	loggable:  true,
	clogCode:  "response_unexpected",
	clogMsg:   "Response doesn't match expected values.",
}

// ErrContactStopped should be returned when channel tells us explicitly that the contact has opted-out
var ErrContactStopped error = &SendError{
	msg:       "contact opted out",
	retryable: false,
	loggable:  false,
	clogCode:  "contact_stopped",
	clogMsg:   "Contact has opted-out of messages from this channel.",
}

func ErrFailedWithReason(code, desc string) *SendError {
	return &SendError{
		msg:         "channel rejected send with reason",
		retryable:   false,
		loggable:    false,
		clogCode:    "rejected_with_reason",
		clogMsg:     desc,
		clogExtCode: code,
	}
}

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
		status = w.sendByHandler(sendCTX, handler, msg, clog, log)
	}

	// we allot 15 seconds to write our status to the db
	writeCTX, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := backend.WriteStatusUpdate(writeCTX, status); err != nil {
		log.Info("error writing msg status", "error", err)
	}

	clog.End()

	// write our logs as well
	if err := backend.WriteChannelLog(writeCTX, clog); err != nil {
		log.Info("error writing msg logs", "error", err)
	}

	// mark our send task as complete
	backend.OnSendComplete(writeCTX, msg, status, clog)
}

func (w *Sender) sendByHandler(ctx context.Context, h ChannelHandler, m MsgOut, clog *ChannelLog, log *slog.Logger) StatusUpdate {
	backend := w.foreman.server.Backend()
	res := &SendResult{newURN: urns.NilURN}
	err := h.Send(ctx, m, res, clog)

	status := backend.NewStatusUpdate(m.Channel(), m.ID(), MsgStatusWired, clog)

	// fow now we can only store one external id per message
	if len(res.ExternalIDs()) > 0 {
		status.SetExternalID(res.ExternalIDs()[0])
	}

	if res.newURN != urns.NilURN {
		urnErr := status.SetURNUpdate(m.URN(), res.newURN)
		if urnErr != nil {
			clog.RawError(urnErr)
		}
	}

	var serr *SendError
	if errors.As(err, &serr) {
		if serr.loggable {
			log.Error("error sending message", "error", err)
		}
		if serr.retryable {
			status.SetStatus(MsgStatusErrored)
		} else {
			status.SetStatus(MsgStatusFailed)
		}

		clog.Error(&clogs.Error{Code: serr.clogCode, ExtCode: serr.clogExtCode, Message: serr.clogMsg})

		// if handler returned ErrContactStopped need to write a stop event
		if serr == ErrContactStopped {
			channelEvent := backend.NewChannelEvent(m.Channel(), EventTypeStopContact, m.URN(), clog)
			if err = backend.WriteChannelEvent(ctx, channelEvent, clog); err != nil {
				log.Error("error writing stop event", "error", err)
			}
		}

	} else if err != nil {
		log.Error("error sending message", "error", err)

		status.SetStatus(MsgStatusErrored)

		clog.Error(&clogs.Error{Code: "internal_error", Message: "An internal error occured."})
	}

	return status
}
