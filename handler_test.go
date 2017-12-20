package courier

import "context"

func init() {
	RegisterHandler(NewHandler())
}

type dummyHandler struct {
	server  Server
	backend Backend
}

// NewHandler returns a new Dummy handler
func NewHandler() ChannelHandler {
	return &dummyHandler{}
}

func (h *dummyHandler) ChannelName() string      { return "Dummy Handler" }
func (h *dummyHandler) ChannelType() ChannelType { return ChannelType("DM") }

// Initialize is called by the engine once everything is loaded
func (h *dummyHandler) Initialize(s Server) error {
	h.server = s
	h.backend = s.Backend()
	return nil
}

// SendMsg sends the passed in message, returning any error
func (h *dummyHandler) SendMsg(ctx context.Context, msg Msg) (MsgStatus, error) {
	return h.backend.NewMsgStatusForID(msg.Channel(), msg.ID(), MsgSent), nil
}
