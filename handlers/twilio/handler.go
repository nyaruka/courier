package twilio

/*
 * Handler for Twilio channels, see https://www.twilio.com/docs/api
 */

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/buger/jsonparser"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/pkg/errors"
)

// TODO: agree on case!
const configAccountSID = "ACCOUNT_SID"
const configMessagingServiceSID = "messaging_service_sid"
const configSendURL = "send_url"

const twSignatureHeader = "X-Twilio-Signature"

type twHandler struct {
	handlers.BaseHandler
}

// NewHandler returns a new TwilioHandler ready to be registered
func NewHandler() courier.ChannelHandler {
	return &twHandler{handlers.NewBaseHandler(courier.ChannelType("TW"), "Twilio")}
}

func init() {
	courier.RegisterHandler(NewHandler())
}

// Initialize is called by the engine once everything is loaded
func (h *twHandler) Initialize(s courier.Server) error {
	h.SetServer(s)
	err := s.AddReceiveMsgRoute(h, "POST", "receive", h.ReceiveMessage)
	if err != nil {
		return err
	}

	return s.AddUpdateStatusRoute(h, "POST", "status", h.StatusMessage)
}

type twMessage struct {
	MessageSID  string `validate:"required"`
	AccountSID  string `validate:"required"`
	From        string `validate:"required"`
	FromCountry string
	To          string `validate:"required"`
	ToCountry   string
	Body        string `validate:"required"`
	NumMedia    int
}

type twStatus struct {
	MessageSID    string `validate:"required"`
	MessageStatus string `validate:"required"`
	ErrorCode     string
}

var twStatusMapping = map[string]courier.MsgStatus{
	"queued":      courier.MsgSent,
	"failed":      courier.MsgFailed,
	"sent":        courier.MsgSent,
	"delivered":   courier.MsgDelivered,
	"undelivered": courier.MsgFailed,
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *twHandler) ReceiveMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]*courier.Msg, error) {
	err := h.validateSignature(channel, r)
	if err != nil {
		return nil, err
	}

	// get our params
	twMsg := &twMessage{}
	err = handlers.DecodeAndValidateForm(twMsg, r)
	if err != nil {
		return nil, err
	}

	// create our URN
	urn := courier.NewTelURNForCountry(twMsg.From, twMsg.FromCountry)

	if twMsg.Body != "" {
		// Twilio sometimes sends concatenated sms as base64 encoded MMS
		twMsg.Body = handlers.DecodePossibleBase64(twMsg.Body)
	}

	// build our msg
	msg := courier.NewIncomingMsg(channel, urn, twMsg.Body).WithExternalID(twMsg.MessageSID)

	// process any attached media
	for i := 0; i < twMsg.NumMedia; i++ {
		mediaURL := r.PostForm.Get(fmt.Sprintf("MediaUrl%d", i))
		msg.AddAttachment(mediaURL)
	}

	// and finally queue our message
	err = h.Server().WriteMsg(msg)
	if err != nil {
		return nil, err
	}

	return []*courier.Msg{msg}, h.writeReceiveSuccess(w)
}

// StatusMessage is our HTTP handler function for status updates
func (h *twHandler) StatusMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]*courier.MsgStatusUpdate, error) {
	err := h.validateSignature(channel, r)
	if err != nil {
		return nil, err
	}

	// get our params
	twStatus := &twStatus{}
	err = handlers.DecodeAndValidateForm(twStatus, r)
	if err != nil {
		return nil, err
	}

	msgStatus, found := twStatusMapping[twStatus.MessageStatus]
	if !found {
		return nil, fmt.Errorf("unknown status '%s', must be one of 'queued', 'failed', 'sent', 'delivered', or 'undelivered'", twStatus.MessageStatus)
	}

	// write our status
	status := courier.NewStatusUpdateForExternalID(channel, twStatus.MessageSID, msgStatus)
	defer status.Release()
	err = h.Server().WriteMsgStatus(status)
	if err != nil {
		return nil, err
	}

	return []*courier.MsgStatusUpdate{status}, courier.WriteStatusSuccess(w, r, status)
}

