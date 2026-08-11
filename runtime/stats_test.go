package runtime_test

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/gocommon/aws/cwatch"
	"github.com/stretchr/testify/assert"
)

func TestStats(t *testing.T) {
	sc := runtime.NewStatsCollector()
	sc.RecordContactCreated()
	sc.RecordContactCreated()
	sc.RecordIncoming("T", 0, 0, 0, 1, time.Second)
	sc.RecordOutgoing("T", true, time.Second)
	sc.RecordOutgoing("T", true, time.Second)
	sc.RecordOutgoing("FBA", true, time.Second)
	sc.RecordOutgoing("FBA", true, time.Second)
	sc.RecordOutgoing("FBA", true, time.Second)

	stats := sc.Extract()

	assert.Equal(t, 2, stats.ContactsCreated)
	assert.Equal(t, runtime.CountByType{"T": 1}, stats.IncomingRequests)
	assert.Equal(t, runtime.CountByType{}, stats.IncomingMessages)
	assert.Equal(t, runtime.CountByType{}, stats.IncomingStatuses)
	assert.Equal(t, runtime.CountByType{}, stats.IncomingEvents)
	assert.Equal(t, runtime.DurationByType{"T": time.Second}, stats.IncomingDuration)
	assert.Equal(t, runtime.CountByType{"T": 2, "FBA": 3}, stats.OutgoingSends)
	assert.Equal(t, runtime.CountByType{}, stats.OutgoingErrors)
	assert.Equal(t, runtime.DurationByType{"T": time.Second * 2, "FBA": time.Second * 3}, stats.OutgoingDuration)

	metrics := stats.ToMetrics(true)
	assert.Len(t, metrics, 8)

	sc.RecordOutgoing("FBA", true, time.Second)
	sc.RecordOutgoing("FBA", true, time.Second)

	stats = sc.Extract()

	assert.Equal(t, 0, stats.ContactsCreated)
	assert.Equal(t, runtime.CountByType{}, stats.IncomingRequests)
	assert.Equal(t, runtime.CountByType{}, stats.IncomingMessages)
	assert.Equal(t, runtime.CountByType{}, stats.IncomingStatuses)
	assert.Equal(t, runtime.CountByType{}, stats.IncomingEvents)
	assert.Equal(t, runtime.DurationByType{}, stats.IncomingDuration)
	assert.Equal(t, runtime.CountByType{"FBA": 2}, stats.OutgoingSends)
	assert.Equal(t, runtime.CountByType{}, stats.OutgoingErrors)
	assert.Equal(t, runtime.DurationByType{"FBA": time.Second * 2}, stats.OutgoingDuration)

	metrics = stats.ToMetrics(true)
	assert.Len(t, metrics, 3)
	assert.Equal(t, []types.MetricDatum{
		cwatch.Datum("OutgoingSends", 2, "Count", cwatch.Dimension("ChannelType", "FBA")),
		cwatch.Datum("OutgoingDuration", 1, "Seconds", cwatch.Dimension("ChannelType", "FBA")),
		cwatch.Datum("ContactsCreated", 0, "Count"),
	}, metrics)
}
