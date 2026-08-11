package whatsapp_legacy

import (
	"context"
	"net/http"

	"github.com/nyaruka/courier/v26"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
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
func (h *handler) Initialize(s *courier.Registry) error {
	s.AddHandlerRoute(h, http.MethodPost, "receive", models.ChannelLogTypeMultiReceive, h.receiveEvents)
	return nil
}

// receiveEvents accepts webhooks but does nothing with them
func (h *handler) receiveEvents(ctx context.Context, channel *models.Channel, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]courier.Event, error) {
	return nil, courier.WriteDataResponse(w, http.StatusOK, "Events Handled", []any{})
}

// Send is a noop - this legacy handler is disabled
func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *courier.SendResult, clog *models.ChannelLog) error {
	return courier.ErrFailedWithReason("disabled", "WhatsApp legacy handler is disabled")
}

// WriteRequestError writes the passed in error to our response writer
func (h *handler) WriteRequestError(ctx context.Context, w http.ResponseWriter, err error) error {
	return courier.WriteError(w, http.StatusOK, err)
}
