package rapidpro

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/utils/clogs"
	"github.com/nyaruka/gocommon/aws/dynamo"
	"github.com/nyaruka/gocommon/httpx"
)

const (
	dynamoChannelLogTTL = 7 * 24 * time.Hour // 1 week
)

// queues the passed in channel log to a writer
func queueChannelLog(b *backend, clog *courier.ChannelLog) {
	log := slog.With("log_uuid", clog.UUID, "log_type", clog.Type, "channel_uuid", clog.Channel().UUID())
	dbChan := clog.Channel().(*Channel)
	isError := clog.IsError()

	// depending on the channel log policy, we might be able to discard this log
	if dbChan.LogPolicy == LogPolicyNone || (dbChan.LogPolicy == LogPolicyErrors && !isError) {
		return
	}

	dynLog, err := NewDynamoChannelLog(clog)
	if err != nil {
		log.Error("error creating dynamo channel log", "error", err)
		return
	}

	if b.dynamoWriter.Queue(dynLog) <= 0 {
		log.With("storage", "dynamo").Error("channel log writer buffer full")
	}

	log.Debug("channel log queued")
}

func NewDynamoChannelLog(clog *courier.ChannelLog) (*DynamoItem, error) {
	key := GetChannelLogKey(clog)

	type DataGZ struct {
		HttpLogs []*httpx.Log   `json:"http_logs"`
		Errors   []*clogs.Error `json:"errors"`
	}

	dataGZ, err := dynamo.MarshalJSONGZ(&DataGZ{HttpLogs: clog.HttpLogs, Errors: clog.Errors})
	if err != nil {
		return nil, fmt.Errorf("error encoding http logs as JSON+GZip: %w", err)
	}

	return &DynamoItem{
		DynamoKey: key,
		OrgID:     int(clog.Channel().(*Channel).OrgID()),
		TTL:       clog.CreatedOn.Add(dynamoChannelLogTTL),
		Data: map[string]any{
			"type":       clog.Type,
			"elapsed_ms": int(clog.Elapsed / time.Millisecond),
			"created_on": clog.CreatedOn,
			"is_error":   clog.IsError(),
		},
		DataGZ: dataGZ,
	}, nil
}

func GetChannelLogKey(l *courier.ChannelLog) DynamoKey {
	pk := fmt.Sprintf("cha#%s#%s", l.Channel().UUID(), l.UUID[len(l.UUID)-1:]) // 16 buckets for each channel
	sk := fmt.Sprintf("log#%s", l.UUID)
	return DynamoKey{PK: pk, SK: sk}
}
