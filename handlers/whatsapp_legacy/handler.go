package whatsapp_legacy

import (
	"context"
	"net/http"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/courier/handlers"
)

const (
	channelTypeWa  = "WA"
	channelTypeD3  = "D3"
	channelTypeTXW = "TXW"
)

func init() {
	courier.RegisterHandler(newWAHandler(models.ChannelType(channelTypeWa), "WhatsApp"))
	courier.RegisterHandler(newWAHandler(models.ChannelType(channelTypeD3), "360Dialog"))
	courier.RegisterHandler(newWAHandler(models.ChannelType(channelTypeTXW), "TextIt"))
}

type handler struct {
	handlers.BaseHandler
}

func newWAHandler(channelType models.ChannelType, name string) courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(channelType, name)}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMultiReceive, h.receiveEvents)
	return nil
}

// receiveEvents accepts webhooks but does nothing with them
func (h *handler) receiveEvents(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	return nil, courier.WriteDataResponse(w, http.StatusOK, "Events Handled", []any{})
}

// Send is a noop - this legacy handler is disabled
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	return courier.ErrFailedWithReason("disabled", "WhatsApp legacy handler is disabled")
}

// WriteRequestError writes the passed in error to our response writer
func (h *handler) WriteRequestError(ctx context.Context, w http.ResponseWriter, err error) error {
	return courier.WriteError(w, http.StatusOK, err)
}
