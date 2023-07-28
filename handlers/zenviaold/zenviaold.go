package zenvia_old

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/pkg/errors"
)

var (
	maxMsgLength = 1152
	sendURL      = "https://api-rest.zenvia.com/services/send-sms"
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("ZV"), "Zenvia")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, handlers.JSONPayload(h, h.receiveMessage))
	s.AddHandlerRoute(h, http.MethodPost, "status", courier.ChannelLogTypeMsgStatus, handlers.JSONPayload(h, h.receiveStatus))
	return nil
}

//	{
//	    "callbackMoRequest": {
//		    	"id": "20690090",
//	        	"mobile": "555191951711",
//	        	"shortCode": "40001",
//	        	"account": "zenvia.envio",
//	        	"body": "Content of reply SMS",
//	        	"received": "2014-08-26T12:27:08.488-03:00",
//	        	"correlatedMessageSmsId": "hs765939061"
//	 	}
//	}
type moPayload struct {
	CallbackMORequest struct {
		ID         string `json:"id"                      validate:"required" `
		From       string `json:"mobile"                  validate:"required" `
		Text       string `json:"body"`
		Date       string `json:"received"                validate:"required" `
		ExternalID string `json:"correlatedMessageSmsId"`
	} `json:"callbackMoRequest"`
}

//	{
//			"callbackMtRequest": {
//	     	"status": "03",
//	        	"statusMessage": "Delivered",
//	        	"statusDetail": "120",
//	        	"statusDetailMessage": "Message received by mobile",
//	        	"id": "hs765939216",
//	        	"received": "2014-08-26T12:55:48.593-03:00",
//	        	"mobileOperatorName": "Claro"
//			}
//	}
type statusPayload struct {
	CallbackMTRequest struct {
		StatusCode string `json:"status" validate:"required"`
		ID         string `json:"id"     validate:"required" `
	}
}

//	{
//	    "sendSmsRequest": {
//	        "to": "555199999999",
//	        "schedule": "2014-08-22T14:55:00",
//	        "msg": "Test message.",
//	        "callbackOption": "NONE",
//	        "id": "002",
//	        "aggregateId": "1111"
//	    }
//	}
type mtPayload struct {
	SendSMSRequest struct {
		To             string `json:"to"`
		Schedule       string `json:"schedule"`
		Msg            string `json:"msg"`
		CallbackOption string `json:"callbackOption"`
		ID             string `json:"id"`
		AggregateID    string `json:"aggregateId"`
	} `json:"sendSmsRequest"`
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

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *moPayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	// create our date from the timestamp
	// 2017-05-03T06:04:45.345-03:00
	date, err := time.Parse("2006-01-02T15:04:05.000-07:00", payload.CallbackMORequest.Date)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("invalid date format: %s", payload.CallbackMORequest.Date))
	}

	// create our URN
	urn, err := handlers.StrictTelForCountry(payload.CallbackMORequest.From, channel.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, payload.CallbackMORequest.Text, payload.CallbackMORequest.ID, clog).WithReceivedOn(date.UTC())
	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r, clog)
}

// receiveStatus is our HTTP handler function for status updates
func (h *handler) receiveStatus(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *statusPayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	msgStatus, found := statusMapping[payload.CallbackMTRequest.StatusCode]
	if !found {
		msgStatus = courier.MsgErrored
	}

	// write our status
	status := h.Backend().NewMsgStatusForExternalID(channel, payload.CallbackMTRequest.ID, msgStatus, clog)
	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)

}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.Msg, clog *courier.ChannelLog) (courier.MsgStatus, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for ZV channel")
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for ZV channel")
	}

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored, clog)
	parts := handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength)
	for _, part := range parts {
		zvMsg := mtPayload{}
		zvMsg.SendSMSRequest.To = strings.TrimLeft(msg.URN().Path(), "+")
		zvMsg.SendSMSRequest.Msg = part
		zvMsg.SendSMSRequest.ID = msg.ID().String()
		zvMsg.SendSMSRequest.CallbackOption = "FINAL"

		requestBody := new(bytes.Buffer)
		json.NewEncoder(requestBody).Encode(zvMsg)

		// build our request
		req, err := http.NewRequest(http.MethodPost, sendURL, requestBody)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.SetBasicAuth(username, password)

		resp, respBody, err := handlers.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 != 2 {
			return status, nil
		}

		// was this request successful?
		responseMsgStatus, _ := jsonparser.GetString(respBody, "sendSmsResponse", "statusCode")
		msgStatus, found := statusMapping[responseMsgStatus]
		if msgStatus == courier.MsgErrored || !found {
			clog.RawError(errors.Errorf("received non-success response: '%s'", responseMsgStatus))
			return status, nil
		}

		status.SetStatus(courier.MsgWired)
	}
	return status, nil
}

func (h *handler) RedactValues(ch courier.Channel) []string {
	return []string{
		httpx.BasicAuth(ch.StringConfigForKey(courier.ConfigUsername, ""), ch.StringConfigForKey(courier.ConfigPassword, "")),
	}
}
