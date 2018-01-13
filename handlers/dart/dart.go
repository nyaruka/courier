package dart

/*
GET /handlers/dartmedia/received/uuid?userid=username&password=xxxxxxxx&original=6285218761111&sendto=93456&messagetype=0&messageid=170503131327@170504131327@93456SMS9755064&message=Msg&date=20170503131559&dcs=0&udhl=0&charset=utf-8
*/

import (
	"strconv"
	"github.com/nyaruka/gocommon/urns"
	"fmt"
	"context"
	"net/http"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
)

type handler struct {
	handlers.BaseHandler
}

// NewHandler returns a new DartMedia ready to be registered
func NewHandler(channelType string, name string) courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType(channelType), name)}
}

func init() {
	courier.RegisterHandler(NewHandler("DA", "DartMedia"))
	courier.RegisterHandler(NewHandler("H9", "Hub9"))
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	err := s.AddHandlerRoute(h, http.MethodGet, "receive", h.ReceiveMessage)
	if err != nil {
		return err
	}

	err = s.AddHandlerRoute(h, http.MethodGet, "received", h.ReceiveMessage)
	if err != nil {
		return err
	}

	return s.AddHandlerRoute(h, http.MethodGet, "delivered", h.ReceiveMessage)
}

type dartStatus struct {
	MessageID string `name:"messageid"`
	Status string `name:"status"`
}


type dartMessage struct {
	Message string `name:"message"`
	From string `name:"original"`
	To string `name:"sendto"`
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *handler) ReceiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	daMessage := &dartMessage{}
	err := handlers.DecodeAndValidateForm(daMessage, r)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	// create our URN
	urn := urns.NewTelURNForCountry(daMessage.From, channel.Country())

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, daMessage.Message)

	// and finally queue our message
	err = h.Backend().WriteMsg(ctx, msg)
	if err != nil {
		return nil, err
	}

	return []courier.Event{msg}, h.writeReceiveSuccess(ctx, w, r, msg)
}

// StatusMessage is our HTTP handler function for status updates
func (h *handler) StatusMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	daStatus := &dartStatus{}
	err := handlers.DecodeAndValidateForm(daStatus, r)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	if daStatus.Status == "" {
		return nil, h.writeStasusSuccess(ctx, w, r, nil)
	}

	statusInt, err := strconv.Atoi(daStatus.Status)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	msgStatus := courier.MsgSent
	if statusInt >= 10 && statusInt <= 12 {
		msgStatus = courier.MsgDelivered
	}

	if statusInt > 20 {
		msgStatus = courier.MsgFailed
	}

	msgID, err := strconv.ParseInt(daStatus.MessageID, 10, 64)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	// write our status
	status := h.Backend().NewMsgStatusForID(channel, courier.NewMsgID(msgID), msgStatus)
	err = h.Backend().WriteMsgStatus(ctx, status)
	if err != nil {
		return nil, err
	}

	return []courier.Event{status}, h.writeStasusSuccess(ctx, w, r, status)
}

// DartMedia expects "000" from a message receive request
func (h *handler) writeReceiveSuccess(ctx context.Context, w http.ResponseWriter, r *http.Request, msg courier.Msg) error {
	courier.LogMsgReceived(r, msg)
	w.WriteHeader(200)
	_, err := fmt.Fprint(w, "000")
	return err
}

// DartMedia expects "000" from a status request
func (h *handler) writeStasusSuccess(ctx context.Context, w http.ResponseWriter, r *http.Request, status courier.MsgStatus) error {
	courier.LogMsgStatusReceived(r, status)
	w.WriteHeader(200)
	_, err := fmt.Fprint(w, "000")
	return err
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	return nil, fmt.Errorf("DA sending via Courier not yet implemented")
}