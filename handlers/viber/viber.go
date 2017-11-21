package viber

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/pkg/errors"
)

/*
POST /handlers/viber_public/uuid?sig=sig
{"event":"delivered","timestamp":1493817791212,"message_token":504054678623710111,"user_id":"Iul/YIu1tJwyRWKkx7Pxyw=="}

POST /handlers/viber_public/uuid?sig=sig
{"event":"message","timestamp":1493823965629,"message_token":50405727362920111,"sender":{"id":"7nulzrc62mo4kiirIg==","name":"User name","avatar":"https://avatar.jpg","language":"th","country":"HK","api_version":2}

POST /handlers/viber_public/uuid?sig=sig
{"event":"message","timestamp":1493814248770,"message_token":50405319809731111,"sender":{"id":"iu7u0ekVY01115lOIg==","name":"User name","avatar":"https://avatar.jpg","language":"en","country":"PK","api_version":2},"message":{"text":"Msg","type":"text","tracking_data":"579777865"},"silent":false}
*/

var sendURL = "https://chatapi.viber.com/pa/send_message"

func init() {
	courier.RegisterHandler(NewHandler())
}

type handler struct {
	handlers.BaseHandler
}

// NewHandler returns a new Infobip handler
func NewHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("VP"), "Viber")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	return nil
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(msg courier.Msg) (courier.MsgStatus, error) {
	confAuth := msg.Channel().ConfigForKey(courier.ConfigAuthToken, "")
	authToken, isStr := confAuth.(string)
	if !isStr || authToken == "" {
		return nil, fmt.Errorf("invalid auth token config")
	}

	viberMsg := viberOutgoingMessage{
		AuthToken:    authToken,
		Receiver:     msg.URN().Path(),
		Text:         courier.GetTextAndAttachments(msg),
		Type:         "text",
		TrackingData: msg.ID().String(),
	}

	requestBody := &bytes.Buffer{}
	err := json.NewEncoder(requestBody).Encode(viberMsg)
	if err != nil {
		return nil, err
	}

	// build our request
	req, err := http.NewRequest(http.MethodPost, sendURL, requestBody)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	rr, err := utils.MakeHTTPRequest(req)

	// record our status and log
	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr)
	status.AddLog(log)
	if err != nil {
		log.WithError("Message Send Error", err)
		return status, nil
	}

	responseStatus, err := jsonparser.GetInt([]byte(rr.Body), "status")
	if err != nil {
		log.WithError("Message Send Error", errors.Errorf("received invalid JSON response"))
		status.SetStatus(courier.MsgFailed)
		return status, nil
	}
	if responseStatus != 0 {
		log.WithError("Message Send Error", errors.Errorf("received non-0 status: '%d'", responseStatus))
		status.SetStatus(courier.MsgFailed)
		return status, nil
	}

	status.SetStatus(courier.MsgWired)
	return status, nil
}

type viberOutgoingMessage struct {
	AuthToken    string `json:"auth_token"`
	Receiver     string `json:"receiver"`
	Text         string `json:"text"`
	Type         string `json:"type"`
	TrackingData string `json:"tracking_data"`
}
