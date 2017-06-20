package kannel

import (
	"fmt"
	"net/http"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
)

func init() {
	courier.RegisterHandler(NewHandler())
}

type kannelHandler struct {
	handlers.BaseHandler
}

// NewHandler returns a new KannelHandler
func NewHandler() courier.ChannelHandler {
	return &kannelHandler{handlers.NewBaseHandler(courier.ChannelType("KN"), "Kannel")}
}

// Initialize is called by the engine once everything is loaded
func (h *kannelHandler) Initialize(s courier.Server) error {
	h.SetServer(s)
	err := s.AddReceiveMsgRoute(h, "POST", "receive", h.ReceiveMessage)
	if err != nil {
		return err
	}

	return s.AddUpdateStatusRoute(h, "GET", "status", h.StatusMessage)
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *kannelHandler) ReceiveMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]*courier.Msg, error) {
	// get our params
	kannelMsg := &kannelMessage{}
	err := handlers.DecodeAndValidateQueryParams(kannelMsg, r)
	if err != nil {
		return nil, err
	}

	// create our date from the timestamp
	date := time.Unix(kannelMsg.Timestamp, 0).UTC()

	// create our URN
	urn := courier.NewTelURNForChannel(kannelMsg.Sender, channel)

	// build our msg
	msg := courier.NewIncomingMsg(channel, urn, kannelMsg.Message).WithExternalID(fmt.Sprintf("%d", kannelMsg.ID)).WithReceivedOn(date)

	// and finally queue our message
	err = h.Server().WriteMsg(msg)
	if err != nil {
		return nil, err
	}

	return []*courier.Msg{msg}, courier.WriteReceiveSuccess(w, r, msg)
}

type kannelMessage struct {
	ID        int64  `validate:"required" name:"id"`
	Timestamp int64  `validate:"required" name:"ts"`
	Message   string `validate:"required" name:"message"`
	Sender    string `validate:"required" name:"sender"`
}

var kannelStatusMapping = map[int]courier.MsgStatus{
	1:  courier.MsgDelivered,
	2:  courier.MsgFailed,
	4:  courier.MsgSent,
	8:  courier.MsgSent,
	16: courier.MsgFailed,
}

// StatusMessage is our HTTP handler function for status updates
func (h *kannelHandler) StatusMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]*courier.MsgStatusUpdate, error) {
	// get our params
	kannelStatus := &kannelStatus{}
	err := handlers.DecodeAndValidateQueryParams(kannelStatus, r)
	if err != nil {
		return nil, err
	}

	msgStatus, found := kannelStatusMapping[kannelStatus.Status]
	if !found {
		return nil, fmt.Errorf("unknown status '%d', must be one of 1,2,4,8,16", kannelStatus.Status)
	}

	// write our status
	status := courier.NewStatusUpdateForID(channel, kannelStatus.ID, msgStatus)
	err = h.Server().WriteMsgStatus(status)
	if err != nil {
		return nil, err
	}

	return []*courier.MsgStatusUpdate{status}, courier.WriteStatusSuccess(w, r, status)
}

// SendMsg sends the passed in message, returning any error
func (h *kannelHandler) SendMsg(msg *courier.Msg) (*courier.MsgStatusUpdate, error) {
	return nil, fmt.Errorf("sending not implemented channel type: %s", msg.Channel.ChannelType())
}

type kannelStatus struct {
	ID     courier.MsgID `validate:"required" name:"id"`
	Status int           `validate:"required" name:"status"`
}
