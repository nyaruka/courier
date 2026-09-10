package cmd

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/gocommon/aws/cwatch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capturingClient is a cloudwatch client which keeps the metric data it's given
type capturingClient struct {
	sent []cwtypes.MetricDatum
}

func (c *capturingClient) PutMetricData(ctx context.Context, params *cloudwatch.PutMetricDataInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.PutMetricDataOutput, error) {
	c.sent = params.MetricData
	return &cloudwatch.PutMetricDataOutput{}, nil
}

func (c *capturingClient) names() []string {
	names := make([]string, len(c.sent))
	for i, md := range c.sent {
		names[i] = aws.ToString(md.MetricName)
	}
	return names
}

func TestReportMetrics(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)

	client := &capturingClient{}
	rt.CW.Client = client
	rt.Config.MetricsReporting = "basic"

	var dbWait, redisWait time.Duration
	report := func() int {
		count, err := reportMetrics(ctx, rt, &dbWait, &redisWait)
		require.NoError(t, err)
		return count
	}

	// by default the standard set is all that's sent
	standard := report()
	assert.Equal(t, standard, len(client.sent))
	assert.NotContains(t, client.names(), "Refusals")

	// a deployment can add its own, which are sent in the same request and told the reporting level
	calls := 0
	advancedSeen := false
	ExtraMetrics = func(ctx context.Context, rt *runtime.Runtime, advanced bool) []cwtypes.MetricDatum {
		calls++
		advancedSeen = advanced
		return []cwtypes.MetricDatum{
			cwatch.Datum("Refusals", 3, cwtypes.StandardUnitCount, cwatch.Dimension("ChannelType", "EX")),
		}
	}
	defer func() {
		ExtraMetrics = func(context.Context, *runtime.Runtime, bool) []cwtypes.MetricDatum { return nil }
	}()

	assert.Equal(t, standard+1, report())
	assert.Equal(t, 1, calls)
	assert.False(t, advancedSeen)

	extra := client.sent[len(client.sent)-1]
	assert.Equal(t, "Refusals", aws.ToString(extra.MetricName))
	assert.Equal(t, 3.0, aws.ToFloat64(extra.Value))
	assert.Equal(t, []cwtypes.Dimension{
		{Name: aws.String("Deployment"), Value: aws.String("dev")},
		{Name: aws.String("ChannelType"), Value: aws.String("EX")},
	}, extra.Dimensions)

	rt.Config.MetricsReporting = "advanced"
	assert.Greater(t, report(), standard+1)
	assert.Equal(t, 2, calls)
	assert.True(t, advancedSeen)

	// and isn't consulted at all when reporting is off
	rt.Config.MetricsReporting = "off"
	assert.Equal(t, 0, report())
	assert.Equal(t, 2, calls)

	// a panic in the hook loses its metrics for the period but not the standard set, and is reported
	rt.Config.MetricsReporting = "basic"
	ExtraMetrics = func(context.Context, *runtime.Runtime, bool) []cwtypes.MetricDatum { panic("boom") }

	var panicked any
	runtime.PanicHandler = func(val any, tags map[string]string) {
		panicked = val
		assert.Equal(t, map[string]string{"comp": "metrics"}, tags)
	}
	defer func() { runtime.PanicHandler = runtime.LogPanic }()

	assert.Equal(t, standard, report())
	assert.Equal(t, "boom", panicked)
}
