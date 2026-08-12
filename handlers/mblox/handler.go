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

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(r *channels.Routes) error {
	r.Add(h, http.MethodPost, "receive", models.ChannelLogTypeUnknown, handlers.JSONPayload(h, h.receiveEvent))
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

// receiveEvent is our HTTP handler function for incoming messages
func (h *handler) receiveEvent(ctx context.Context, channel *models.Channel, w http.ResponseWriter, r *http.Request, payload *eventPayload, clog *models.ChannelLog) ([]channels.Event, error) {
	if payload.Type == "recipient_delivery_report_sms" {
		clog.Type = models.ChannelLogTypeMsgStatus

		if payload.BatchID == "" || payload.Status == "" {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("missing one of 'batch_id' or 'status' in request body"))
		}

		msgStatus, found := statusMapping[payload.Status]
		if !found {
			return nil, fmt.Errorf(`unknown status '%s', must be one of 'Delivered', 'Dispatched', 'Aborted', 'Rejected', 'Failed'  or 'Expired'`, payload.Status)
		}

		// write our status
		status := models.NewStatusUpdateByExternalID(channel, payload.BatchID, msgStatus, clog)
		return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)

	} else if payload.Type == "mo_text" {
		clog.Type = models.ChannelLogTypeMsgReceive

		if payload.ID == "" || payload.From == "" || payload.To == "" || payload.Body == "" || payload.ReceivedAt == "" {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("missing one of 'id', 'from', 'to', 'body' or 'received_at' in request body"))
		}

		date, err := time.Parse("2006-01-02T15:04:05.000Z", payload.ReceivedAt)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}

		// create our URN
		urn, err := urns.ParsePhone(payload.From, channel.Country(), true, false)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}

		// build our Message
		msg := models.NewIncomingMsg(channel, urn, payload.Body, payload.ID, clog).WithReceivedOn(date.UTC())

		// and finally write our message
		return handlers.WriteMsgsAndResponse(ctx, h, []*models.MsgIn{msg}, w, r, clog)
	}

	return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("not handled, unknown type: %s", payload.Type))
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
