package rapidpro

import (
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/gocommon/aws/cwatch"
)

type CountByType map[courier.ChannelType]int

// converts per channel counts into a set of cloudwatch metrics with type as a dimension, and a total count without type
func (c CountByType) metrics(name string) []types.MetricDatum {
	m := make([]types.MetricDatum, 0, len(c)+1)
	for typ, count := range c {
		m = append(m, cwatch.Datum(name, float64(count), types.StandardUnitCount, cwatch.Dimension("ChannelType", string(typ))))
	}
	return m
}

type DurationByType map[courier.ChannelType]time.Duration

func (c DurationByType) metrics(name string, avgDenom func(courier.ChannelType) int) []types.MetricDatum {
	m := make([]types.MetricDatum, 0, len(c)+1)
	for typ, d := range c { // convert to averages
		avgTime := d / time.Duration(avgDenom(typ))
		m = append(m, cwatch.Datum(name, float64(avgTime)/float64(time.Second), types.StandardUnitSeconds, cwatch.Dimension("ChannelType", string(typ))))
	}
	return m
}

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

func (s *Stats) ToMetrics() []types.MetricDatum {
	metrics := make([]types.MetricDatum, 0, 20)
	metrics = append(metrics, s.IncomingRequests.metrics("IncomingRequests")...)
	metrics = append(metrics, s.IncomingMessages.metrics("IncomingMessages")...)
	metrics = append(metrics, s.IncomingStatuses.metrics("IncomingStatuses")...)
	metrics = append(metrics, s.IncomingEvents.metrics("IncomingEvents")...)
	metrics = append(metrics, s.IncomingIgnored.metrics("IncomingIgnored")...)
	metrics = append(metrics, s.IncomingDuration.metrics("IncomingDuration", func(typ courier.ChannelType) int { return s.IncomingRequests[typ] })...)

	metrics = append(metrics, s.OutgoingSends.metrics("OutgoingSends")...)
	metrics = append(metrics, s.OutgoingErrors.metrics("OutgoingErrors")...)
	metrics = append(metrics, s.OutgoingDuration.metrics("OutgoingDuration", func(typ courier.ChannelType) int { return s.OutgoingSends[typ] + s.OutgoingErrors[typ] })...)

	metrics = append(metrics, cwatch.Datum("ContactsCreated", float64(s.ContactsCreated), types.StandardUnitCount))
	return metrics
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
