package courier

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	validator "gopkg.in/go-playground/validator.v9"
)

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
	Status MsgStatus `json:"status"`
}

func writeResponse(w http.ResponseWriter, statusCode int, response interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	return json.NewEncoder(w).Encode(response)
}

func writeData(w http.ResponseWriter, statusCode int, message string, response interface{}) error {
	return writeResponse(w, statusCode, &successResponse{message, response})
}

func WriteError(w http.ResponseWriter, err error) error {
	errors := []string{err.Error()}

	vErrs, isValidation := err.(validator.ValidationErrors)
	if isValidation {
		errors = []string{}
		for i := range vErrs {
			errors = append(errors, fmt.Sprintf("field '%s' %s", strings.ToLower(vErrs[i].Field()), vErrs[i].Tag()))
		}
	}
	return writeResponse(w, http.StatusBadRequest, &errorResponse{errors})
}

func WriteIgnored(w http.ResponseWriter, message string) error {
	return writeData(w, http.StatusOK, message, struct{}{})
}

func WriteReceiveSuccess(w http.ResponseWriter, msg Msg) error {
	return writeData(w, http.StatusOK, "Message Accepted", &receiveData{msg.UUID()})
}

func WriteStatusSuccess(w http.ResponseWriter, status MsgStatusUpdate) error {
	return writeData(w, http.StatusOK, "Status Update Accepted", &statusData{status.Status()})
}

func stringArrayContains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}
