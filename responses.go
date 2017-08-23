package courier

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/pressly/lg"
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
	} else {
		lg.Log(r.Context()).WithError(err).Error()
	}
	return writeJSONResponse(w, http.StatusBadRequest, &errorResponse{errors})
}

// WriteIgnored writes a JSON response for the passed in message
func WriteIgnored(w http.ResponseWriter, r *http.Request, message string) error {
	lg.Log(r.Context()).Info("msg ignored")
	return writeData(w, http.StatusOK, message, struct{}{})
}

// WriteReceiveSuccess writes a JSON response for the passed in msg indicating we handled it
func WriteReceiveSuccess(w http.ResponseWriter, r *http.Request, msg Msg) error {
	lg.Log(r.Context()).WithFields(logrus.Fields{
		"channel_uuid":    msg.Channel().UUID(),
		"msg_uuid":        msg.UUID(),
		"msg_id":          msg.ID().Int64,
		"msg_urn":         msg.URN().Identity(),
		"msg_text":        msg.Text(),
		"msg_attachments": msg.Attachments(),
	}).Info("msg received")
	return writeData(w, http.StatusOK, "Message Accepted", &receiveData{msg.UUID()})
}

// WriteStatusSuccess writes a JSON response for the passed in status update indicating we handled it
func WriteStatusSuccess(w http.ResponseWriter, r *http.Request, status MsgStatus) error {
	if status.ID() != NilMsgID {
		lg.Log(r.Context()).WithField("channel_uuid", status.ChannelUUID()).WithField("msg_id", status.ID().Int64).Info("status updated")
	} else {
		lg.Log(r.Context()).WithField("channel_uuid", status.ChannelUUID()).WithField("msg_external_id", status.ExternalID()).Info("status updated")
	}

	return writeData(w, http.StatusOK, "Status Update Accepted", &statusData{status.Status()})
}

type errorResponse struct {
	Text []string `json:"errors"`
}

type successResponse struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

type receiveData struct {
	UUID MsgUUID `json:"uuid"`
}

type statusData struct {
	Status MsgStatusValue `json:"status"`
}

func writeJSONResponse(w http.ResponseWriter, statusCode int, response interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	return json.NewEncoder(w).Encode(response)
}

func writeData(w http.ResponseWriter, statusCode int, message string, response interface{}) error {
	return writeJSONResponse(w, statusCode, &successResponse{message, response})
}
