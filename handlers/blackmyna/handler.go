package blackmyna

import (
	"fmt"
	"net/http"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
)

type bmHandler struct {
	handlers.BaseHandler
}

// NewHandler returns a new Blackmyna Handler
func NewHandler() courier.ChannelHandler {
	return &bmHandler{handlers.NewBaseHandler(courier.ChannelType("BM"), "Blackmyna")}
}

func init() {
	courier.RegisterHandler(NewHandler())
}

// Initialize is called by the engine once everything is loaded
func (h *bmHandler) Initialize(s courier.Server) error {
	h.SetServer(s)
	err := s.AddReceiveMsgRoute(h, "GET", "receive", h.ReceiveMessage)
	if err != nil {
		return err
	}

	return s.AddUpdateStatusRoute(h, "GET", "status", h.StatusMessage)
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *bmHandler) ReceiveMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]*courier.Msg, error) {
	// get our params
	bmMsg := &bmMessage{}
	err := handlers.DecodeAndValidateForm(bmMsg, r)
	if err != nil {
		return nil, err
	}

	// create our URN
	urn := courier.NewTelURNForChannel(bmMsg.From, channel)

	// build our msg
	msg := courier.NewIncomingMsg(channel, urn, bmMsg.Text)

	// and finally queue our message
	err = h.Server().WriteMsg(msg)
	if err != nil {
		return nil, err
	}

	return []*courier.Msg{msg}, courier.WriteReceiveSuccess(w, r, msg)
}

type bmMessage struct {
	To   string `validate:"required" name:"to"`
	Text string `validate:"required" name:"text"`
	From string `validate:"required" name:"from"`
}

var bmStatusMapping = map[int]courier.MsgStatus{
	1:  courier.MsgDelivered,
	2:  courier.MsgFailed,
	8:  courier.MsgSent,
	16: courier.MsgFailed,
}

// StatusMessage is our HTTP handler function for status updates
func (h *bmHandler) StatusMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]*courier.MsgStatusUpdate, error) {
	// get our params
	bmStatus := &bmStatus{}
	err := handlers.DecodeAndValidateForm(bmStatus, r)
	if err != nil {
		return nil, err
	}

	msgStatus, found := bmStatusMapping[bmStatus.Status]
	if !found {
		return nil, fmt.Errorf("unknown status '%d', must be one of 1, 2, 8 or 16", bmStatus.Status)
	}

	// write our status
	status := courier.NewStatusUpdateForExternalID(channel, bmStatus.ID, msgStatus)
	defer status.Release()
	err = h.Server().WriteMsgStatus(status)
	if err != nil {
		return nil, err
	}

	return []*courier.MsgStatusUpdate{status}, courier.WriteStatusSuccess(w, r, status)
}

// SendMsg sends the passed in message, returning any error
func (h *bmHandler) SendMsg(msg *courier.Msg) (*courier.MsgStatusUpdate, error) {
	return nil, fmt.Errorf("sending not implemented channel type: %s", msg.Channel.ChannelType())
}

type bmStatus struct {
	ID     string `validate:"required" name:"id"`
	Status int    `validate:"required" name:"status"`
}
