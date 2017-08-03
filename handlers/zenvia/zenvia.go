package zenvia

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/pkg/errors"
)

var sendURL = "http://www.zenvia360.com.br/GatewayIntegration/msgSms.do"

func init() {
	courier.RegisterHandler(NewHandler())
}

type handler struct {
	handlers.BaseHandler
}

// NewHandler returns a new Zenvia handler
func NewHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("ZV"), "Zenvia")}
}

type messageRequest struct {
	ID   string `validate:"required" name:"id"`
	Text string `validate:"required" name:"msg"`
	From string `validate:"required" name:"from"`
	To   string `validate:"required" name:"to"`
	Date string `validate:"required" name:"date"`
}

type statusRequest struct {
	ID     string `validate:"required" name:"id"`
	Status int32  `validate:"required" name:"status"`
}

var statusMapping = map[int32]courier.MsgStatusValue{
	120: courier.MsgDelivered,
	111: courier.MsgSent,
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
	zvMsg := &messageRequest{}
	err := handlers.DecodeAndValidateForm(zvMsg, r)
	if err != nil {
		return nil, err
	}

	// create our date from the timestamp
	// 03/05/2017 06:04:45
	date, err := time.Parse("02/01/2006 15:04:05", zvMsg.Date)
	if err != nil {
		return nil, fmt.Errorf("invalid date format: %s", zvMsg.Date)
	}

	// create our URN
	urn := courier.NewTelURNForChannel(zvMsg.From, channel)

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, zvMsg.Text).WithReceivedOn(date)

	// and finally queue our message
	err = h.Backend().WriteMsg(msg)
	if err != nil {
		return nil, err
	}

	return []courier.Msg{msg}, courier.WriteReceiveSuccess(w, r, msg)
}

// StatusMessage is our HTTP handler function for status updates
func (h *handler) StatusMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.MsgStatus, error) {
	// get our params
	zvStatus := &statusRequest{}
	err := handlers.DecodeAndValidateForm(zvStatus, r)
	if err != nil {
		return nil, err
	}

	msgStatus, found := statusMapping[zvStatus.Status]
	if !found {
		msgStatus = courier.MsgFailed
	}

	// write our status
	status := h.Backend().NewMsgStatusForExternalID(channel, zvStatus.ID, msgStatus)
	err = h.Backend().WriteMsgStatus(status)
	if err != nil {
		return nil, err
	}

	return []courier.MsgStatus{status}, courier.WriteStatusSuccess(w, r, status)

}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(msg courier.Msg) (courier.MsgStatus, error) {
	account := msg.Channel().StringConfigForKey(courier.ConfigAccount, "")
	if account == "" {
		return nil, fmt.Errorf("no account set for Zenvia channel")
	}

	code := msg.Channel().StringConfigForKey(courier.ConfigCode, "")
	if code == "" {
		return nil, fmt.Errorf("no code set for Zenvia channel")
	}

	// build our request
	form := url.Values{
		"dispatch":        []string{"send"},
		"account":         []string{account},
		"code":            []string{code},
		"to":              []string{msg.URN().Path()},
		"msg":             []string{courier.GetTextAndAttachments(msg)},
		"id":              []string{msg.ID().String()},
		"callbackOptions": []string{strconv.Itoa(1)},
	}

	req, err := http.NewRequest(http.MethodGet, sendURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "text/html")
	req.Header.Set("Accept-Charset", "ISO-8859-1")
	rr, err := utils.MakeHTTPRequest(req)

	// record our status and log
	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	status.AddLog(courier.NewChannelLogFromRR(msg.Channel(), msg.ID(), rr))
	if err != nil {
		return status, err
	}

	// was this request successful?
	msgStatus, err := strconv.ParseInt(rr.Response, 10, 64)
	if err != nil {
		return status, err
	}

	if msgStatus != 0 {
		msgStatusText := strconv.Itoa(123)
		return status, errors.Errorf("received non-zero response from Zenvia '%s'", msgStatusText)
	}

	return status, nil

}
