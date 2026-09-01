package africastalking

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

const configIsShared = "is_shared"

var defaultSendURL = "https://api.africastalking.com/version1/messaging"

func init() {
	channels.RegisterHandler(newHandler)
}

type handler struct {
	handlers.BaseHandler
}

func newHandler(rt *runtime.Runtime, r *channels.Routes) channels.Handler {
	h := &handler{handlers.NewBaseHandler(rt, models.ChannelType("AT"), "Africas Talking")}

	r.AddReceive(h, http.MethodPost, "receive", channels.ReceiveKindMsg, handlers.FormPayload(h.receiveMessage))
	r.AddReceive(h, http.MethodPost, "callback", channels.ReceiveKindMsg, handlers.FormPayload(h.receiveMessage))
	r.AddReceive(h, http.MethodPost, "delivery", channels.ReceiveKindStatus, handlers.FormPayload(h.receiveStatus))
	r.AddReceive(h, http.MethodPost, "status", channels.ReceiveKindStatus, handlers.FormPayload(h.receiveStatus))
	return h
}

type moForm struct {
	ID   string `name:"id"   validate:"required"`
	Text string `name:"text" validate:"required"`
	From string `name:"from" validate:"required"`
	To   string `name:"to"   validate:"required"`
	Date string `name:"date" validate:"required"`
}

// receiveMessage is our receive function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel *models.Channel, r *http.Request, form *moForm, in *channels.Received, clog *models.ChannelLog) error {
	// create our date from the timestamp
	// 2017-05-03T06:04:45Z
	date, err := time.Parse("2006-01-02T15:04:05Z", form.Date)
	if err != nil {
		date, err = time.Parse("2006-01-02 15:04:05", form.Date)
		if err != nil {
			return fmt.Errorf("invalid date format: %s", form.Date)
		}
		date = date.UTC()
	}

	// create our URN
	urn, err := urns.ParsePhone(form.From, channel.Country(), true, false)
	if err != nil {
		return err
	}
	// build our msg
	msg := models.NewIncomingMsg(channel, urn, form.Text, form.ID, clog).WithReceivedOn(date)

	in.Msg(msg)
	return nil
}

type statusForm struct {
	ID     string `name:"id"     validate:"required"`
	Status string `name:"status" validate:"required"`
}

var statusMapping = map[string]models.MsgStatus{
	"Success":  models.MsgStatusDelivered,
	"Sent":     models.MsgStatusSent,
	"Buffered": models.MsgStatusSent,
	"Rejected": models.MsgStatusFailed,
	"Failed":   models.MsgStatusFailed,
	"Expired":  models.MsgStatusFailed,
}

// receiveStatus is our receive function for status updates
func (h *handler) receiveStatus(ctx context.Context, channel *models.Channel, r *http.Request, form *statusForm, in *channels.Received, clog *models.ChannelLog) error {
	msgStatus, found := statusMapping[form.Status]
	if !found {
		return handlers.UnknownStatusError(statusMapping, form.Status)
	}

	status := models.NewStatusUpdateByExternalID(channel, form.ID, msgStatus, clog)
	in.Status(status)
	return nil
}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	isSharedStr := msg.Channel().ConfigForKey(configIsShared, false)
	isShared, _ := isSharedStr.(bool)

	username := msg.Channel().StringConfigForKey(models.ConfigUsername, "")
	apiKey := msg.Channel().StringConfigForKey(models.ConfigAPIKey, "")

	sendURL := msg.Channel().StringConfigForKey(models.ConfigSendURL, defaultSendURL)
	if username == "" || apiKey == "" || sendURL == "" {
		return channels.ErrChannelConfig
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
	if err := handlers.ErrorFromResponse(resp, err); err != nil {
		return err
	}

	// was this request successful?
	msgStatus, _ := jsonparser.GetString(respBody, "SMSMessageData", "Recipients", "[0]", "status")
	if msgStatus != "Success" {
		return channels.ErrResponseContent
	}

	// grab the external id if we can
	externalID, _ := jsonparser.GetString(respBody, "SMSMessageData", "Recipients", "[0]", "messageId")
	if externalID != "" {
		res.AddExternalID(externalID)
	}

	return nil
}
