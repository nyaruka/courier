package courier

import (
	"time"

	"github.com/sirupsen/logrus"
)

// Foreman takes care of managing our set of sending workers and assigns msgs for each to send
type Foreman struct {
	server           Server
	workers          []*Worker
	availableWorkers chan *Worker
	quit             chan bool
}

// NewForeman creates a new Foreman for the passed in server with the number of max workers
func NewForeman(server Server, maxWorkers int) *Foreman {
	foreman := &Foreman{
		server:           server,
		workers:          make([]*Worker, maxWorkers),
		availableWorkers: make(chan *Worker, maxWorkers),
		quit:             make(chan bool),
	}

	for i := 0; i < maxWorkers; i++ {
		foreman.workers[i] = NewWorker(foreman, i)
	}

	return foreman
}

// Start starts the foreman and all its workers, assigning jobs while there are some
func (f *Foreman) Start() {
	for _, worker := range f.workers {
		worker.Start()
	}
	go f.Assign()
}

// Stop stops the foreman and all its workers, the wait group of the server can be used to track progress
func (f *Foreman) Stop() {
	for _, worker := range f.workers {
		worker.Stop()
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
		"workers": len(f.workers),
	}).Info("sending workers started and waiting")

	backend := f.server.Backend()
	lastSleep := false

	for true {
		select {
		// return if we have been told to stop
		case <-f.quit:
			log.WithField("state", "stopped").Info("foreman stopped")
			return

		// otherwise, grab the next msg and assign it to a worker
		default:
			// get the next worker that is ready
			worker := <-f.availableWorkers

			// see if we have a message to work on
			msg, err := backend.PopNextOutgoingMsg()
			if err != nil {
				log.WithError(err).Error("error popping outgoing msg")
				break
			}

			if msg != nil {
				// if so, assign it to our worker
				worker.job <- msg
				lastSleep = false
			} else {
				// otherwise, add our worker back to our queue and sleep a bit
				if !lastSleep {
					log.Info("sleeping, no messages")
					lastSleep = true
				}
				f.availableWorkers <- worker
				time.Sleep(250 * time.Millisecond)
			}
		}
	}
}

// Worker is our type for a single goroutine that is sending messages
type Worker struct {
	id      int
	foreman *Foreman
	job     chan *Msg
}

// NewWorker creates a new worker responsible for sending messages
func NewWorker(foreman *Foreman, id int) *Worker {
	worker := &Worker{
		id:      id,
		foreman: foreman,
		job:     make(chan *Msg, 1),
	}
	return worker
}

// Start starts our Worker's goroutine and has it start waiting for tasks from the foreman
func (w *Worker) Start() {
	go w.Work()
}

// Stop stops our workers, callers can use the server's wait group to track progress
func (w *Worker) Stop() {
	close(w.job)
}

// Work is our main work loop for our worker. The Worker marks itself as available for work
// to the foreman, then waits for the next job
func (w *Worker) Work() {
	w.foreman.server.WaitGroup().Add(1)
	defer w.foreman.server.WaitGroup().Done()

	log := logrus.WithField("comp", "worker").WithField("workerID", w.id)
	log.Debug("started")

	server := w.foreman.server

	for true {
		// list ourselves as available for work
		w.foreman.availableWorkers <- w

		// grab our next piece of work
		msg := <-w.job

		// exit if we were stopped
		if msg == nil {
			log.Debug("stopped")
			return
		}

		status, err := server.SendMsg(msg)
		if err != nil {
			log.WithField("msgID", msg.ID.Int64).WithError(err).Info("msg errored")
		} else {
			log.WithField("msgID", msg.ID.Int64).Info("msg sent")
		}

		// record our status
		err = server.WriteMsgStatus(status)
		if err != nil {
			log.WithField("msgID", msg.ID.Int64).WithError(err).Info("error writing msg status")
		}

		// mark our send task as complete
		server.Backend().MarkOutgoingMsgComplete(msg)
	}
}
