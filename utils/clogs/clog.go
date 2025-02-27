package clogs

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/nyaruka/gocommon/aws/dynamo"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/stringsx"
	"github.com/nyaruka/gocommon/uuids"
)

const (
	dynamoTTL = 7 * 24 * time.Hour // 1 week
)

// UUID is the type of a channel log UUID (should be v7)
type UUID uuids.UUID

// NewUUID creates a new channel log UUID
func NewUUID() UUID {
	return UUID(uuids.NewV7())
}

type Type string

// Error is an error that occurred during a channel interaction
type Error struct {
	Code    string `json:"code"`
	ExtCode string `json:"ext_code,omitempty"`
	Message string `json:"message"`
}

// Redact applies the given redactor to this error
func (e *Error) Redact(r stringsx.Redactor) *Error {
	return &Error{Code: e.Code, ExtCode: e.ExtCode, Message: r(e.Message)}
}

// Log is the basic channel log structure
type Log struct {
	UUID      UUID
	Type      Type
	HttpLogs  []*httpx.Log
	Errors    []*Error
	CreatedOn time.Time
	Elapsed   time.Duration

	recorder *httpx.Recorder
	redactor stringsx.Redactor
}

func New(t Type, r *httpx.Recorder, redactVals []string) *Log {
	return &Log{
		UUID:      NewUUID(),
		Type:      t,
		HttpLogs:  []*httpx.Log{},
		Errors:    []*Error{},
		CreatedOn: time.Now(),

		recorder: r,
		redactor: stringsx.NewRedactor("**********", redactVals...),
	}
}

// HTTP adds the given HTTP trace to this log
func (l *Log) HTTP(t *httpx.Trace) {
	l.HttpLogs = append(l.HttpLogs, l.traceToLog(t))
}

// Error adds the given error to this log
func (l *Log) Error(e *Error) {
	l.Errors = append(l.Errors, e.Redact(l.redactor))
}

// End finalizes this log
func (l *Log) End() {
	if l.recorder != nil {
		// prepend so it's the first HTTP request in the log
		l.HttpLogs = append([]*httpx.Log{l.traceToLog(l.recorder.Trace)}, l.HttpLogs...)
	}

	l.Elapsed = time.Since(l.CreatedOn)
}

func (l *Log) traceToLog(t *httpx.Trace) *httpx.Log {
	return httpx.NewLog(t, 2048, 50000, l.redactor)
}

// log struct to be written to DynamoDB
type dynamoLog struct {
	UUID      UUID      `dynamodbav:"UUID"`
	Type      Type      `dynamodbav:"Type"`
	DataGZ    []byte    `dynamodbav:"DataGZ,omitempty"`
	ElapsedMS int       `dynamodbav:"ElapsedMS"`
	CreatedOn time.Time `dynamodbav:"CreatedOn,unixtime"`
	ExpiresOn time.Time `dynamodbav:"ExpiresOn,unixtime"`
}

type dynamoLogData struct {
	HttpLogs []*httpx.Log `json:"http_logs"`
	Errors   []*Error     `json:"errors"`
}

func (l *Log) MarshalDynamo() (map[string]types.AttributeValue, error) {
	data, err := dynamo.MarshalJSONGZ(&dynamoLogData{HttpLogs: l.HttpLogs, Errors: l.Errors})
	if err != nil {
		return nil, fmt.Errorf("error marshaling log data: %w", err)
	}

	return attributevalue.MarshalMap(&dynamoLog{
		UUID:      l.UUID,
		Type:      l.Type,
		DataGZ:    data,
		ElapsedMS: int(l.Elapsed / time.Millisecond),
		CreatedOn: l.CreatedOn,
		ExpiresOn: l.CreatedOn.Add(dynamoTTL),
	})
}

func (l *Log) UnmarshalDynamo(m map[string]types.AttributeValue) error {
	d := &dynamoLog{}

	if err := attributevalue.UnmarshalMap(m, d); err != nil {
		return fmt.Errorf("error unmarshaling log: %w", err)
	}

	data := &dynamoLogData{}
	if err := dynamo.UnmarshalJSONGZ(d.DataGZ, data); err != nil {
		return fmt.Errorf("error unmarshaling log data: %w", err)
	}

	l.UUID = d.UUID
	l.Type = d.Type
	l.HttpLogs = data.HttpLogs
	l.Errors = data.Errors
	l.Elapsed = time.Duration(d.ElapsedMS) * time.Millisecond
	l.CreatedOn = d.CreatedOn
	return nil
}
