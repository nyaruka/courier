package shaqodoon

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/urns"
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

	urn, err := urns.ParsePhone(form.From, channel.Country(), true, false)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// create and write the message
	msg := h.Backend().NewIncomingMsg(ctx, channel, urn, form.Text, "", clog).WithReceivedOn(date)
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
}

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	sendURL := msg.Channel().StringConfigForKey(courier.ConfigSendURL, "")

	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if username == "" || password == "" || sendURL == "" {
		return courier.ErrChannelConfig
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
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, _, err := h.RequestHTTPInsecure(req, clog)
	if err != nil || resp.StatusCode/100 == 5 {
		return courier.ErrConnectionFailed
	} else if resp.StatusCode/100 != 2 {
		return courier.ErrResponseStatus
	}

	return nil
}