// SendMsg sends the passed in message, returning any error
func (h *twHandler) SendMsg(msg *courier.Msg) (*courier.MsgStatusUpdate, error) {
	// build our callback URL
	callbackURL := fmt.Sprintf("%s/c/kn/%s/status/", h.Server().Config().BaseURL, msg.Channel.UUID())

	accountSID := msg.Channel.StringConfigForKey(configAccountSID, "")
	if accountSID == "" {
		return nil, fmt.Errorf("missing account sid for twilio channel")
	}

	accountToken := msg.Channel.StringConfigForKey(courier.ConfigAuthToken, "")
	if accountToken == "" {
		return nil, fmt.Errorf("missing account auth token for twilio channel")
	}

	// build our request
	form := url.Values{
		"To":             []string{msg.URN.Path()},
		"Body":           []string{msg.Text},
		"StatusCallback": []string{callbackURL},
	}

	// add any media URL
	if len(msg.Attachments) > 0 {
		_, mediaURL := courier.SplitAttachment(msg.Attachments[0])
		form["MediaURL"] = []string{mediaURL}
	}

	// set our from, either as a messaging service or from our address
	serviceSID := msg.Channel.StringConfigForKey(configMessagingServiceSID, "")
	if serviceSID != "" {
		form["MessagingServiceSID"] = []string{serviceSID}
	} else {
		form["From"] = []string{msg.Channel.Address()}
	}

	baseSendURL := msg.Channel.StringConfigForKey(configSendURL, "https://api.twilio.com/2010-04-01/Accounts/")
	sendURL := fmt.Sprintf("%s%s/Messages.json", baseSendURL, accountSID)
	req, err := http.NewRequest("POST", sendURL, strings.NewReader(form.Encode()))
	rr, err := utils.MakeHTTPRequest(req)

	// record our status and log
	status := courier.NewStatusUpdateForID(msg.Channel, msg.ID, courier.MsgErrored)
	status.AddLog(courier.NewChannelLogFromRR(msg.Channel, msg.ID, rr))

	// was this request successful?
	errorCode, _ := jsonparser.GetInt([]byte(rr.Body), "error_code")
	if err != nil || errorCode != 0 {
		// TODO: Notify RapidPro of blocked contacts (code 21610)
		return status, errors.Errorf("received error from twilio")
	}

	// grab the external id
	externalID, err := jsonparser.GetString([]byte(rr.Body), "sid")
	if err != nil {
		return status, errors.Errorf("unable to get sid from body")
	}

	status.Status = courier.MsgWired
	status.ExternalID = externalID

	return status, nil
}

// Twilio expects Twiml from a message receive request
func (h *twHandler) writeReceiveSuccess(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(200)
	_, err := fmt.Fprint(w, "<Response/>")
	return err
}

// see https://www.twilio.com/docs/api/security
func (h *twHandler) validateSignature(channel courier.Channel, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s%s", h.Server().Config().BaseURL, r.URL.RequestURI())
	confAuth := channel.ConfigForKey(courier.ConfigAuthToken, "")
	authToken, isStr := confAuth.(string)
	if !isStr || authToken == "" {
		return fmt.Errorf("invalid or missing auth token in config")
	}

	expected, err := twCalculateSignature(url, r.PostForm, authToken)
	if err != nil {
		return err
	}

	actual := r.Header.Get(twSignatureHeader)
	if actual == "" {
		return fmt.Errorf("missing request signature")
	}

	// compare signatures in way that isn't sensitive to a timing attack
	if !hmac.Equal(expected, []byte(actual)) {
		return fmt.Errorf("invalid request signature")
	}
	return nil
}

// see https://www.twilio.com/docs/api/security
func twCalculateSignature(url string, form url.Values, authToken string) ([]byte, error) {
	var buffer bytes.Buffer
	buffer.WriteString(url)

	keys := make(sort.StringSlice, 0, len(form))
	for k := range form {
		keys = append(keys, k)
	}
	keys.Sort()

	for _, k := range keys {
		buffer.WriteString(k)
		for _, v := range form[k] {
			buffer.WriteString(v)
		}
	}

	// hash with SHA1
	mac := hmac.New(sha1.New, []byte(authToken))
	mac.Write(buffer.Bytes())
	hash := mac.Sum(nil)

	// encode with Base64
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(hash)))
	base64.StdEncoding.Encode(encoded, hash)

	return encoded, nil
}
