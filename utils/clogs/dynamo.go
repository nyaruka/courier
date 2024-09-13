package clogs

import (
	"bytes"
	"compress/gzip"
	"time"

	"github.com/nyaruka/gocommon/jsonx"
)

const (
	dynamoTTL = 14 * 24 * time.Hour
)

// DynamoLog channel log to be written to DynamoDB
type DynamoLog struct {
	UUID      LogUUID   `dynamodbav:"UUID"`
	Type      LogType   `dynamodbav:"Type"`
	DataGZ    []byte    `dynamodbav:"DataGZ,omitempty"`
	ElapsedMS int       `dynamodbav:"ElapsedMS"`
	CreatedOn time.Time `dynamodbav:"CreatedOn,unixtime"`
	ExpiresOn time.Time `dynamodbav:"ExpiresOn,unixtime"`
}

func NewDynamoLog(l *Log) *DynamoLog {
	data := jsonx.MustMarshal(map[string]any{"http_logs": l.HttpLogs, "errors": l.Errors})
	buf := &bytes.Buffer{}
	w := gzip.NewWriter(buf)
	w.Write(data)
	w.Close()

	return &DynamoLog{
		UUID:      l.UUID,
		Type:      l.Type,
		DataGZ:    buf.Bytes(),
		ElapsedMS: int(l.Elapsed / time.Millisecond),
		CreatedOn: l.CreatedOn,
		ExpiresOn: l.CreatedOn.Add(dynamoTTL),
	}
}
