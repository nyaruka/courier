package courier

import (
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
func WriteError(w http.ResponseWriter, statusCode int, err error) error {
	errors := []any{NewErrorData(err.Error())}

	vErrs, isValidation := err.(validator.ValidationErrors)
	if isValidation {
		for i := range vErrs {
			errors = append(errors, NewErrorData(fmt.Sprintf("field '%s' %s", strings.ToLower(vErrs[i].Field()), vErrs[i].Tag())))
		}
	}
	return WriteDataResponse(w, statusCode, "Error", errors)
}

// WriteIgnored writes a JSON response indicating that we ignored the request
func WriteIgnored(w http.ResponseWriter, details string) error {
	return WriteDataResponse(w, http.StatusOK, "Ignored", []any{NewInfoData(details)})
}

// WriteAndLogUnauthorized writes a JSON response for the passed in message and logs an info message
func WriteAndLogUnauthorized(w http.ResponseWriter, r *http.Request, c Channel, err error) error {
	LogRequestError(r, c, err)
	return WriteDataResponse(w, http.StatusUnauthorized, "Unauthorized", []any{NewErrorData(err.Error())})
}

// WriteChannelEventSuccess writes a JSON response for the passed in event indicating we handled it
func WriteChannelEventSuccess(w http.ResponseWriter, event ChannelEvent) error {
	return WriteDataResponse(w, http.StatusOK, "Event Accepted", []any{NewEventReceiveData(event)})
}

// WriteMsgSuccess writes a JSON response for the passed in msg indicating we handled it
func WriteMsgSuccess(w http.ResponseWriter, msgs []MsgIn) error {
	data := []any{}
	for _, msg := range msgs {
		data = append(data, NewMsgReceiveData(msg))
	}

	return WriteDataResponse(w, http.StatusOK, "Message Accepted", data)
}

// WriteStatusSuccess writes a JSON response for the passed in status update indicating we handled it
func WriteStatusSuccess(w http.ResponseWriter, statuses []StatusUpdate) error {
	data := []any{}
	for _, status := range statuses {
		data = append(data, NewStatusData(status))
	}

	return WriteDataResponse(w, http.StatusOK, "Status Update Accepted", data)
}

// WriteDataResponse writes a JSON formatted response with the passed in status code, message and data
func WriteDataResponse(w http.ResponseWriter, statusCode int, message string, data []any) error {
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
func NewMsgReceiveData(msg MsgIn) MsgReceiveData {
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
	Type        string            `json:"type"`
	ChannelUUID ChannelUUID       `json:"channel_uuid"`
	EventType   ChannelEventType  `json:"event_type"`
	URN         urns.URN          `json:"urn"`
	ReceivedOn  time.Time         `json:"received_on"`
	Extra       map[string]string `json:"extra,omitempty"`
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
	Type        string      `json:"type"`
	ChannelUUID ChannelUUID `json:"channel_uuid"`
	Status      MsgStatus   `json:"status"`
	MsgID       MsgID       `json:"msg_id,omitempty"`
	ExternalID  string      `json:"external_id,omitempty"`
}

// NewStatusData creates a new status data object for the passed in status
func NewStatusData(status StatusUpdate) StatusData {
	return StatusData{
		"status",
		status.ChannelUUID(),
		status.Status(),
		status.MsgID(),
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
	Message string `json:"message"`
	Data    []any  `json:"data"`
}

func writeJSONResponse(w http.ResponseWriter, statusCode int, response any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	return json.NewEncoder(w).Encode(response)
}
