package rapidpro_test

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/backends/rapidpro"
	"github.com/nyaruka/gocommon/aws/cwatch"
	"github.com/stretchr/testify/assert"
)

func TestStats(t *testing.T) {
	sc := rapidpro.NewStatsCollector()
	sc.RecordContactCreated()
	sc.RecordContactCreated()
	sc.RecordIncoming("T", []courier.Event{}, time.Second)
	sc.RecordOutgoing("T", true, time.Second)
	sc.RecordOutgoing("T", true, time.Second)
	sc.RecordOutgoing("FBA", true, time.Second)
	sc.RecordOutgoing("FBA", true, time.Second)
	sc.RecordOutgoing("FBA", true, time.Second)

	stats := sc.Extract()

	assert.Equal(t, 2, stats.ContactsCreated)
	assert.Equal(t, rapidpro.CountByType{"T": 1}, stats.IncomingRequests)
	assert.Equal(t, rapidpro.CountByType{}, stats.IncomingMessages)
	assert.Equal(t, rapidpro.CountByType{}, stats.IncomingStatuses)
	assert.Equal(t, rapidpro.CountByType{}, stats.IncomingEvents)
	assert.Equal(t, rapidpro.DurationByType{"T": time.Second}, stats.IncomingDuration)
	assert.Equal(t, rapidpro.CountByType{"T": 2, "FBA": 3}, stats.OutgoingSends)
	assert.Equal(t, rapidpro.CountByType{}, stats.OutgoingErrors)
	assert.Equal(t, rapidpro.DurationByType{"T": time.Second * 2, "FBA": time.Second * 3}, stats.OutgoingDuration)

	metrics := stats.ToMetrics()
	assert.Len(t, metrics, 8)

	sc.RecordOutgoing("FBA", true, time.Second)
	sc.RecordOutgoing("FBA", true, time.Second)

	stats = sc.Extract()

	assert.Equal(t, 0, stats.ContactsCreated)
	assert.Equal(t, rapidpro.CountByType{}, stats.IncomingRequests)
	assert.Equal(t, rapidpro.CountByType{}, stats.IncomingMessages)
	assert.Equal(t, rapidpro.CountByType{}, stats.IncomingStatuses)
	assert.Equal(t, rapidpro.CountByType{}, stats.IncomingEvents)
	assert.Equal(t, rapidpro.DurationByType{}, stats.IncomingDuration)
	assert.Equal(t, rapidpro.CountByType{"FBA": 2}, stats.OutgoingSends)
	assert.Equal(t, rapidpro.CountByType{}, stats.OutgoingErrors)
	assert.Equal(t, rapidpro.DurationByType{"FBA": time.Second * 2}, stats.OutgoingDuration)

	metrics = stats.ToMetrics()
	assert.Len(t, metrics, 3)
	assert.Equal(t, []types.MetricDatum{
		cwatch.Datum("OutgoingSends", 2, "Count", cwatch.Dimension("ChannelType", "FBA")),
		cwatch.Datum("OutgoingDuration", 1, "Seconds", cwatch.Dimension("ChannelType", "FBA")),
		cwatch.Datum("ContactsCreated", 0, "Count"),
	}, metrics)
}
