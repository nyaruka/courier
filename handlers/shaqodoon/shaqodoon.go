package shaqodoon

import (
	"context"
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
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("SQ"), "Shaqodoon")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
	return nil
}

type moForm struct {
	From string `name:"from" validate:"required"`
	Text string `name:"text"`
	Date string `name:"date"`
	Time string `name:"time"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	form := &moForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// if we have a date, parse it
	dateString := form.Date
	if dateString == "" {
		dateString = form.Time
	}

	date := time.Now()
	if dateString != "" {
		date, err = time.Parse(time.RFC3339Nano, dateString)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, errors.New("invalid date format, must be RFC 3339"))
		}
	}

	// create our URN
	urn, err := handlers.StrictTelForCountry(form.From, channel.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, form.Text).WithReceivedOn(date)
	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r)
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	sendURL := msg.Channel().StringConfigForKey(courier.ConfigSendURL, "")
	if sendURL == "" {
		return nil, fmt.Errorf("missing send_url for SQ channel")
	}

	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if username == "" || password == "" {
		return nil, fmt.Errorf("missing username or password for SQ channel")
	}

	// build our request
	form := url.Values{
		"from":     []string{strings.TrimPrefix(msg.Channel().Address(), "+")},
		"msg":      []string{handlers.GetTextAndAttachments(msg)},
		"to":       []string{strings.TrimPrefix(msg.URN().Path(), "+")},
		"username": []string{username},
		"password": []string{password},
	}

	encodedForm := form.Encode()
	sendURL = fmt.Sprintf("%s?%s", sendURL, encodedForm)

	req, _ := http.NewRequest(http.MethodGet, sendURL, nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr, err := utils.MakeInsecureHTTPRequest(req)

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	status.AddLog(courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err))
	if err == nil {
		status.SetStatus(courier.MsgWired)
	}
	return status, nil
}
