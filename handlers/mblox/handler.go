package mblox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/buger/jsonparser"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/gocommon/urns"
)

var (
	sendURL      = "https://api.mblox.com/xms/v1"
	maxMsgLength = 459
)

func init() {
	channels.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() channels.Handler {
	return &handler{handlers.NewBaseHandler(models.ChannelType("MB"), "Mblox")}
}

// Initialize registers the routes this handler serves
func (h *handler) Initialize(r *channels.Routes) error {
	r.AddReceive(h, http.MethodPost, "receive", models.ChannelLogTypeUnknown, handlers.JSONPayload(h.receiveEvent))
	return nil
}

type eventPayload struct {
	Type       string `json:"type"       validate:"required"`
	BatchID    string `json:"batch_id"`
	Status     string `json:"status"`
	ID         string `json:"id"`
	From       string `json:"from"`
	To         string `json:"to"`
	Body       string `json:"body"`
	ReceivedAt string `json:"received_at"`
}

var statusMapping = map[string]models.MsgStatus{
	"Delivered":  models.MsgStatusDelivered,
	"Dispatched": models.MsgStatusSent,
	"Aborted":    models.MsgStatusFailed,
	"Rejected":   models.MsgStatusFailed,
	"Failed":     models.MsgStatusFailed,
	"Expired":    models.MsgStatusFailed,
}

// receiveEvent is our receive function for incoming messages
func (h *handler) receiveEvent(ctx context.Context, channel *models.Channel, r *http.Request, payload *eventPayload, in *channels.Received, clog *models.ChannelLog) error {
	if payload.Type == "recipient_delivery_report_sms" {
		in.As(models.ChannelLogTypeMsgStatus)

		if payload.BatchID == "" || payload.Status == "" {
			return fmt.Errorf("missing one of 'batch_id' or 'status' in request body")
		}

		msgStatus, found := statusMapping[payload.Status]
		if !found {
			return handlers.UnknownStatusError(statusMapping, payload.Status)
		}

		in.Status(models.NewStatusUpdateByExternalID(channel, payload.BatchID, msgStatus, clog))
		return nil

	} else if payload.Type == "mo_text" {
		in.As(models.ChannelLogTypeMsgReceive)

		if payload.ID == "" || payload.From == "" || payload.To == "" || payload.Body == "" || payload.ReceivedAt == "" {
			return fmt.Errorf("missing one of 'id', 'from', 'to', 'body' or 'received_at' in request body")
		}

		date, err := time.Parse("2006-01-02T15:04:05.000Z", payload.ReceivedAt)
		if err != nil {
			return err
		}

		urn, err := urns.ParsePhone(payload.From, channel.Country(), true, false)
		if err != nil {
			return err
		}

		in.Msg(models.NewIncomingMsg(channel, urn, payload.Body, payload.ID, clog).WithReceivedOn(date.UTC()))
		return nil
	}

	return fmt.Errorf("not handled, unknown type: %s", payload.Type)
}

type mtPayload struct {
	From           string   `json:"from"`
	To             []string `json:"to"`
	Body           string   `json:"body"`
	DeliveryReport string   `json:"delivery_report"`
}

func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {

	username := msg.Channel().StringConfigForKey(models.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(models.ConfigPassword, "")
	if username == "" || password == "" {
		return channels.ErrChannelConfig
	}
	parts := handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength)
	for _, part := range parts {
		payload := &mtPayload{}
		payload.From = strings.TrimPrefix(msg.Channel().Address(), "+")
		payload.To = []string{strings.TrimPrefix(msg.URN().Path(), "+")}
		payload.Body = part
		payload.DeliveryReport = "per_recipient"

		requestBody := &bytes.Buffer{}
		json.NewEncoder(requestBody).Encode(payload)

		// build our request
		req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/%s/batches", sendURL, username), requestBody)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", password))

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err := handlers.ErrorFromResponse(resp, err); err != nil {
			return err
		}

		externalID, err := jsonparser.GetString(respBody, "id")
		if err != nil {
			clog.Error(models.ErrorResponseValueMissing("id"))
		} else {
			res.AddExternalID(externalID)
		}
	}

	return nil
}
