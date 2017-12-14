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
	"strconv"
	"strings"

	"github.com/buger/jsonparser"
	"github.com/sirupsen/logrus"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
	"github.com/pkg/errors"
)

const configAccountSID = "account_sid"
const configMessagingServiceSID = "messaging_service_sid"
const configSendURL = "send_url"

const twSignatureHeader = "X-Twilio-Signature"

var maxMsgLength = 1600
var sendURL = "https://api.twilio.com/2010-04-01/Accounts"

// error code twilio returns when a contact has sent "stop"
const errorStopped = 21610

type handler struct {
	handlers.BaseHandler
	ignoreDeliveryReports bool
}

// NewHandler returns a new TwilioHandler ready to be registered
func NewHandler(channelType string, name string) courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType(channelType), name), false}
}

func init() {
	courier.RegisterHandler(NewHandler("T", "Twilio"))
	courier.RegisterHandler(NewHandler("TMS", "Twilio Messaging Service"))

}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)

	// save whether we should ignore delivery reports
	h.ignoreDeliveryReports = s.Config().IgnoreDeliveryReports

	err := s.AddHandlerRoute(h, "POST", "receive", h.ReceiveMessage)
	if err != nil {
		return err
	}

	return s.AddHandlerRoute(h, "POST", "status", h.StatusMessage)
}

type twMessage struct {
	MessageSID  string `validate:"required"`
	AccountSID  string `validate:"required"`
	From        string `validate:"required"`
	FromCountry string
	To          string `validate:"required"`
	ToCountry   string
	Body        string
	NumMedia    int
}

type twStatus struct {
	MessageSID    string `validate:"required"`
	MessageStatus string `validate:"required"`
	ErrorCode     string
}

var twStatusMapping = map[string]courier.MsgStatusValue{
	"queued":      courier.MsgSent,
	"failed":      courier.MsgFailed,
	"sent":        courier.MsgSent,
	"delivered":   courier.MsgDelivered,
	"undelivered": courier.MsgFailed,
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *handler) ReceiveMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
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
	urn := urns.NewTelURNForCountry(twMsg.From, twMsg.FromCountry)

	if twMsg.Body != "" {
		// Twilio sometimes sends concatenated sms as base64 encoded MMS
		twMsg.Body = handlers.DecodePossibleBase64(twMsg.Body)
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, twMsg.Body).WithExternalID(twMsg.MessageSID)

	// process any attached media
	for i := 0; i < twMsg.NumMedia; i++ {
		mediaURL := r.PostForm.Get(fmt.Sprintf("MediaUrl%d", i))
		msg.WithAttachment(mediaURL)
	}

	// and finally queue our message
	err = h.Backend().WriteMsg(msg)
	if err != nil {
		return nil, err
	}

	return []courier.Event{msg}, h.writeReceiveSuccess(w, r, msg)
}

