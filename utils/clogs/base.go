package clogs

import (
	"fmt"
	"time"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/stringsx"
	"github.com/nyaruka/gocommon/uuids"
)

// LogUUID is the type of a channel log UUID (should be v7)
type LogUUID uuids.UUID

// NewLogUUID creates a new channel log UUID
func NewLogUUID() LogUUID {
	return LogUUID(uuids.NewV7())
}

type LogType string

// Error is an error that occurred during a channel interaction
type LogError struct {
	Code    string `json:"code"`
	ExtCode string `json:"ext_code,omitempty"`
	Message string `json:"message"`
}

// NewLogError creates a new log error
func NewLogError(code, extCode, message string, args ...any) *LogError {
	return &LogError{Code: code, ExtCode: extCode, Message: fmt.Sprintf(message, args...)}
}

// Redact applies the given redactor to this error
func (e *LogError) Redact(r stringsx.Redactor) *LogError {
	return &LogError{Code: e.Code, ExtCode: e.ExtCode, Message: r(e.Message)}
}

// Log is the basic channel log structure
type Log struct {
	UUID      LogUUID
	Type      LogType
	HttpLogs  []*httpx.Log
	Errors    []*LogError
	CreatedOn time.Time
	Elapsed   time.Duration

	recorder *httpx.Recorder
	redactor stringsx.Redactor
}

func NewLog(t LogType, r *httpx.Recorder, redactVals []string) *Log {
	return &Log{
		UUID:      NewLogUUID(),
		Type:      t,
		HttpLogs:  []*httpx.Log{},
		Errors:    []*LogError{},
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
func (l *Log) Error(e *LogError) {
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
