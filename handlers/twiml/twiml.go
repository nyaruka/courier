package twiml

/*
 * Handler for TWIML based channels
 */

import (
	"bytes"
	"context"
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

const (
	configAccountSID          = "account_sid"
	configMessagingServiceSID = "messaging_service_sid"
	configSendURL             = "send_url"
	configBaseURL             = "base_url"
	configIgnoreDLRs          = "ignore_dlrs"

	signatureHeader     = "X-Twilio-Signature"
	forwardedPathHeader = "X-Forwarded-Path"
)

var (
	maxMsgLength  = 1600
	twilioBaseURL = "https://api.twilio.com"
)

// error code twilio returns when a contact has sent "stop"
const errorStopped = 21610

type handler struct {
	handlers.BaseHandler
	validateSignatures bool
}

func newTWIMLHandler(channelType courier.ChannelType, name string, validateSignatures bool) courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(channelType, name), validateSignatures}
}

func init() {
	courier.RegisterHandler(newTWIMLHandler("TW", "TWIML API", true))
	courier.RegisterHandler(newTWIMLHandler("T", "Twilio", true))
	courier.RegisterHandler(newTWIMLHandler("TMS", "Twilio Messaging Service", true))
	courier.RegisterHandler(newTWIMLHandler("SW", "SignalWire", false))
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodPost, "status", h.receiveStatus)
	return nil
}

type moForm struct {
	MessageSID  string `validate:"required"`
	AccountSID  string `validate:"required"`
	From        string `validate:"required"`
	FromCountry string
	To          string `validate:"required"`
	ToCountry   string
	Body        string
	NumMedia    int
}

type statusForm struct {
	MessageSID    string `validate:"required"`
	MessageStatus string `validate:"required"`
	ErrorCode     string
}

var statusMapping = map[string]courier.MsgStatusValue{
	"queued":      courier.MsgSent,
	"failed":      courier.MsgFailed,
	"sent":        courier.MsgSent,
	"delivered":   courier.MsgDelivered,
	"undelivered": courier.MsgFailed,
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	err := h.validateSignature(channel, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// get our params
	form := &moForm{}
	err = handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// create our URN
	var urn urns.URN
	if channel.IsScheme(urns.WhatsAppScheme) {
		// Twilio Whatsapp from is in the form: whatsapp:+12211414154
		parts := strings.Split(form.From, ":")

		// trim off left +, official whatsapp IDs dont have that
		urn, err = urns.NewWhatsAppURN(strings.TrimLeft(parts[1], "+"))
	} else {
		urn, err = urns.NewTelURNForCountry(form.From, form.FromCountry)
	}

	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	if form.Body != "" {
		// Twilio sometimes sends concatenated sms as base64 encoded MMS
		form.Body = handlers.DecodePossibleBase64(form.Body)
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, form.Body).WithExternalID(form.MessageSID)

	// process any attached media
	for i := 0; i < form.NumMedia; i++ {
		mediaURL := r.PostForm.Get(fmt.Sprintf("MediaUrl%d", i))
		msg.WithAttachment(mediaURL)
	}
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r)
}

