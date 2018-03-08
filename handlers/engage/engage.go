package engage

import (
	"context"
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
	return &handler{handlers.NewBaseHandler(courier.ChannelType("ED"), "Engage Direct")}
}

func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	return nil
}

func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	return status, nil
}
