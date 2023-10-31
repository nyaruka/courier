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

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
)

var (
	sendURL      = "https://api.mblox.com/xms/v1"
	maxMsgLength = 459
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("MB"), "Mblox")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeUnknown, handlers.JSONPayload(h, h.receiveEvent))
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

var statusMapping = map[string]courier.MsgStatus{
	"Delivered":  courier.MsgStatusDelivered,
	"Dispatched": courier.MsgStatusSent,
	"Aborted":    courier.MsgStatusFailed,
	"Rejected":   courier.MsgStatusFailed,
	"Failed":     courier.MsgStatusFailed,
	"Expired":    courier.MsgStatusFailed,
}

// receiveEvent is our HTTP handler function for incoming messages
func (h *handler) receiveEvent(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *eventPayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	if payload.Type == "recipient_delivery_report_sms" {
		clog.SetType(courier.ChannelLogTypeMsgStatus)

		if payload.BatchID == "" || payload.Status == "" {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("missing one of 'batch_id' or 'status' in request body"))
		}

		msgStatus, found := statusMapping[payload.Status]
		if !found {
			return nil, fmt.Errorf(`unknown status '%s', must be one of 'Delivered', 'Dispatched', 'Aborted', 'Rejected', 'Failed'  or 'Expired'`, payload.Status)
		}

		// write our status
		status := h.Backend().NewStatusUpdateByExternalID(channel, payload.BatchID, msgStatus, clog)
		return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)

	} else if payload.Type == "mo_text" {
		clog.SetType(courier.ChannelLogTypeMsgReceive)

		if payload.ID == "" || payload.From == "" || payload.To == "" || payload.Body == "" || payload.ReceivedAt == "" {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("missing one of 'id', 'from', 'to', 'body' or 'received_at' in request body"))
		}

		date, err := time.Parse("2006-01-02T15:04:05.000Z", payload.ReceivedAt)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}

		// create our URN
		urn, err := handlers.StrictTelForCountry(payload.From, channel.Country())
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}

		// build our Message
		msg := h.Backend().NewIncomingMsg(channel, urn, payload.Body, payload.ID, clog).WithReceivedOn(date.UTC())

		// and finally write our message
		return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
	}

	return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("not handled, unknown type: %s", payload.Type))
}

type mtPayload struct {
	From           string   `json:"from"`
	To             []string `json:"to"`
	Body           string   `json:"body"`
	DeliveryReport string   `json:"delivery_report"`
}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if username == "" || password == "" {
		return nil, fmt.Errorf("Missing username or password for MB channel")
	}

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)
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
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", password))

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 != 2 {
			return status, nil
		}

		externalID, err := jsonparser.GetString(respBody, "id")
		if err != nil {
			return status, fmt.Errorf("unable to parse response body from MBlox")
		}

		status.SetStatus(courier.MsgStatusWired)
		status.SetExternalID(externalID)
	}

	return status, nil
}
