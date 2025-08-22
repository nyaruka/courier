package rapidpro

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/courier/utils/clogs"
	"github.com/nyaruka/gocommon/aws/dynamo"
	"github.com/nyaruka/gocommon/httpx"
)

const (
	dynamoChannelLogTTL = 7 * 24 * time.Hour // 1 week
)

// ChannelLog wraps a courier.ChannelLog to add DynamoDB support.
type ChannelLog struct {
	*courier.ChannelLog
}

func (l *ChannelLog) DynamoKey() models.DynamoKey {
	pk := fmt.Sprintf("cha#%s#%s", l.Channel().UUID(), l.UUID[len(l.UUID)-1:]) // 16 buckets for each channel
	sk := fmt.Sprintf("log#%s", l.UUID)
	return models.DynamoKey{PK: pk, SK: sk}
}

func (l *ChannelLog) MarshalDynamo() (map[string]types.AttributeValue, error) {
	type DataGZ struct {
		HttpLogs []*httpx.Log   `json:"http_logs"`
		Errors   []*clogs.Error `json:"errors"`
	}

	dataGZ, err := dynamo.MarshalJSONGZ(&DataGZ{HttpLogs: l.HttpLogs, Errors: l.Errors})
	if err != nil {
		return nil, fmt.Errorf("error encoding http logs as JSON+GZip: %w", err)
	}

	ttl := l.CreatedOn.Add(dynamoChannelLogTTL)

	return dynamo.Marshal(&models.DynamoItem{
		DynamoKey: l.DynamoKey(),
		OrgID:     l.Channel().(*models.Channel).OrgID(),
		TTL:       &ttl,
		Data: map[string]any{
			"type":       l.Type,
			"elapsed_ms": int(l.Elapsed / time.Millisecond),
			"created_on": l.CreatedOn,
			"is_error":   l.IsError(),
		},
		DataGZ: dataGZ,
	})
}

// queues the passed in channel log to a writer
func queueChannelLog(b *backend, clog *courier.ChannelLog) {
	log := slog.With("log_uuid", clog.UUID, "log_type", clog.Type, "channel_uuid", clog.Channel().UUID())
	dbChan := clog.Channel().(*models.Channel)
	isError := clog.IsError()

	// depending on the channel log policy, we might be able to discard this log
	if dbChan.LogPolicy == models.LogPolicyNone || (dbChan.LogPolicy == models.LogPolicyErrors && !isError) {
		return
	}

	capacity, err := b.dynamoWriter.Queue(&ChannelLog{clog})
	if err != nil {
		log.Error("error writing channel log to dynamo writer", "error", err)
		return
	}
	if capacity <= 0 {
		log.With("storage", "dynamo").Error("channel log writer buffer full")
	}

	log.Debug("channel log queued")
}
