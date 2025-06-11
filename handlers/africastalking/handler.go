package africastalking

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
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

const configIsShared = "is_shared"

var sendURL = "https://api.africastalking.com/version1/messaging"

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("AT"), "Africas Talking")}
}

type moForm struct {
	ID   string `validate:"required" name:"id"`
	Text string `validate:"required" name:"text"`
	From string `validate:"required" name:"from"`
	To   string `validate:"required" name:"to"`
	Date string `validate:"required" name:"date"`
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodPost, "callback", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodPost, "delivery", courier.ChannelLogTypeMsgStatus, h.receiveStatus)
	s.AddHandlerRoute(h, http.MethodPost, "status", courier.ChannelLogTypeMsgStatus, h.receiveStatus)
	return nil
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	// get our params
	form := &moForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// create our date from the timestamp
	// 2017-05-03T06:04:45Z
	date, err := time.Parse("2006-01-02T15:04:05Z", form.Date)
	if err != nil {
		date, err = time.Parse("2006-01-02 15:04:05", form.Date)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("invalid date format: %s", form.Date))
		}
		date = date.UTC()
	}

	// create our URN
	urn, err := urns.ParsePhone(form.From, channel.Country(), true, false)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}
	// build our msg
	msg := h.Backend().NewIncomingMsg(ctx, channel, urn, form.Text, form.ID, clog).WithReceivedOn(date)

	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
}

type statusForm struct {
	ID     string `validate:"required" name:"id"`
	Status string `validate:"required" name:"status"`
}

var statusMapping = map[string]courier.MsgStatus{
	"Success":  courier.MsgStatusDelivered,
	"Sent":     courier.MsgStatusSent,
	"Buffered": courier.MsgStatusSent,
	"Rejected": courier.MsgStatusFailed,
	"Failed":   courier.MsgStatusFailed,
	"Expired":  courier.MsgStatusFailed,
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
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r,
			fmt.Errorf("unknown status '%s', must be one of 'Success','Sent','Buffered','Rejected', 'Failed', or 'Expired'", form.Status))
	}

	// write our status
	status := h.Backend().NewStatusUpdateByExternalID(channel, form.ID, msgStatus, clog)
	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	isSharedStr := msg.Channel().ConfigForKey(configIsShared, false)
	isShared, _ := isSharedStr.(bool)

	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	apiKey := msg.Channel().StringConfigForKey(courier.ConfigAPIKey, "")

	if username == "" || apiKey == "" {
		return courier.ErrChannelConfig
	}

	// build our request
	form := url.Values{
		"username": []string{username},
		"to":       []string{msg.URN().Path()},
		"message":  []string{handlers.GetTextAndAttachments(msg)},
	}

	// if this isn't shared, include our from
	if !isShared {
		form["from"] = []string{msg.Channel().Address()}
	}

	req, err := httpx.NewRequest(ctx, http.MethodPost, sendURL, strings.NewReader(form.Encode()), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Accept":       "application/json",
		"apikey":       apiKey,
	})
	if err != nil {
		return err
	}

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 == 5 {
		return courier.ErrConnectionFailed
	} else if resp.StatusCode/100 != 2 {
		return courier.ErrResponseStatus
	}

	// was this request successful?
	msgStatus, _ := jsonparser.GetString(respBody, "SMSMessageData", "Recipients", "[0]", "status")
	if msgStatus != "Success" {
		return courier.ErrResponseContent
	}

	// grab the external id if we can
	externalID, _ := jsonparser.GetString(respBody, "SMSMessageData", "Recipients", "[0]", "messageId")
	if externalID != "" {
		res.AddExternalID(externalID)
	}

	return nil
}