// receiveStatus is our HTTP handler function for status updates
func (h *handler) receiveStatus(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	err := h.validateSignature(channel, r)
	if err != nil {
		return nil, err
	}

	// get our params
	form := &statusForm{}
	err = handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "no msg status, ignoring")
	}

	msgStatus, found := statusMapping[form.MessageStatus]
	if !found {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unknown status '%s', must be one of 'queued', 'failed', 'sent', 'delivered', or 'undelivered'", form.MessageStatus))
	}

	// if we are ignoring delivery reports and this isn't failed then move on
	if channel.BoolConfigForKey(configIgnoreDLRs, false) && msgStatus != courier.MsgFailed {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring non error delivery report")
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
		status = h.Backend().NewMsgStatusForExternalID(channel, form.MessageSID, msgStatus)
	}
	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	// build our callback URL
	callbackDomain := msg.Channel().CallbackDomain(h.Server().Config().Domain)
	callbackURL := fmt.Sprintf("https://%s/c/%s/%s/status?id=%d&action=callback", callbackDomain, strings.ToLower(h.ChannelType().String()), msg.Channel().UUID(), msg.ID())

	accountSID := msg.Channel().StringConfigForKey(configAccountSID, "")
	if accountSID == "" {
		return nil, fmt.Errorf("missing account sid for %s channel", h.ChannelName())
	}

	accountToken := msg.Channel().StringConfigForKey(courier.ConfigAuthToken, "")
	if accountToken == "" {
		return nil, fmt.Errorf("missing account auth token for %s channel", h.ChannelName())
	}

	channel := msg.Channel()

	status := h.Backend().NewMsgStatusForID(channel, msg.ID(), courier.MsgErrored)
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
			_, mediaURL := handlers.SplitAttachment(msg.Attachments()[0])
			form["MediaUrl"] = []string{mediaURL}
		}

		// set our from, either as a messaging service or from our address
		serviceSID := channel.StringConfigForKey(configMessagingServiceSID, "")
		if serviceSID != "" {
			form["MessagingServiceSid"] = []string{serviceSID}
		} else {
			form["From"] = []string{channel.Address()}
		}

		// for whatsapp channels, we have to prepend whatsapp to the To and From
		if channel.IsScheme(urns.WhatsAppScheme) {
			form["To"][0] = fmt.Sprintf("%s:+%s", urns.WhatsAppScheme, form["To"][0])
			form["From"][0] = fmt.Sprintf("%s:%s", urns.WhatsAppScheme, form["From"][0])
		}

		// build our URL
		baseURL := h.baseURL(channel)
		if baseURL == "" {
			return nil, fmt.Errorf("missing base URL for %s channel", h.ChannelName())
		}

		sendURL, err := utils.AddURLPath(baseURL, "2010-04-01", "Accounts", accountSID, "Messages.json")
		if err != nil {
			return nil, err
		}

		req, _ := http.NewRequest(http.MethodPost, sendURL, strings.NewReader(form.Encode()))
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

					// create a stop channel event
					channelEvent := h.Backend().NewChannelEvent(msg.Channel(), courier.StopContact, msg.URN())
					err = h.Backend().WriteChannelEvent(ctx, channelEvent)
					if err != nil {
						return nil, err
					}
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

func (h *handler) baseURL(c courier.Channel) string {
	// Twilio channels use the Twili base URL
	if c.ChannelType() == "T" || c.ChannelType() == "TMS" {
		return twilioBaseURL
	}

	return c.StringConfigForKey(configSendURL, c.StringConfigForKey(configBaseURL, ""))
}

// see https://www.twilio.com/docs/api/security
func (h *handler) validateSignature(c courier.Channel, r *http.Request) error {
	if !h.validateSignatures {
		return nil
	}

	actual := r.Header.Get(signatureHeader)
	if actual == "" {
		return fmt.Errorf("missing request signature")
	}

	if err := r.ParseForm(); err != nil {
		return err
	}

	confAuth := c.ConfigForKey(courier.ConfigAuthToken, "")
	authToken, isStr := confAuth.(string)
	if !isStr || authToken == "" {
		return fmt.Errorf("invalid or missing auth token in config")
	}

	path := r.URL.RequestURI()
	proxyPath := r.Header.Get(forwardedPathHeader)
	if proxyPath != "" {
		path = proxyPath
	}

	url := fmt.Sprintf("https://%s%s", r.Host, path)
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

// WriteMsgSuccessResponse writes our response in TWIML format
func (h *handler) WriteMsgSuccessResponse(ctx context.Context, w http.ResponseWriter, r *http.Request, msgs []courier.Msg) error {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(200)
	_, err := fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><Response/>`)
	return err
}

// WriteRequestIgnored writes our response in TWIML format
func (h *handler) WriteRequestIgnored(ctx context.Context, w http.ResponseWriter, r *http.Request, details string) error {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(200)
	_, err := fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><!-- %s --><Response/>`, details)
	return err
}
