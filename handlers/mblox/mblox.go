package mblox

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/urns"
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
	return s.AddHandlerRoute(h, http.MethodPost, "receive", h.ReceiveMessage)
}

type moPayload struct {
	Type       string `json:"type"`
	BatchID    string `json:"batch_id"`
	Status     string `json:"status"`
	ID         string `json:"id"`
	From       string `json:"from"`
	To         string `json:"to"`
	Body       string `json:"body"`
	ReceivedAt string `json:"received_at"`
}

var statusMapping = map[string]courier.MsgStatusValue{
	"Delivered":  courier.MsgDelivered,
	"Dispatched": courier.MsgSent,
	"Aborted":    courier.MsgFailed,
	"Rejected":   courier.MsgFailed,
	"Failed":     courier.MsgFailed,
	"Expired":    courier.MsgFailed,
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *handler) ReceiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	payload := &moPayload{}
	err := handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	if payload.Type == "recipient_delivery_report_sms" {
		if payload.BatchID == "" || payload.Status == "" {
			return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf("missing one of 'batch_id' or 'status' in request body"))
		}

		msgStatus, found := statusMapping[payload.Status]
		if !found {
			return nil, fmt.Errorf(`unknown status '%s', must be one of 'Delivered', 'Dispatched', 'Aborted', 'Rejected', 'Failed'  or 'Expired'`, payload.Status)
		}

		// write our status
		status := h.Backend().NewMsgStatusForExternalID(channel, payload.BatchID, msgStatus)
		err = h.Backend().WriteMsgStatus(ctx, status)
		if err == courier.ErrMsgNotFound {
			return nil, courier.WriteAndLogStatusMsgNotFound(ctx, w, r, channel)
		}

		if err != nil {
			return nil, err
		}

		return []courier.Event{status}, courier.WriteStatusSuccess(ctx, w, r, []courier.MsgStatus{status})

	} else if payload.Type == "mo_text" {
		if payload.ID == "" || payload.From == "" || payload.To == "" || payload.Body == "" || payload.ReceivedAt == "" {
			return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf("missing one of 'id', 'from', 'to', 'body' or 'received_at' in request body"))
		}

		date, err := time.Parse("2006-01-02T15:04:05.000Z", payload.ReceivedAt)
		if err != nil {
			return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
		}

		// create our URN
		urn := urns.NewTelURNForCountry(payload.From, channel.Country())

		// build our Message
		msg := h.Backend().NewIncomingMsg(channel, urn, payload.Body).WithReceivedOn(date.UTC()).WithExternalID(payload.ID)

		// and write it
		err = h.Backend().WriteMsg(ctx, msg)
		if err != nil {
			return nil, err
		}
		return []courier.Event{msg}, courier.WriteMsgSuccess(ctx, w, r, []courier.Msg{msg})

	}

	return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf("not handled, unknown type: %s", payload.Type))
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	return nil, fmt.Errorf("MB sending via courier not yet implemented")
}
