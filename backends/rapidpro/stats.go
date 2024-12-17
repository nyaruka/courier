package rapidpro

import (
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/gocommon/aws/cwatch"
)

type CountByType map[courier.ChannelType]int

// Metrics converts per channel counts into cloudwatch metrics with type as a dimension
func (c CountByType) Metrics(name string) []types.MetricDatum {
	m := make([]types.MetricDatum, 0, len(c))
	for typ, count := range c {
		m = append(m, cwatch.Datum(name, float64(count), types.StandardUnitCount, cwatch.Dimension("ChannelType", string(typ))))
	}
	return m
}

type DurationByType map[courier.ChannelType]time.Duration

type Stats struct {
	IncomingRequests CountByType    // number of handler requests
	IncomingMessages CountByType    // number of messages received
	IncomingStatuses CountByType    // number of status updates received
	IncomingEvents   CountByType    // number of other events received
	IncomingIgnored  CountByType    // number of requests ignored
	IncomingDuration DurationByType // total time spent handling requests

	OutgoingSends    CountByType    // number of sends that succeeded
	OutgoingErrors   CountByType    // number of sends that errored
	OutgoingDuration DurationByType // total time spent sending messages

	ContactsCreated int
}

func newStats() *Stats {
	return &Stats{
		IncomingRequests: make(CountByType),
		IncomingMessages: make(CountByType),
		IncomingStatuses: make(CountByType),
		IncomingEvents:   make(CountByType),
		IncomingIgnored:  make(CountByType),
		IncomingDuration: make(DurationByType),

		OutgoingSends:    make(CountByType),
		OutgoingErrors:   make(CountByType),
		OutgoingDuration: make(DurationByType),

		ContactsCreated: 0,
	}
}

// StatsCollector provides threadsafe stats collection
type StatsCollector struct {
	mutex sync.Mutex
	stats *Stats
}

// NewStatsCollector creates a new stats collector
func NewStatsCollector() *StatsCollector {
	return &StatsCollector{stats: newStats()}
}

func (c *StatsCollector) RecordIncoming(typ courier.ChannelType, evts []courier.Event, d time.Duration) {
	c.mutex.Lock()
	c.stats.IncomingRequests[typ]++

	for _, e := range evts {
		switch e.(type) {
		case courier.MsgIn:
			c.stats.IncomingMessages[typ]++
		case courier.StatusUpdate:
			c.stats.IncomingStatuses[typ]++
		case courier.ChannelEvent:
			c.stats.IncomingEvents[typ]++
		}
	}
	if len(evts) == 0 {
		c.stats.IncomingIgnored[typ]++
	}

	c.stats.IncomingDuration[typ] += d
	c.mutex.Unlock()
}

func (c *StatsCollector) RecordOutgoing(typ courier.ChannelType, success bool, d time.Duration) {
	c.mutex.Lock()
	if success {
		c.stats.OutgoingSends[typ]++
	} else {
		c.stats.OutgoingErrors[typ]++
	}
	c.stats.OutgoingDuration[typ] += d
	c.mutex.Unlock()
}

func (c *StatsCollector) RecordContactCreated() {
	c.mutex.Lock()
	c.stats.ContactsCreated++
	c.mutex.Unlock()
}

// Extract returns the stats for the period since the last call
func (c *StatsCollector) Extract() *Stats {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	s := c.stats
	c.stats = newStats()
	return s
}
