package kannel

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"strings"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/gsm7"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/phonenumbers"
	"github.com/pkg/errors"
)

const configUseNational = "use_national"
const configEncoding = "encoding"
const configVerifySSL = "verify_ssl"

const encodingDefault = "D"
const encodingUnicode = "U"
const encodingSmart = "S"

func init() {
	courier.RegisterHandler(NewHandler())
}

type handler struct {
	handlers.BaseHandler
}

// NewHandler returns a new KannelHandler
func NewHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("KN"), "Kannel")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	err := s.AddReceiveMsgRoute(h, "POST", "receive", h.ReceiveMessage)
	if err != nil {
		return err
	}

	return s.AddUpdateStatusRoute(h, "GET", "status", h.StatusMessage)
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *handler) ReceiveMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]*courier.Msg, error) {
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
func (h *handler) StatusMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]*courier.MsgStatusUpdate, error) {
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
func (h *handler) SendMsg(msg *courier.Msg) (*courier.MsgStatusUpdate, error) {
	username := msg.Channel.StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for KN channel")
	}

	password := msg.Channel.StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for KN channel")
	}

	sendURL := msg.Channel.StringConfigForKey(courier.ConfigSendURL, "")
	if sendURL == "" {
		return nil, fmt.Errorf("no send url set for KN channel")
	}

	dlrURL := fmt.Sprintf("%s%s%s/?id=%d&status=%%d", h.Server().Config().BaseURL, "/c/kn/", msg.Channel.UUID(), msg.ID.Int64)

	// build our request
	form := url.Values{
		"username": []string{username},
		"password": []string{password},
		"from":     []string{msg.Channel.Address()},
		"text":     []string{msg.TextAndAttachments()},
		"to":       []string{msg.URN.Path()},
		"dlr-url":  []string{dlrURL},
		"dlr-mask": []string{"31"},
	}

	// TODO: higher priority for responses
	//if msg.ResponseTo != 0 {
	//	form["priority"] = []string{"1"}
	//}

	useNationalStr := msg.Channel.ConfigForKey(configUseNational, false)
	useNational, _ := useNationalStr.(bool)

	// if we are meant to use national formatting (no country code) pull that out
	if useNational {
		parsed, err := phonenumbers.Parse(msg.URN.Path(), encodingDefault)
		if err == nil {
			form["to"] = []string{strings.Replace(phonenumbers.Format(parsed, phonenumbers.NATIONAL), " ", "", -1)}
		}
	}

	// figure out what encoding to tell kannel to send as
	encoding := msg.Channel.StringConfigForKey(configEncoding, encodingSmart)

	// if we are smart, first try to convert to GSM7 chars
	if encoding == encodingSmart {
		replaced := gsm7.ReplaceNonGSM7Chars(msg.TextAndAttachments())
		if gsm7.IsGSM7(replaced) {
			form["text"] = []string{replaced}
		} else {
			encoding = encodingUnicode
		}
	}

	// if we are UTF8, set our coding appropriately
	if encoding == encodingUnicode {
		form["coding"] = []string{"2"}
		form["charset"] = []string{"utf8"}
	}

	// our send URL may have form parameters in it already, append our own afterwards
	encodedForm := form.Encode()
	if strings.Contains(sendURL, "?") {
		sendURL = fmt.Sprintf("%s&%s", sendURL, encodedForm)
	} else {
		sendURL = fmt.Sprintf("%s?%s", sendURL, encodedForm)
	}

	// ignore SSL warnings if they ask
	verifySSLStr := msg.Channel.ConfigForKey(configVerifySSL, true)
	verifySSL, _ := verifySSLStr.(bool)

	req, err := http.NewRequest(http.MethodGet, sendURL, nil)
	var rr *utils.RequestResponse

	if verifySSL {
		rr, err = utils.MakeHTTPRequest(req)
	} else {
		rr, err = utils.MakeInsecureHTTPRequest(req)
	}

	// record our status and log
	status := courier.NewStatusUpdateForID(msg.Channel, msg.ID, courier.MsgErrored)
	status.AddLog(courier.NewChannelLogFromRR(msg.Channel, msg.ID, rr))
	if err != nil {
		return status, errors.Errorf("received error sending message")
	}

	status.Status = courier.MsgWired
	return status, nil
}

type kannelStatus struct {
	ID     courier.MsgID `validate:"required" name:"id"`
	Status int           `validate:"required" name:"status"`
}
