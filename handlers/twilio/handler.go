package twilio

/*
Handler for Twilio channels, see https://www.twilio.com/docs/api

Examples:

POST /c/tw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/
ToCountry=US&ToState=IN&SmsMessageSid=SMa741ddeb574e5dda5516c73417fcd28a&NumMedia=0&ToCity=&FromZip=46204&SmsSid=SMa741ddeb574e5dda5516c73417fcd28a&FromState=IN&SmsStatus=received&FromCity=INDIANAPOLIS&Body=Hi+there+from+Twilio&FromCountry=US&To=%2B13177933221&ToZip=&NumSegments=1&MessageSid=SMa741ddeb574e5dda5516c73417fcd28a&AccountSid=AC7ef44158dbb01b972d64d7e5c851c8d7&From=%2B13177592786&ApiVersion=2010-04-01

POST /c/tw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/
MessageStatus=sent&ApiVersion=2010-04-01&SmsSid=SM7ac25b8b7f04410093ff54e1fd2b4256&SmsStatus=sent&To=%2B13177933221&From=%2B13177592786&MessageSid=SM7ac25b8b7f04410093ff54e1fd2b4256&AccountSid=AC7ef44158dbb01b972d64d7e5c851c8d7
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

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
)

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
	route := s.AddChannelRoute(h, "POST", "receive", h.ReceiveMessage)
	if route.GetError() != nil {
		return route.GetError()
	}

	route = s.AddChannelRoute(h, "POST", "status", h.StatusMessage)
	return route.GetError()
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
func (h *twHandler) ReceiveMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) error {
	err := h.validateSignature(channel, r)
	if err != nil {
		return err
	}

	// get our params
	twMsg := &twMessage{}
	err = handlers.DecodeAndValidateForm(twMsg, r)
	if err != nil {
		return err
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
		return err
	}

	return h.writeReceiveSuccess(w)
}

// StatusMessage is our HTTP handler function for status updates
func (h *twHandler) StatusMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) error {
	err := h.validateSignature(channel, r)
	if err != nil {
		return err
	}

	// get our params
	twStatus := &twStatus{}
	err = handlers.DecodeAndValidateForm(twStatus, r)
	if err != nil {
		return err
	}

	msgStatus, found := twStatusMapping[twStatus.MessageStatus]
	if !found {
		return fmt.Errorf("unknown status '%s', must be one of 'queued', 'failed', 'sent', 'delivered', or 'undelivered'", twStatus.MessageStatus)
	}

	// write our status
	status := courier.NewStatusUpdateForExternalID(channel, twStatus.MessageSID, msgStatus)
	defer status.Release()
	err = h.Server().WriteMsgStatus(status)
	if err != nil {
		return err
	}

	return courier.WriteStatusSuccess(w, status)
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
