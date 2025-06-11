package clogs

import (
	"time"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/stringsx"
	"github.com/nyaruka/gocommon/uuids"
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

// if we have an error or a non 2XX/3XX http response then log is considered an error
func (l *Log) IsError() bool {
	if len(l.Errors) > 0 {
		return true
	}

	for _, l := range l.HttpLogs {
		if l.StatusCode < 200 || l.StatusCode >= 400 {
			return true
		}
	}

	return false
}

func (l *Log) traceToLog(t *httpx.Trace) *httpx.Log {
	return httpx.NewLog(t, 2048, 50000, l.redactor)
}
