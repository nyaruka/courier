package zenvia

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/pkg/errors"
)

var sendURL = "https://api-rest.zenvia360.com.br/services"

func init() {
	courier.RegisterHandler(NewHandler())
}

type handler struct {
	handlers.BaseHandler
}

// NewHandler returns a new Zenvia handler
func NewHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("ZV"), "Zenvia")}
}

// {
//     "callbackMoRequest": {
// 	    	"id": "20690090",
//         	"mobile": "555191951711",
//         	"shortCode": "40001",
//         	"account": "zenvia.envio",
//         	"body": "Content of reply SMS",
//         	"received": "2014-08-26T12:27:08.488-03:00",
//         	"correlatedMessageSmsId": "hs765939061"
//  	}
// }
type messageRequest struct {
	CallbackMORequest struct {
		ID         string `validate:"required" json:"id"`
		From       string `validate:"required" json:"mobile"`
		Text       string `validate:"required" json:"body"`
		Date       string `validate:"required" json:"received"`
		ExternalID string `validate:"required" json:"correlatedMessageSmsId"`
	} `json:"callbackMoRequest"`
}

// {
// 		"callbackMtRequest": {
//      	"status": "03",
//         	"statusMessage": "Delivered",
//         	"statusDetail": "120",
//         	"statusDetailMessage": "Message received by mobile",
//         	"id": "hs765939216",
//         	"received": "2014-08-26T12:55:48.593-03:00",
//         	"mobileOperatorName": "Claro"
// 		}
// }
type statusRequest struct {
	CallbackMTRequest struct {
		StatusCode string `validate:"required" json:"status"`
		ID         string `validate:"required" json:"id"`
	}
}

// {
//     "sendSmsRequest": {
//         "from": "Sender",
//         "to": "555199999999",
//         "schedule": "2014-08-22T14:55:00",
//         "msg": "Test message.",
//         "callbackOption": "NONE",
//         "id": "002",
//         "aggregateId": "1111"
//     }
// }
type zvOutgoingMsg struct {
	SendSMSRequest zvSendSMSRequest `json:"sendSmsRequest"`
}

type zvSendSMSRequest struct {
	From           string `validate:"required" json:"from"`
	To             string `validate:"required" json:"to"`
	Schedule       string `validate:"required" json:"schedule"`
	Msg            string `validate:"required" json:"msg"`
	CallbackOption string `validate:"required" json:"callbackOption"`
	ID             string `validate:"required" json:"id"`
	AggregateID    string `validate:"required" json:"aggregateId"`
}

var statusMapping = map[string]courier.MsgStatusValue{
	"00": courier.MsgSent,
	"01": courier.MsgSent,
	"02": courier.MsgSent,
	"03": courier.MsgDelivered,
	"04": courier.MsgErrored,
	"05": courier.MsgErrored,
	"06": courier.MsgErrored,
	"07": courier.MsgErrored,
	"08": courier.MsgErrored,
	"09": courier.MsgErrored,
	"10": courier.MsgErrored,
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	err := s.AddReceiveMsgRoute(h, "POST", "receive", h.ReceiveMessage)
	if err != nil {
		return err
	}
	return s.AddUpdateStatusRoute(h, "POST", "status", h.StatusMessage)
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *handler) ReceiveMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Msg, error) {
	// get our params
	zvMsg := &messageRequest{}
	err := handlers.DecodeAndValidateJSON(zvMsg, r)
	if err != nil {
		return nil, err
	}

	// create our date from the timestamp
	// 2017-05-03T06:04:45.345-03:00
	date, err := time.Parse("2006-01-02T15:04:05.000-07:00", zvMsg.CallbackMORequest.Date)
	if err != nil {
		return nil, fmt.Errorf("invalid date format: %s", zvMsg.CallbackMORequest.Date)
	}

	// create our URN
	urn := courier.NewTelURNForChannel(zvMsg.CallbackMORequest.From, channel)

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, zvMsg.CallbackMORequest.Text).WithExternalID(zvMsg.CallbackMORequest.ExternalID).WithReceivedOn(date.UTC())

	// and finally queue our message
	err = h.Backend().WriteMsg(msg)
	if err != nil {
		return nil, err
	}

	return []courier.Msg{msg}, courier.WriteReceiveSuccess(w, r, msg)
}

// StatusMessage is our HTTP handler function for status updates
func (h *handler) StatusMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.MsgStatus, error) {
	// get our params
	zvStatus := &statusRequest{}
	err := handlers.DecodeAndValidateJSON(zvStatus, r)
	if err != nil {
		return nil, err
	}

	msgStatus, found := statusMapping[zvStatus.CallbackMTRequest.StatusCode]
	if !found {
		msgStatus = courier.MsgErrored
	}

	// write our status
	status := h.Backend().NewMsgStatusForExternalID(channel, zvStatus.CallbackMTRequest.ID, msgStatus)
	err = h.Backend().WriteMsgStatus(status)
	if err != nil {
		return nil, err
	}

	return []courier.MsgStatus{status}, courier.WriteStatusSuccess(w, r, status)

}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(msg courier.Msg) (courier.MsgStatus, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for Zenvia channel")
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for Zenvia channel")
	}

	encodedCreds := utils.EncodeBase64([]string{username, ":", password})
	authHeader := "Basic " + encodedCreds

	zvMsg := zvOutgoingMsg{
		SendSMSRequest: zvSendSMSRequest{
			From:           "Sender",
			To:             strings.TrimLeft(msg.URN().Path(), "+"),
			Schedule:       "",
			Msg:            courier.GetTextAndAttachments(msg),
			ID:             msg.ID().String(),
			CallbackOption: strconv.Itoa(1),
			AggregateID:    "",
		},
	}

	requestBody := new(bytes.Buffer)
	json.NewEncoder(requestBody).Encode(zvMsg)

	// build our request
	req, err := http.NewRequest(http.MethodPost, sendURL, requestBody)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", authHeader)
	rr, err := utils.MakeHTTPRequest(req)

	// record our status and log
	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	status.AddLog(courier.NewChannelLogFromRR(msg.Channel(), msg.ID(), rr))
	if err != nil {
		return status, err
	}

	// was this request successful?
	responseMsgStatus, _ := jsonparser.GetString([]byte(rr.Body), "sendSmsResponse", "statusCode")
	msgStatus, found := statusMapping[responseMsgStatus]
	if msgStatus == courier.MsgErrored || !found {
		return status, errors.Errorf("received non-success response from Zenvia '%s'", responseMsgStatus)
	}
	status.SetStatus(courier.MsgWired)

	return status, nil

}