// StatusMessage is our HTTP handler function for status updates
func (h *handler) StatusMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
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

	// if we are ignoring delivery reports and this isn't failed then move on
	if h.ignoreDeliveryReports && msgStatus != courier.MsgFailed {
		return nil, courier.WriteIgnored(w, r, "ignoring non error delivery report")
	}

	// if the message id was passed explicitely, use that
	var status courier.MsgStatus
	idString := r.URL.Query().Get("id")
	if idString != "" {
		msgID, err := strconv.ParseInt(idString, 10, 64)
		if err != nil {
			logrus.WithError(err).WithField("id", idString).Error("error converting twilio callback id to integer")
		} else {
			status = h.Backend().NewMsgStatusForID(channel, courier.NewMsgID(msgID), msgStatus)
		}
	}

	// if we have no status, then build it from the external (twilio) id
	if status == nil {
		status = h.Backend().NewMsgStatusForExternalID(channel, twStatus.MessageSID, msgStatus)
	}

	// write our status
	err = h.Backend().WriteMsgStatus(status)
	if err != nil {
		return nil, err
	}

	return []courier.Event{status}, courier.WriteStatusSuccess(w, r, []courier.MsgStatus{status})
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(msg courier.Msg) (courier.MsgStatus, error) {
	// build our callback URL
	callbackDomain := msg.Channel().CallbackDomain(h.Server().Config().Domain)
	callbackURL := fmt.Sprintf("https://%s/c/%s/%s/status?id=%d&action=callback", callbackDomain, strings.ToLower(msg.Channel().ChannelType().String()), msg.Channel().UUID(), msg.ID().Int64)

	accountSID := msg.Channel().StringConfigForKey(configAccountSID, "")
	if accountSID == "" {
		return nil, fmt.Errorf("missing account sid for twilio channel")
	}

	accountToken := msg.Channel().StringConfigForKey(courier.ConfigAuthToken, "")
	if accountToken == "" {
		return nil, fmt.Errorf("missing account auth token for twilio channel")
	}

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	parts := handlers.SplitMsg(msg.Text(), maxMsgLength)
	for i, part := range parts {
		// build our request
		form := url.Values{
			"To":             []string{msg.URN().Path()},
			"Body":           []string{part},
			"StatusCallback": []string{callbackURL},
		}

		// add any media URL to the first part
		if len(msg.Attachments()) > 0 && i == 0 {
			_, mediaURL := courier.SplitAttachment(msg.Attachments()[0])
			form["MediaUrl"] = []string{mediaURL}
		}

		// set our from, either as a messaging service or from our address
		serviceSID := msg.Channel().StringConfigForKey(configMessagingServiceSID, "")
		if serviceSID != "" {
			form["MessagingServiceSID"] = []string{serviceSID}
		} else {
			form["From"] = []string{msg.Channel().Address()}
		}

		baseSendURL := msg.Channel().StringConfigForKey(configSendURL, sendURL)
		sendURL, err := utils.AddURLPath(baseSendURL, accountSID, "Messages.json")
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequest(http.MethodPost, sendURL, strings.NewReader(form.Encode()))
		req.SetBasicAuth(accountSID, accountToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		rr, err := utils.MakeHTTPRequest(req)

		// record our status and log
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
		status.AddLog(log)

		// see if we can parse the error if we have one
		if err != nil && rr.Body != nil {
			errorCode, _ := jsonparser.GetInt([]byte(rr.Body), "code")
			if errorCode != 0 {
				if errorCode == errorStopped {
					status.SetStatus(courier.MsgFailed)
					h.Backend().StopMsgContact(msg)
				}
				log.WithError("Message Send Error", errors.Errorf("received error code from twilio '%d'", errorCode))
				return status, nil
			}
		}

		// fail if we received an error
		if err != nil {
			return status, nil
		}

		// grab the external id
		externalID, err := jsonparser.GetString([]byte(rr.Body), "sid")
		if err != nil {
			log.WithError("Message Send Error", errors.Errorf("unable to get sid from body"))
			return status, nil
		}

		status.SetStatus(courier.MsgWired)

		// only save the first external id
		if i == 0 {
			status.SetExternalID(externalID)
		}
	}

	return status, nil
}

// Twilio expects Twiml from a message receive request
func (h *handler) writeReceiveSuccess(w http.ResponseWriter, r *http.Request, msg courier.Msg) error {
	courier.LogMsgReceived(r, msg)
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(200)
	_, err := fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><Response/>`)
	return err
}

// see https://www.twilio.com/docs/api/security
func (h *handler) validateSignature(channel courier.Channel, r *http.Request) error {
	actual := r.Header.Get(twSignatureHeader)
	if actual == "" {
		return fmt.Errorf("missing request signature")
	}

	if err := r.ParseForm(); err != nil {
		return err
	}

	confAuth := channel.ConfigForKey(courier.ConfigAuthToken, "")
	authToken, isStr := confAuth.(string)
	if !isStr || authToken == "" {
		return fmt.Errorf("invalid or missing auth token in config")
	}

	url := fmt.Sprintf("https://%s%s", r.Host, r.URL.RequestURI())
	expected, err := twCalculateSignature(url, r.PostForm, authToken)
	if err != nil {
		return err
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
