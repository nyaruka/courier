package africastalking

import (
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
)

func init() {
	courier.RegisterHandler(NewHandler())
}

type handler struct {
	handlers.BaseHandler
}

// NewHandler returns a new Dummy handler
func NewHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("DM"), "Dummy")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	return nil
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(msg courier.Msg) (*courier.MsgStatusUpdate, error) {
	time.Sleep(time.Second * 1)
	return nil, nil
}
