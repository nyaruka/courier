package smscentral

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/pkg/errors"
)

/*
POST /handlers/smscentral/receive/uuid/
mobile=9779811781111&message=Msg
*/

var sendURL = "http://smail.smscentral.com.np/bp/ApiSms.php"

func init() {
	courier.RegisterHandler(NewHandler())
}

type handler struct {
	handlers.BaseHandler
}

// NewHandler returns a new Yo! handler
func NewHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("SC"), "SMS Central")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	err := s.AddReceiveMsgRoute(h, "POST", "receive", h.ReceiveMessage)
	if err != nil {
		return err
	}

	return nil
}

type smsCentralMessage struct {
	Message string `validate:"required" name:"message"`
	Mobile  string `validate:"required" name:"mobile"`
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *handler) ReceiveMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Msg, error) {
	smsCentralMessage := &smsCentralMessage{}
	handlers.DecodeAndValidateQueryParams(smsCentralMessage, r)

	// if this is a post, also try to parse the form body
	if r.Method == http.MethodPost {
		handlers.DecodeAndValidateForm(smsCentralMessage, r)
	}

	// validate whether our required fields are present
	err := handlers.Validate(smsCentralMessage)
	if err != nil {
		return nil, err
	}

	// create our URN
	urn := courier.NewTelURNForChannel(smsCentralMessage.Mobile, channel)

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, smsCentralMessage.Message)

	// and finally queue our message
	err = h.Backend().WriteMsg(msg)
	if err != nil {
		return nil, err
	}

	return []courier.Msg{msg}, courier.WriteReceiveSuccess(w, r, msg)

}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(msg courier.Msg) (courier.MsgStatus, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for SC channel")
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for SC channel")
	}

	// build our request
	form := url.Values{
		"user":    []string{username},
		"pass":    []string{password},
		"mobile":  []string{strings.TrimPrefix(msg.URN().Path(), "+")},
		"content": []string{courier.GetTextAndAttachments(msg)},
	}

	req, err := http.NewRequest(http.MethodPost, sendURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr, err := utils.MakeHTTPRequest(req)

	// record our status and log
	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	status.AddLog(courier.NewChannelLogFromRR(msg.Channel(), msg.ID(), rr, err))
	if err != nil {
		return status, err
	}

	if rr.StatusCode/100 != 2 {
		return status, errors.Errorf("Got non-200 response [%d] from API")
	}

	status.SetStatus(courier.MsgWired)

	return status, nil
}
