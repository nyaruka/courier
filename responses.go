package courier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/nyaruka/gocommon/urns"
	validator "gopkg.in/go-playground/validator.v9"
)

// writeAndLogRequestError writes a JSON response for the passed in message and logs an info messages
func writeAndLogRequestError(ctx context.Context, h ChannelHandler, w http.ResponseWriter, r *http.Request, c Channel, err error) error {
	LogRequestError(r, c, err)
	return h.WriteRequestError(ctx, w, err)
}

// WriteError writes a JSON response for the passed in error
func WriteError(ctx context.Context, w http.ResponseWriter, statusCode int, err error) error {
	errors := []interface{}{NewErrorData(err.Error())}

	vErrs, isValidation := err.(validator.ValidationErrors)
	if isValidation {
		for i := range vErrs {
			errors = append(errors, NewErrorData(fmt.Sprintf("field '%s' %s", strings.ToLower(vErrs[i].Field()), vErrs[i].Tag())))
		}
	}
	return WriteDataResponse(ctx, w, statusCode, "Error", errors)
}

// WriteIgnored writes a JSON response indicating that we ignored the request
func WriteIgnored(ctx context.Context, w http.ResponseWriter, details string) error {
	return WriteDataResponse(ctx, w, http.StatusOK, "Ignored", []interface{}{NewInfoData(details)})
}

// WriteAndLogUnauthorized writes a JSON response for the passed in message and logs an info message
func WriteAndLogUnauthorized(ctx context.Context, w http.ResponseWriter, r *http.Request, c Channel, err error) error {
	LogRequestError(r, c, err)
	return WriteDataResponse(ctx, w, http.StatusUnauthorized, "Unauthorized", []interface{}{NewErrorData(err.Error())})
}

// WriteChannelEventSuccess writes a JSON response for the passed in event indicating we handled it
func WriteChannelEventSuccess(ctx context.Context, w http.ResponseWriter, event ChannelEvent) error {
	return WriteDataResponse(ctx, w, http.StatusOK, "Event Accepted", []interface{}{NewEventReceiveData(event)})
}

// WriteMsgSuccess writes a JSON response for the passed in msg indicating we handled it
func WriteMsgSuccess(ctx context.Context, w http.ResponseWriter, msgs []Msg) error {
	data := []interface{}{}
	for _, msg := range msgs {
		data = append(data, NewMsgReceiveData(msg))
	}

	return WriteDataResponse(ctx, w, http.StatusOK, "Message Accepted", data)
}

// WriteStatusSuccess writes a JSON response for the passed in status update indicating we handled it
func WriteStatusSuccess(ctx context.Context, w http.ResponseWriter, statuses []MsgStatus) error {
	data := []interface{}{}
	for _, status := range statuses {
		data = append(data, NewStatusData(status))
	}

	return WriteDataResponse(ctx, w, http.StatusOK, "Status Update Accepted", data)
}

// WriteDataResponse writes a JSON formatted response with the passed in status code, message and data
func WriteDataResponse(ctx context.Context, w http.ResponseWriter, statusCode int, message string, data []interface{}) error {
	return writeJSONResponse(w, statusCode, &dataResponse{message, data})
}

// MsgReceiveData is our response payload for a received message
type MsgReceiveData struct {
	Type        string      `json:"type"`
	ChannelUUID ChannelUUID `json:"channel_uuid"`
	MsgUUID     MsgUUID     `json:"msg_uuid"`
	Text        string      `json:"text"`
	URN         urns.URN    `json:"urn"`
	Attachments []string    `json:"attachments,omitempty"`
	ExternalID  string      `json:"external_id,omitempty"`
	ReceivedOn  *time.Time  `json:"received_on,omitempty"`
}

// NewMsgReceiveData creates a new data response for the passed in msg parameters
func NewMsgReceiveData(msg Msg) MsgReceiveData {
	return MsgReceiveData{
		"msg",
		msg.Channel().UUID(),
		msg.UUID(),
		msg.Text(),
		msg.URN(),
		msg.Attachments(),
		msg.ExternalID(),
		msg.ReceivedOn(),
	}
}

// EventReceiveData is our response payload for a channel event
type EventReceiveData struct {
	Type        string                 `json:"type"`
	ChannelUUID ChannelUUID            `json:"channel_uuid"`
	EventType   ChannelEventType       `json:"event_type"`
	URN         urns.URN               `json:"urn"`
	ReceivedOn  time.Time              `json:"received_on"`
	Extra       map[string]interface{} `json:"extra,omitempty"`
}

// NewEventReceiveData creates a new receive data for the passed in event
func NewEventReceiveData(event ChannelEvent) EventReceiveData {
	return EventReceiveData{
		"event",
		event.ChannelUUID(),
		event.EventType(),
		event.URN(),
		event.OccurredOn(),
		event.Extra(),
	}
}

// StatusData is our response payload for a status update
type StatusData struct {
	Type        string         `json:"type"`
	ChannelUUID ChannelUUID    `json:"channel_uuid"`
	Status      MsgStatusValue `json:"status"`
	MsgID       MsgID          `json:"msg_id,omitempty"`
	ExternalID  string         `json:"external_id,omitempty"`
}

// NewStatusData creates a new status data object for the passed in status
func NewStatusData(status MsgStatus) StatusData {
	return StatusData{
		"status",
		status.ChannelUUID(),
		status.Status(),
		status.ID(),
		status.ExternalID(),
	}
}

// ErrorData is our response payload for an error
type ErrorData struct {
	Type  string `json:"type"`
	Error string `json:"error"`
}

// NewErrorData creates a new data segment for the passed in error string
func NewErrorData(err string) ErrorData {
	return ErrorData{"error", err}
}

// InfoData is our response payload for an informational message
type InfoData struct {
	Type string `json:"type"`
	Info string `json:"info"`
}

// NewInfoData creates a new data segment for the passed in info string
func NewInfoData(info string) InfoData {
	return InfoData{"info", info}
}

type dataResponse struct {
	Message string        `json:"message"`
	Data    []interface{} `json:"data"`
}

func writeJSONResponse(w http.ResponseWriter, statusCode int, response interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	return json.NewEncoder(w).Encode(response)
}

func writeTextResponse(w http.ResponseWriter, text string) {
	var buf bytes.Buffer
	buf.WriteString("<html><head><title>courier</title></head><body><pre>\n")
	buf.WriteString(text)
	buf.WriteString("</pre></body></html>")
	w.Write(buf.Bytes())
}
