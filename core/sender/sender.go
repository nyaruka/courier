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
	assignWG         *sync.WaitGroup // tracks the assign loop
	stopOnce         sync.Once
	waitGroup        *sync.WaitGroup // tracks the senders
}

// NewForeman creates a new Foreman for the passed in runtime with the number of max senders
func NewForeman(rt *runtime.Runtime, maxSenders int) *Foreman {
	foreman := &Foreman{
		rt:               rt,
		senders:          make([]*Sender, maxSenders),
		availableSenders: make(chan *Sender, maxSenders),
		quit:             make(chan bool),
		assignWG:         &sync.WaitGroup{},
		waitGroup:        &sync.WaitGroup{},
	}

	for i := 0; i < maxSenders; i++ {
		foreman.senders[i] = NewSender(foreman, i)
	}

	return foreman
}

// Start starts the foreman and all its senders, assigning jobs while there are some. It must be called at most once.
func (f *Foreman) Start() {
	for _, sender := range f.senders {
		sender.Start()
	}

	f.assignWG.Add(1)
	go func() {
		defer f.assignWG.Done()
		f.assign()
	}()
}

// Stop stops assigning messages, then stops the senders and waits for the sends in progress to finish. Calling it
// again waits for the same thing.
func (f *Foreman) Stop() {
	f.stopOnce.Do(func() {
		slog.Info("foreman stopping", "comp", "foreman", "state", "stopping")

		// stop the assign loop first and wait for it to return, so that nothing more is popped from the queue - a
		// message popped after the senders have been stopped has nobody to send it and is lost
		close(f.quit)
		f.assignWG.Wait()

		for _, sender := range f.senders {
			sender.Stop()
		}
	})

	// wait for all senders to finish what they're doing
	f.waitGroup.Wait()
}

// assign is our main loop for the Foreman, it takes care of popping the next outgoing messages from the
// queue and assigning them to workers. It's only run by Start, which is what lets Stop wait for it.
func (f *Foreman) assign() {
	log := slog.With("comp", "foreman")
	defer log.Info("foreman stopped", "state", "stopped")

	log.Info("senders started and waiting",
		"state", "started",
		"senders", len(f.senders))

	lastSleep := false

	for {
		// check for being told to stop before looking for a sender - the select below picks at random when both are
		// ready, and each extra message popped whilst stopping adds its pop and send to how long stopping takes
		select {
		case <-f.quit:
			return
		default:
		}

		select {
		// return if we have been told to stop
		case <-f.quit:
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

				select {
				case <-f.quit:
				case <-time.After(250 * time.Millisecond):
				}
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

// Stop stops our sender once it has sent any message it's been given, callers can use the foreman's wait group to
// track progress
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
	handler := channels.GetHandler(msg.Channel().ChannelType())
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

	// write our logs as well - unless this channel type doesn't store them
	if handler == nil || handler.StoreChannelLogs() {
		models.WriteChannelLog(rt, clog)
	}

	// mark our send task as complete
	var cost int
	var otherURNs []urns.URN
	if res != nil {
		cost = res.Cost()
		otherURNs = res.OtherURNs()
	}
	models.OnSendComplete(writeCTX, rt, msg, status, cost, otherURNs, clog)
}

func (w *Sender) sendByHandler(ctx context.Context, h channels.Handler, m *models.MsgOut, clog *models.ChannelLog, log *slog.Logger) (*models.StatusUpdate, *channels.SendResult) {
	rt := w.foreman.rt
	res := &channels.SendResult{}
	err := h.Send(ctx, m, res, clog)

	// if the channel can't take any more for now, stop popping its messages so they wait in our queue rather than its
	if backoff := channels.RequestedBackoff(res, err); backoff > 0 {
		log.Info("pausing sending on channel", "backoff", backoff)

		// the send may have used up most of its context, so the pause gets its own
		pauseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := models.PauseSending(pauseCtx, rt, m, time.Now().Add(backoff)); err != nil {
			log.Error("error pausing sending on channel", "error", err)
		}
		cancel()
	}

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
