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
	route := s.AddChannelRoute(h, "POST", "receive", h.ReceiveMessage)
	if route.GetError() != nil {
		return route.GetError()
	}

	route = s.AddChannelRoute(h, "GET", "status", h.StatusMessage)
	return route.GetError()
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *kannelHandler) ReceiveMessage(channel *courier.Channel, w http.ResponseWriter, r *http.Request) error {
	// get our params
	kannelMsg := &kannelMessage{}
	err := handlers.DecodeAndValidateQueryParams(kannelMsg, r)
	if err != nil {
		return err
	}

	// create our date from the timestamp
	date := time.Unix(kannelMsg.Timestamp, 0).UTC()

	// create our URN
	urn := courier.NewTelURN(kannelMsg.Sender, channel.Country)

	// build our msg
	msg := courier.NewMsg(channel, urn, kannelMsg.Message).WithExternalID(fmt.Sprintf("%d", kannelMsg.ID)).WithReceivedOn(date)
	defer msg.Release()

	// and finally queue our message
	err = h.Server().QueueMsg(msg)
	if err != nil {
		return err
	}

	return courier.WriteReceiveSuccess(w, msg)
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
func (h *kannelHandler) StatusMessage(channel *courier.Channel, w http.ResponseWriter, r *http.Request) error {
	// get our params
	kannelStatus := &kannelStatus{}
	err := handlers.DecodeAndValidateQueryParams(kannelStatus, r)
	if err != nil {
		return err
	}

	msgStatus, found := kannelStatusMapping[kannelStatus.Status]
	if !found {
		return fmt.Errorf("unknown status '%d', must be one of 1,2,4,8,16", kannelStatus.Status)
	}

	// write our status
	status := courier.NewStatusUpdateForID(channel, kannelStatus.ID, msgStatus)
	defer status.Release()
	err = h.Server().UpdateMsgStatus(status)
	if err != nil {
		return err
	}

	return courier.WriteStatusSuccess(w, status)
}

type kannelStatus struct {
	ID     string `validate:"required" name:"id"`
	Status int    `validate:"required" name:"status"`
}
