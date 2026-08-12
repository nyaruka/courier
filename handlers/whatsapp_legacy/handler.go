package whatsapp_legacy

import (
	"context"
	"net/http"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
)

const (
	channelTypeWa  = "WA"
	channelTypeD3  = "D3"
	channelTypeTXW = "TXW"
)

func init() {
	channels.RegisterHandler(newWAHandler(models.ChannelType(channelTypeWa), "WhatsApp"))
	channels.RegisterHandler(newWAHandler(models.ChannelType(channelTypeD3), "360Dialog"))
	channels.RegisterHandler(newWAHandler(models.ChannelType(channelTypeTXW), "TextIt"))
}

type handler struct {
	handlers.BaseHandler
}

func newWAHandler(channelType models.ChannelType, name string) channels.Handler {
	return &handler{handlers.NewBaseHandler(channelType, name)}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(r *channels.Routes) error {
	r.Add(h, http.MethodPost, "receive", models.ChannelLogTypeMultiReceive, h.receiveEvents)
	return nil
}

// receiveEvents accepts webhooks but does nothing with them
func (h *handler) receiveEvents(ctx context.Context, channel *models.Channel, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
	return nil, channels.WriteDataResponse(w, http.StatusOK, "Events Handled", []any{})
}

// Send is a noop - this legacy handler is disabled
func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	return channels.ErrFailedWithReason("disabled", "WhatsApp legacy handler is disabled")
}

// WriteRequestError writes the passed in error to our response writer
func (h *handler) WriteRequestError(ctx context.Context, w http.ResponseWriter, err error) error {
	return channels.WriteError(w, http.StatusOK, err)
}
