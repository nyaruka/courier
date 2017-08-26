package courier

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	validator "gopkg.in/go-playground/validator.v9"
)

// WriteError writes a JSON response for the passed in error
func WriteError(w http.ResponseWriter, r *http.Request, err error) error {
	errors := []string{err.Error()}

	vErrs, isValidation := err.(validator.ValidationErrors)
	if isValidation {
		errors = []string{}
		for i := range vErrs {
			errors = append(errors, fmt.Sprintf("field '%s' %s", strings.ToLower(vErrs[i].Field()), vErrs[i].Tag()))
		}
	}
	return writeJSONResponse(w, http.StatusBadRequest, &errorResponse{errors})
}

// WriteIgnored writes a JSON response for the passed in message
func WriteIgnored(w http.ResponseWriter, r *http.Request, details string) error {
	logrus.WithFields(logrus.Fields{
		"url":        r.Context().Value(contextRequestURL),
		"elapsed_ms": getElapsedMS(r),
		"details":    details,
	}).Info("msg ignored")
	return writeData(w, http.StatusOK, details, struct{}{})
}

// WriteReceiveSuccess writes a JSON response for the passed in msg indicating we handled it
func WriteReceiveSuccess(w http.ResponseWriter, r *http.Request, msg Msg) error {
	logrus.WithFields(logrus.Fields{
		"url":             r.Context().Value(contextRequestURL),
		"elapsed_ms":      getElapsedMS(r),
		"channel_uuid":    msg.Channel().UUID(),
		"msg_uuid":        msg.UUID(),
		"msg_id":          msg.ID().Int64,
		"msg_urn":         msg.URN().Identity(),
		"msg_text":        msg.Text(),
		"msg_attachments": msg.Attachments(),
	}).Info("msg received")
	return writeData(w, http.StatusOK, "Message Accepted",
		&receiveData{
			msg.Channel().UUID(),
			msg.UUID(),
			msg.Text(),
			msg.URN(),
			msg.Attachments(),
			msg.ExternalID(),
			msg.ReceivedOn(),
		})
}

// WriteStatusSuccess writes a JSON response for the passed in status update indicating we handled it
func WriteStatusSuccess(w http.ResponseWriter, r *http.Request, status MsgStatus) error {
	log := logrus.WithFields(logrus.Fields{
		"url":          r.Context().Value(contextRequestURL),
		"elapsed_ms":   getElapsedMS(r),
		"channel_uuid": status.ChannelUUID(),
	})

	if status.ID() != NilMsgID {
		log = log.WithField("msg_id", status.ID().Int64)
	} else {
		log = log.WithField("msg_external_id", status.ExternalID())
	}
	log.Info("status updated")

	return writeData(w, http.StatusOK, "Status Update Accepted",
		&statusData{
			status.ChannelUUID(),
			status.Status(),
			status.ID(),
			status.ExternalID(),
		})
}

func getElapsedMS(r *http.Request) float64 {
	start := r.Context().Value(contextRequestStart)
	if start == nil {
		return -1
	}
	startTime, isTime := start.(time.Time)
	if !isTime {
		return -1
	}
	return float64(time.Now().Sub(startTime)) / float64(time.Millisecond)
}

type errorResponse struct {
	Errors []string `json:"errors"`
}

type successResponse struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

type receiveData struct {
	ChannelUUID ChannelUUID `json:"channel_uuid"`
	MsgUUID     MsgUUID     `json:"msg_uuid"`
	Text        string      `json:"text"`
	URN         URN         `json:"urn"`
	Attachments []string    `json:"attachments,omitempty"`
	ExternalID  string      `json:"external_id,omitempty"`
	ReceivedOn  *time.Time  `json:"received_on,omitempty"`
}

type statusData struct {
	ChannelUUID ChannelUUID    `json:"channel_uuid"`
	Status      MsgStatusValue `json:"status"`
	MsgID       MsgID          `json:"msg_id,omitempty"`
	ExternalID  string         `json:"external_id,omitempty"`
}

func writeJSONResponse(w http.ResponseWriter, statusCode int, response interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	return json.NewEncoder(w).Encode(response)
}

func writeData(w http.ResponseWriter, statusCode int, message string, response interface{}) error {
	return writeJSONResponse(w, statusCode, &successResponse{message, response})
}
