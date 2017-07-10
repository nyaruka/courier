package africastalking

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/pkg/errors"
)

const configIsShared = "is_shared"

var sendURL = "https://api.africastalking.com/version1/messaging"

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
	err := s.AddReceiveMsgRoute(h, "POST", "receive", h.ReceiveMessage)
	if err != nil {
		return err
	}
	return s.AddUpdateStatusRoute(h, "POST", "status", h.StatusMessage)
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *handler) ReceiveMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Msg, error) {
	// get our params
	atMsg := &messageRequest{}
	err := handlers.DecodeAndValidateForm(atMsg, r)
	if err != nil {
		return nil, err
	}

	// create our date from the timestamp
	// 2017-05-03T06:04:45Z
	date, err := time.Parse("2006-01-02T15:04:05Z", atMsg.Date)
	if err != nil {
		return nil, fmt.Errorf("invalid date format: %s", atMsg.Date)
	}

	// create our URN
	urn := courier.NewTelURNForChannel(atMsg.From, channel)

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, atMsg.Text).WithExternalID(atMsg.ID).WithReceivedOn(date)

	// and finally queue our message
	err = h.Backend().WriteMsg(msg)
	if err != nil {
		return nil, err
	}

	return []courier.Msg{msg}, courier.WriteReceiveSuccess(w, r, msg)
}

// StatusMessage is our HTTP handler function for status updates
func (h *handler) StatusMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]*courier.MsgStatusUpdate, error) {
	// get our params
	atStatus := &statusRequest{}
	err := handlers.DecodeAndValidateForm(atStatus, r)
	if err != nil {
		return nil, err
	}

	msgStatus, found := statusMapping[atStatus.Status]
	if !found {
		return nil, fmt.Errorf("unknown status '%s', must be one of 'Success','Sent','Buffered','Rejected' or 'Failed'", atStatus.Status)
	}

	// write our status
	status := courier.NewStatusUpdateForExternalID(channel, atStatus.ID, msgStatus)
	err = h.Backend().WriteMsgStatus(status)
	if err != nil {
		return nil, err
	}

	return []*courier.MsgStatusUpdate{status}, courier.WriteStatusSuccess(w, r, status)
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(msg courier.Msg) (*courier.MsgStatusUpdate, error) {
	isSharedStr := msg.Channel().ConfigForKey(configIsShared, false)
	isShared, _ := isSharedStr.(bool)

	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for AT channel")
	}

	apiKey := msg.Channel().StringConfigForKey(courier.ConfigAPIKey, "")
	if apiKey == "" {
		return nil, fmt.Errorf("no API key set for AT channel")
	}

	// build our request
	form := url.Values{
		"username": []string{username},
		"to":       []string{msg.URN().Path()},
		"message":  []string{courier.GetTextAndAttachments(msg)},
	}

	// if this isn't shared, include our from
	if !isShared {
		form["from"] = []string{msg.Channel().Address()}
	}

	req, err := http.NewRequest(http.MethodPost, sendURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("apikey", apiKey)
	rr, err := utils.MakeHTTPRequest(req)

	// record our status and log
	status := courier.NewStatusUpdateForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	status.AddLog(courier.NewChannelLogFromRR(msg.Channel(), msg.ID(), rr))
	if err != nil {
		return status, err
	}

	// was this request successful?
	msgStatus, _ := jsonparser.GetString([]byte(rr.Body), "SMSMessageData", "Recipients", "[0]", "status")
	if msgStatus != "Success" {
		return status, errors.Errorf("received non-success status '%s'", msgStatus)
	}

	// grab the external id if we can
	externalID, _ := jsonparser.GetString([]byte(rr.Body), "SMSMessageData", "Recipients", "[0]", "messageId")
	status.Status = courier.MsgWired
	status.ExternalID = externalID

	return status, nil
}
