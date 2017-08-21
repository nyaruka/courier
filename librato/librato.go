package librato

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nyaruka/courier/utils"
	"github.com/sirupsen/logrus"
)

// Default is our default librato collector
var Default *Sender

// NewSender creates a new librato Sender with the passed in parameters
func NewSender(waitGroup *sync.WaitGroup, username string, token string, source string, timeout time.Duration) *Sender {
	return &Sender{
		waitGroup: waitGroup,
		stop:      make(chan bool),

		buffer:   make(chan gauge, 1000),
		username: username,
		token:    token,
		source:   source,
		timeout:  timeout,
	}
}

// AddGauge can be used to add a new gauge to be sent to librato
func (c *Sender) AddGauge(name string, value float64) {
	// if no librato configured, return
	if c == nil {
		return
	}

	// our buffer is full, log an error but continue
	if len(c.buffer) >= cap(c.buffer) {
		logrus.Error("unable to add new gauges, buffer full, you may want to increase your buffer size or decrease your timeout")
		return
	}

	c.buffer <- gauge{Name: strings.ToLower(name), Value: value, MeasureTime: time.Now().Unix()}
}

// Start starts our librato sender, callers can use Stop to stop it
func (c *Sender) Start() {
	if c == nil {
		return
	}

	go func() {
		c.waitGroup.Add(1)
		defer c.waitGroup.Done()

		logrus.WithField("comp", "librato").Info("started for username ", c.username)
		for {
			select {
			case <-c.stop:
				for len(c.buffer) > 0 {
					c.flush(250)
				}
				logrus.WithField("comp", "librato").Info("stopped")
				return

			case <-time.After(c.timeout * time.Second):
				for i := 0; i < 4; i++ {
					c.flush(250)
				}
			}
		}
	}()
}

func (c *Sender) flush(count int) {
	if len(c.buffer) <= 0 {
		return
	}

	// build our payload
	reqPayload := &payload{
		MeasureTime: time.Now().Unix(),
		Source:      c.source,
		Gauges:      make([]gauge, 0, len(c.buffer)),
	}

	// read up to our count of gauges
	for i := 0; i < count; i++ {
		select {
		case g := <-c.buffer:
			reqPayload.Gauges = append(reqPayload.Gauges, g)
		default:
			break
		}
	}

	// send it off
	encoded, err := json.Marshal(reqPayload)
	if err != nil {
		logrus.WithField("comp", "librato").WithError(err).Error("error encoding librato metrics")
		return
	}

	req, err := http.NewRequest("POST", "https://metrics-api.librato.com/v1/metrics", bytes.NewReader(encoded))
	if err != nil {
		logrus.WithField("comp", "librato").WithError(err).Error("error sending librato metrics")
		return
	}
	req.SetBasicAuth(c.username, c.token)
	req.Header.Set("Content-Type", "application/json")
	_, err = utils.MakeHTTPRequest(req)

	if err != nil {
		logrus.WithField("comp", "librato").WithError(err).Error("error sending librato metrics")
		return
	}

	logrus.WithField("comp", "librato").WithField("count", len(reqPayload.Gauges)).Info("flushed to librato")
}

// Stop stops our sender, callers can use the WaitGroup used during initialization to block for stop
func (c *Sender) Stop() {
	if c == nil {
		return
	}
	close(c.stop)
}

type gauge struct {
	Name        string  `json:"name"`
	Value       float64 `json:"value"`
	MeasureTime int64   `json:"measure_time"`
}

type payload struct {
	MeasureTime int64   `json:"measure_time"`
	Source      string  `json:"source"`
	Gauges      []gauge `json:"gauges"`
}

// Sender is responsible for collecting gauges and sending them in batches to our librato server
type Sender struct {
	waitGroup *sync.WaitGroup
	stop      chan bool

	buffer chan gauge

	username string
	token    string
	source   string
	timeout  time.Duration
}
