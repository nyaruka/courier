package dmark

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
)

var (
	sendURL      = "https://smsapi1.dmarkmobile.com/sms/"
	maxMsgLength = 453
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("DK"), "dmark")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodPost, "status", courier.ChannelLogTypeMsgStatus, h.receiveStatus)
	return nil
}

type moForm struct {
	MSISDN    string `validate:"required" name:"msisdn"`
	Text      string `validate:"required" name:"text"`
	ShortCode string `validate:"required" name:"short_code"`
	TStamp    string `validate:"required" name:"tstamp"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	// get our params
	form := &moForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// create our date from the timestamp "2017-10-26T15:51:32.906335+00:00"
	date, err := time.Parse("2006-01-02T15:04:05.999999-07:00", form.TStamp)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("invalid tstamp: %s", form.TStamp))
	}

	// create our URN
	urn, err := handlers.StrictTelForCountry(form.MSISDN, channel.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, form.Text, "", clog).WithReceivedOn(date)

	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
}

type statusForm struct {
	ID     string `validate:"required" name:"id"`
	Status string `validate:"required" name:"status"`
}

var statusMapping = map[string]courier.MsgStatus{
	"1":  courier.MsgStatusDelivered,
	"2":  courier.MsgStatusErrored,
	"4":  courier.MsgStatusSent,
	"8":  courier.MsgStatusSent,
	"16": courier.MsgStatusErrored,
}

// receiveStatus is our HTTP handler function for status updates
func (h *handler) receiveStatus(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	// get our params
	form := &statusForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	msgStatus, found := statusMapping[form.Status]
	if !found {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unknown status '%s', must be one of '1','2','4','8' or '16'", form.Status))
	}

	// write our status
	status := h.Backend().NewStatusUpdateByExternalID(channel, form.ID, msgStatus, clog)
	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	// get our authentication token
	auth := msg.Channel().StringConfigForKey(courier.ConfigAuthToken, "")
	if auth == "" {
		return nil, fmt.Errorf("no authorization token set for DK channel")
	}

	callbackDomain := msg.Channel().CallbackDomain(h.Server().Config().Domain)
	dlrURL := fmt.Sprintf("https://%s%s%s/status?id=%s&status=%%s", callbackDomain, "/c/dk/", msg.Channel().UUID(), msg.ID().String())

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)
	parts := handlers.SplitMsgByChannel(msg.Channel(), msg.Text(), maxMsgLength)
	for i, part := range parts {
		form := url.Values{
			"sender":   []string{strings.TrimLeft(msg.Channel().Address(), "+")},
			"receiver": []string{strings.TrimLeft(msg.URN().Path(), "+")},
			"text":     []string{part},
			"dlr_url":  []string{dlrURL},
		}

		req, err := http.NewRequest(http.MethodPost, sendURL, strings.NewReader(form.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Token %s", auth))

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 != 2 {
			return status, nil
		}

		// grab the external id
		externalID, err := jsonparser.GetString(respBody, "sms_id")
		if err != nil {
			clog.Error(courier.ErrorResponseValueMissing("sms_id"))
			return status, nil
		}

		// if this is our first message, record the external id
		if i == 0 {
			status.SetExternalID(externalID)
		}

		// this was wired successfully
		status.SetStatus(courier.MsgStatusWired)
	}

	return status, nil
}
