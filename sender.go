package courier

import (
	"fmt"
	"time"

	"github.com/nyaruka/courier/librato"
	"github.com/sirupsen/logrus"
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
	logrus.WithField("comp", "foreman").WithField("state", "stopping").Info("foreman stopping")
}

// Assign is our main loop for the Foreman, it takes care of popping the next outgoing messages from our
// backend and assigning them to workers
func (f *Foreman) Assign() {
	f.server.WaitGroup().Add(1)
	defer f.server.WaitGroup().Done()
	log := logrus.WithField("comp", "foreman")

	log.WithFields(logrus.Fields{
		"state":   "started",
		"senders": len(f.senders),
	}).Info("senders started and waiting")

	backend := f.server.Backend()
	lastSleep := false

	for true {
		select {
		// return if we have been told to stop
		case <-f.quit:
			log.WithField("state", "stopped").Info("foreman stopped")
			return

		// otherwise, grab the next msg and assign it to a sender
		case sender := <-f.availableSenders:
			// see if we have a message to work on
			msg, err := backend.PopNextOutgoingMsg()

			if err == nil && msg != nil {
				// if so, assign it to our sender
				sender.job <- msg
				lastSleep = false
			} else {
				// we received an error getting the next message, log it
				if err != nil {
					log.WithError(err).Error("error popping outgoing msg")
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
	job     chan Msg
}

// NewSender creates a new sender responsible for sending messages
func NewSender(foreman *Foreman, id int) *Sender {
	sender := &Sender{
		id:      id,
		foreman: foreman,
		job:     make(chan Msg, 1),
	}
	return sender
}

// Start starts our Sender's goroutine and has it start waiting for tasks from the foreman
func (w *Sender) Start() {
	go w.Send()
}

// Stop stops our senders, callers can use the server's wait group to track progress
func (w *Sender) Stop() {
	close(w.job)
}

// Send is our main work loop for our worker. The Worker marks itself as available for work
// to the foreman, then waits for the next job
func (w *Sender) Send() {
	w.foreman.server.WaitGroup().Add(1)
	defer w.foreman.server.WaitGroup().Done()

	log := logrus.WithField("comp", "sender").WithField("sender_id", w.id)
	log.Debug("started")

	server := w.foreman.server
	backend := server.Backend()

	var status MsgStatus

	for true {
		// list ourselves as available for work
		w.foreman.availableSenders <- w

		// grab our next piece of work
		msg := <-w.job

		// exit if we were stopped
		if msg == nil {
			log.Debug("stopped")
			return
		}

		msgLog := log.WithField("msg_id", msg.ID().String()).WithField("msg_text", msg.Text()).WithField("msg_urn", msg.URN().Identity())
		if msg.Attachments() != nil {
			msgLog = log.WithField("attachments", msg.Attachments())
		}
		if msg.QuickReplies() != nil {
			msgLog = log.WithField("quick_replies", msg.QuickReplies())
		}

		start := time.Now()

		// was this msg already sent? (from a double queue?)
		sent, err := backend.WasMsgSent(msg)

		// failing on a lookup isn't a halting problem but we should log it
		if err != nil {
			msgLog.WithError(err).Warning("error looking up msg was sent")
		}

		if sent {
			// if this message was already sent, create a wired status for it
			status = backend.NewMsgStatusForID(msg.Channel(), msg.ID(), MsgWired)
			msgLog.Warning("duplicate send, marking as wired")
		} else {
			// send our message
			status, err = server.SendMsg(msg)
			duration := time.Now().Sub(start)
			secondDuration := float64(duration) / float64(time.Second)

			if err != nil {
				msgLog.WithError(err).WithField("elapsed", duration).Error("error sending message")
				if status == nil {
					status = backend.NewMsgStatusForID(msg.Channel(), msg.ID(), MsgErrored)
				}
			}

			// report to librato and log locally
			if status.Status() == MsgErrored || status.Status() == MsgFailed {
				msgLog.WithField("elapsed", duration).Warning("msg errored")
				librato.Default.AddGauge(fmt.Sprintf("courier.msg_send_error_%s", msg.Channel().ChannelType()), secondDuration)
			} else {
				msgLog.WithField("elapsed", duration).Info("msg sent")
				librato.Default.AddGauge(fmt.Sprintf("courier.msg_send_%s", msg.Channel().ChannelType()), secondDuration)
			}
		}

		err = backend.WriteMsgStatus(status)
		if err != nil {
			msgLog.WithError(err).Info("error writing msg status")
		}

		// write our logs as well
		err = backend.WriteChannelLogs(status.Logs())
		if err != nil {
			msgLog.WithError(err).Info("error writing msg logs")
		}

		// mark our send task as complete
		backend.MarkOutgoingMsgComplete(msg, status)
	}
}
