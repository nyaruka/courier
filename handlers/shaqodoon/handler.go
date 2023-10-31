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
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
	return nil
}

type moForm struct {
	From string `name:"from" validate:"required"`
	Text string `name:"text"`
	Date string `name:"date"`
	Time string `name:"time"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
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

	// Shaqodoon doesn't encode the "+" when sending us numbers with country codes. This throws off our URN parsing backdown of using
	// the numbers as is for numbers phonenumbers doesn't recognize. Fix this on our side before passing to our parsing lib.
	if strings.HasPrefix(form.From, " ") {
		form.From = "+" + form.From[1:]
	}

	urn, err := handlers.StrictTelForCountry(form.From, channel.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// create and write the message
	msg := h.Backend().NewIncomingMsg(channel, urn, form.Text, "", clog).WithReceivedOn(date)
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
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

	req, err := http.NewRequest(http.MethodGet, sendURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)

	resp, _, err := h.RequestHTTPInsecure(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return status, nil
	}

	status.SetStatus(courier.MsgStatusWired)
	return status, nil
}
