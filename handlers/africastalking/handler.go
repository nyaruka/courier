package africastalking

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

type handler struct {
	handlers.BaseHandler
}

// NewHandler returns a new Africa's Talking handler
func NewHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("AT"), "Africas Talking")}
}

type messageRequest struct {
	ID   string `validate:"required" name:"id"`
	Text string `validate:"required" name:"text"`
	From string `validate:"required" name:"from"`
	To   string `validate:"required" name:"to"`
	Date string `validate:"required" name:"date"`
}

type statusRequest struct {
	ID     string `validate:"required" name:"id"`
	Status string `validate:"required" name:"status"`
}

var statusMapping = map[string]courier.MsgStatus{
	"Success":  courier.MsgDelivered,
	"Sent":     courier.MsgSent,
	"Buffered": courier.MsgSent,
	"Rejected": courier.MsgFailed,
	"Failed":   courier.MsgFailed,
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	route := s.AddChannelRoute(h, "POST", "receive", h.ReceiveMessage)
	if route.GetError() != nil {
		return route.GetError()
	}

	route = s.AddChannelRoute(h, "POST", "status", h.StatusMessage)
	return route.GetError()
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *handler) ReceiveMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) error {
	// get our params
	atMsg := &messageRequest{}
	err := handlers.DecodeAndValidateForm(atMsg, r)
	if err != nil {
		return err
	}

	// create our date from the timestamp
	// 2017-05-03T06:04:45Z
	date, err := time.Parse("2006-01-02T15:04:05Z", atMsg.Date)
	if err != nil {
		return fmt.Errorf("invalid date format: %s", atMsg.Date)
	}

	// create our URN
	urn := courier.NewTelURNForChannel(atMsg.From, channel)

	// build our msg
	msg := courier.NewIncomingMsg(channel, urn, atMsg.Text).WithExternalID(atMsg.ID).WithReceivedOn(date)

	// and finally queue our message
	err = h.Server().WriteMsg(msg)
	if err != nil {
		return err
	}

	return courier.WriteReceiveSuccess(w, msg)
}

// StatusMessage is our HTTP handler function for status updates
func (h *handler) StatusMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) error {
	// get our params
	atStatus := &statusRequest{}
	err := handlers.DecodeAndValidateForm(atStatus, r)
	if err != nil {
		return err
	}

	msgStatus, found := statusMapping[atStatus.Status]
	if !found {
		return fmt.Errorf("unknown status '%s', must be one of 'Success','Sent','Buffered','Rejected' or 'Failed'", atStatus.Status)
	}

	// write our status
	status := courier.NewStatusUpdateForExternalID(channel, atStatus.ID, msgStatus)
	defer status.Release()
	err = h.Server().WriteMsgStatus(status)
	if err != nil {
		return err
	}

	return courier.WriteStatusSuccess(w, status)
}
