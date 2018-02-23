package macrokiosk

import (
	"context"
	"fmt"
	"github.com/nyaruka/gocommon/urns"
	"net/http"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("MK"), "Macrokiosk")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	err := s.AddHandlerRoute(h, http.MethodPost, "status", h.receiveStatus)
	if err != nil {
		return err
	}
	err = s.AddHandlerRoute(h, http.MethodGet, "status", h.receiveStatus)
	if err != nil {
		return err
	}
	err = s.AddHandlerRoute(h, http.MethodGet, "receive", h.receiveMessage)
	if err != nil {
		return err
	}
	return s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
}

type statusForm struct {
	MsgID  string `name:"msgid" validate:"required"`
	Status string `name:"status" validate:"required"`
}

var statusMapping = map[string]courier.MsgStatusValue{
	"ACCEPTED":    courier.MsgSent,
	"DELIVERED":   courier.MsgDelivered,
	"UNDELIVERED": courier.MsgFailed,
	"PROCESSING":  courier.MsgWired,
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
	// write our status
	status := h.Backend().NewMsgStatusForExternalID(channel, form.MsgID, msgStatus)
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
	Longcode  string `name:"longcode"`
	Shortcode string `name:"shortcode"`
	MSISDN    string `name:"msisdn"`
	From      string `name:"from"`
	Text      string `name:"text"`
	MsgID     string `name:"msgId"`
	Time      string `name:"time"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	form := &moForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	recipient := form.Longcode
	sender := form.MSISDN
	if form.Shortcode != "" {
		recipient = form.Shortcode
		sender = form.From
	}

	if recipient == "" || sender == "" {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf("missing shortcode, longcode, from or msisdn parameters"))
	}

	if channel.Address() != recipient {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf("invalid to number [%s], expecting [%s]", recipient, channel.Address()))
	}

	loc, err := time.LoadLocation("Asia/Kuala_Lumpur")
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	date, err := time.ParseInLocation("2006-01-0215:04:05", form.Time, loc)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	// create our URN
	urn := urns.NewTelURNForCountry(sender, channel.Country())

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, form.Text).WithExternalID(form.MsgID).WithReceivedOn(date.UTC())

	// and write it
	err = h.Backend().WriteMsg(ctx, msg)
	if err != nil {
		return nil, err
	}
	courier.LogMsgReceived(r, msg)
	return []courier.Event{msg}, h.writeReceiveSuccess(ctx, w, r, "-1") // MacroKiosk expects "-1" back for successful requests
}

func (h *handler) writeReceiveSuccess(ctx context.Context, w http.ResponseWriter, r *http.Request, responseText string) error {
	w.WriteHeader(200)
	_, err := fmt.Fprint(w, responseText)
	return err
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	return nil, fmt.Errorf("MK sending via courier not yet implemented")
}
