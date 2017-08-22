package shaqodoon

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/pkg/errors"
)

/*
POST /api/v1/shaqodoon/received/uuid/
from=252634101111&text=Msg
*/

func init() {
	courier.RegisterHandler(NewHandler())
}

type handler struct {
	handlers.BaseHandler
}

// NewHandler returns a new External handler
func NewHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("SQ"), "Shaqodoon")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddReceiveMsgRoute(h, "POST", "receive", h.ReceiveMessage)
	return nil
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *handler) ReceiveMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Msg, error) {
	shaqodoonMessage := &shaqodoonMessage{}
	handlers.DecodeAndValidateQueryParams(shaqodoonMessage, r)

	// if this is a post, also try to parse the form body
	if r.Method == http.MethodPost {
		handlers.DecodeAndValidateForm(shaqodoonMessage, r)
	}

	// validate whether our required fields are present
	err := handlers.Validate(shaqodoonMessage)
	if err != nil {
		return nil, err
	}

	// must have one of from or sender set, error if neither
	sender := shaqodoonMessage.From
	if sender == "" {
		return nil, errors.New("must have one of 'sender' or 'from' set")
	}

	// if we have a date, parse it
	dateString := shaqodoonMessage.Date
	if dateString == "" {
		dateString = shaqodoonMessage.Time
	}

	date := time.Now()
	if dateString != "" {
		date, err = time.Parse(time.RFC3339Nano, dateString)
		if err != nil {
			return nil, errors.New("invalid date format, must be RFC 3339")
		}
	}

	// create our URN
	urn := courier.NewTelURNForChannel(sender, channel)
	if err != nil {
		return nil, err
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, shaqodoonMessage.Text).WithReceivedOn(date)

	// and write it
	err = h.Backend().WriteMsg(msg)
	if err != nil {
		return nil, err
	}

	return []courier.Msg{msg}, courier.WriteReceiveSuccess(w, r, msg)
}

type shaqodoonMessage struct {
	From string `name:"from"`
	Text string `validate:"required" name:"text"`
	Date string `name:"date"`
	Time string `name:"time"`
}

// buildStatusHandler deals with building a handler that takes what status is received in the URL
func (h *handler) buildStatusHandler(status string) courier.ChannelUpdateStatusFunc {
	return func(channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.MsgStatus, error) {
		return h.StatusMessage(status, channel, w, r)
	}
}

// StatusMessage is our HTTP handler function for status updates
func (h *handler) StatusMessage(statusString string, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.MsgStatus, error) {
	statusForm := &statusForm{}
	handlers.DecodeAndValidateQueryParams(statusForm, r)

	// if this is a post, also try to parse the form body
	if r.Method == http.MethodPost {
		handlers.DecodeAndValidateForm(statusForm, r)
	}

	// validate whether our required fields are present
	err := handlers.Validate(statusForm)
	if err != nil {
		return nil, err
	}

	// get our id
	msgStatus, found := statusMappings[strings.ToLower(statusString)]
	if !found {
		return nil, fmt.Errorf("unknown status '%s', must be one failed, sent or delivered", statusString)
	}

	// write our status
	status := h.Backend().NewMsgStatusForID(channel, courier.NewMsgID(statusForm.ID), msgStatus)
	err = h.Backend().WriteMsgStatus(status)
	if err != nil {
		return nil, err
	}

	return []courier.MsgStatus{status}, courier.WriteStatusSuccess(w, r, status)
}

type statusForm struct {
	ID int64 `name:"id" validate:"required"`
}

var statusMappings = map[string]courier.MsgStatusValue{
	"failed":    courier.MsgFailed,
	"sent":      courier.MsgSent,
	"delivered": courier.MsgDelivered,
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(msg courier.Msg) (courier.MsgStatus, error) {
	sendURL := msg.Channel().StringConfigForKey(courier.ConfigSendURL, "")
	if sendURL == "" {
		return nil, fmt.Errorf("no send url set for SQ channel")
	}

	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for SQ channel")
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for SQ channel")
	}

	// build our request
	form := url.Values{
		"from":     []string{strings.TrimPrefix(msg.Channel().Address(), "+")},
		"msg":      []string{courier.GetTextAndAttachments(msg)},
		"to":       []string{strings.TrimPrefix(msg.URN().Path(), "+")},
		"username": []string{username},
		"password": []string{password},
	}

	encodedForm := form.Encode()
	sendURL = fmt.Sprintf("%s?%s", sendURL, encodedForm)

	req, err := http.NewRequest(http.MethodGet, sendURL, nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr, err := utils.MakeHTTPRequest(req)

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	status.AddLog(courier.NewChannelLogFromRR(msg.Channel(), msg.ID(), rr))
	if err != nil {
		return status, err
	}

	status.SetStatus(courier.MsgWired)
	return status, nil
}
