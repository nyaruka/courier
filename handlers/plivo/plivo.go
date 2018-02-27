package plivo

/*
POST /handlers/plivo/status/uuid
Status=delivered&From=13342031111&ParentMessageUUID=83876bdb-2033-4876-bfaf-0ff8693705af&TotalRate=0.0025&MCC=405&PartInfo=1+of+1&ErrorCode=&To=918553651111&Units=1&TotalAmount=0.0025&MNC=803&MessageUUID=83876bdb-2033-4876-bfaf-0ff8693705af

POST /api/v1/plivo/receive/uuid
To=4759440448&From=4795961111&TotalRate=0&Units=1&Text=Msg&TotalAmount=0&Type=sms&MessageUUID=7a242edc-2f57-11e7-98c9-06ab0bf64327
*/

import (
	"context"
	"fmt"
	"net/http"
	"strings"

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
	return &handler{handlers.NewBaseHandler(courier.ChannelType("PL"), "Plivo")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	err := s.AddHandlerRoute(h, http.MethodPost, "status", h.receiveStatus)
	if err != nil {
		return err
	}
	return s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
}

type statusForm struct {
	From              string `name:"From" validate:"required"`
	To                string `name:"To" validate:"required"`
	MessageUUID       string `name:"MessageUUID" validate:"required"`
	Status            string `name:"Status" validate:"required"`
	ParentMessageUUID string `name:"ParentMessageUUID"`
}

var statusMapping = map[string]courier.MsgStatusValue{
	"queued":      courier.MsgWired,
	"delivered":   courier.MsgDelivered,
	"undelivered": courier.MsgSent,
	"sent":        courier.MsgSent,
	"rejected":    courier.MsgFailed,
}

// receiveStatus is our HTTP handler function for status updates
func (h *handler) receiveStatus(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	form := &statusForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	msgStatus, found := statusMapping[form.Status]
	if !found {
		return nil, courier.WriteAndLogRequestIgnored(ctx, w, r, channel, fmt.Sprintf("ignoring unknown status '%s'", form.Status))
	}

	if strings.TrimPrefix(channel.Address(), "+") != strings.TrimPrefix(form.To, "+") {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf("invalid to number [%s], expecting [%s]", strings.TrimPrefix(form.To, "+"), strings.TrimPrefix(channel.Address(), "+")))
	}

	externalID := form.MessageUUID
	if form.ParentMessageUUID != "" {
		externalID = form.ParentMessageUUID
	}

	// write our status
	status := h.Backend().NewMsgStatusForExternalID(channel, externalID, msgStatus)
	err = h.Backend().WriteMsgStatus(ctx, status)
	if err == courier.ErrMsgNotFound {
		return nil, courier.WriteAndLogStatusMsgNotFound(ctx, w, r, channel)
	}

	if err != nil {
		return nil, err
	}

	return []courier.Event{status}, courier.WriteStatusSuccess(ctx, w, r, []courier.MsgStatus{status})
}

type moForm struct {
	From        string `name:"From" validate:"required"`
	To          string `name:"To" validate:"required"`
	MessageUUID string `name:"MessageUUID" validate:"required"`
	Text        string `name:"Text"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	form := &moForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	if strings.TrimPrefix(channel.Address(), "+") != strings.TrimPrefix(form.To, "+") {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf("invalid to number [%s], expecting [%s]", strings.TrimPrefix(form.To, "+"), strings.TrimPrefix(channel.Address(), "+")))
	}

	// create our URN
	urn := urns.NewTelURNForCountry(form.From, channel.Country())

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, form.Text).WithExternalID(form.MessageUUID)

	// and write it
	err = h.Backend().WriteMsg(ctx, msg)
	if err != nil {
		return nil, err
	}
	return []courier.Event{msg}, courier.WriteMsgSuccess(ctx, w, r, []courier.Msg{msg})
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	return nil, fmt.Errorf("PL sending via courier not yet implemented")
}
