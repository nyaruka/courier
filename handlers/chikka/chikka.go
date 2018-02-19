package chikka

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/urns"
)

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("CK"), "Chikka")}
}

func init() {
	courier.RegisterHandler(newHandler())
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	return s.AddHandlerRoute(h, http.MethodPost, "receive", h.ReceiveMessage)
}

type moMsg struct {
	MessageType  string  `name:"message_type" validate:"required"`
	RequestID    string  `name:"request_id"`
	MobileNumber string  `name:"mobile_number"`
	Message      string  `name:"message"`
	Timestamp    float64 `name:"timestamp"`
	MessageID    int64   `name:"message_id"`
	Status       string  `name:"status"`
}

var statusMapping = map[string]courier.MsgStatusValue{
	"SENT":   courier.MsgSent,
	"FAILED": courier.MsgFailed,
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *handler) ReceiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	ckRequest := &moMsg{}
	err := handlers.DecodeAndValidateForm(ckRequest, r)
	if err != nil {
		return nil, err
	}

	if ckRequest.MessageType == "outgoing" {

		msgStatus, found := statusMapping[ckRequest.Status]
		if !found {
			return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf(`unknown status '%s', must be either 'SENT' or 'FAILED'`, ckRequest.Status))
		}

		// write our status
		status := h.Backend().NewMsgStatusForID(channel, courier.NewMsgID(ckRequest.MessageID), msgStatus)
		err = h.Backend().WriteMsgStatus(ctx, status)
		if err != nil {
			return nil, err
		}

		return []courier.Event{status}, courier.WriteStatusSuccess(ctx, w, r, []courier.MsgStatus{status})

	} else if ckRequest.MessageType == "incoming" {

		// create our date from the timestamp
		date := time.Unix(0, int64(ckRequest.Timestamp*1000000000)).UTC()

		// create our URN
		urn := urns.NewTelURNForCountry(ckRequest.MobileNumber, channel.Country())

		// build our msg
		msg := h.Backend().NewIncomingMsg(channel, urn, ckRequest.Message).WithExternalID(ckRequest.RequestID).WithReceivedOn(date)

		// and finally queue our message
		err = h.Backend().WriteMsg(ctx, msg)
		if err != nil {
			return nil, err
		}

		return []courier.Event{msg}, courier.WriteMsgSuccess(ctx, w, r, []courier.Msg{msg})
	} else {
		return nil, courier.WriteAndLogRequestIgnored(ctx, w, r, channel, "unknown message_type request")
	}
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	return nil, fmt.Errorf("CK sending via courier not yet implemented")
}
