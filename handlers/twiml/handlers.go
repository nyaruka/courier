package twiml

/*
 * Handler for TWIML based channels
 */

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	_ "embed"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/httpx"
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

	//go:embed errors.json
	errorCodes []byte
)

// see https://www.twilio.com/docs/sms/accepted-mime-types#accepted-mime-types
var mediaSupport = map[handlers.MediaType]handlers.MediaTypeSupport{
	handlers.MediaTypeImage:       {MaxBytes: 5 * 1024 * 1024},
	handlers.MediaTypeAudio:       {MaxBytes: 5 * 1024 * 1024},
	handlers.MediaTypeVideo:       {MaxBytes: 5 * 1024 * 1024},
	handlers.MediaTypeApplication: {MaxBytes: 5 * 1024 * 1024},
}

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
	courier.RegisterHandler(newTWIMLHandler("TWA", "Twilio Whatsapp", true))
	courier.RegisterHandler(newTWIMLHandler("SW", "SignalWire", false))
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodPost, "status", courier.ChannelLogTypeMsgStatus, h.receiveStatus)
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
	ButtonText  string
	NumMedia    int
}

type statusForm struct {
	MessageSID    string `validate:"required"`
	MessageStatus string `validate:"required"`
	ErrorCode     string
	To            string
}

var statusMapping = map[string]courier.MsgStatus{
	"queued":      courier.MsgStatusSent,
	"failed":      courier.MsgStatusFailed,
	"sent":        courier.MsgStatusSent,
	"delivered":   courier.MsgStatusDelivered,
	"read":        courier.MsgStatusDelivered,
	"undelivered": courier.MsgStatusFailed,
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
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

	urn, err := h.parseURN(channel, form.From, form.FromCountry)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	if form.Body != "" {
		// Twilio sometimes sends concatenated sms as base64 encoded MMS
		form.Body = handlers.DecodePossibleBase64(form.Body)
	}

	text := form.Body
	if channel.IsScheme(urns.WhatsAppScheme) && form.ButtonText != "" {
		text = form.ButtonText
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, text, form.MessageSID, clog)

	// process any attached media
	for i := 0; i < form.NumMedia; i++ {
		mediaURL := r.PostForm.Get(fmt.Sprintf("MediaUrl%d", i))
		msg.WithAttachment(mediaURL)
	}
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
}

// receiveStatus is our HTTP handler function for status updates
func (h *handler) receiveStatus(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
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
	if channel.BoolConfigForKey(configIgnoreDLRs, false) && msgStatus != courier.MsgStatusFailed {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring non error delivery report")
	}

	// if the message id was passed explicitely, use that
	var status courier.StatusUpdate
	idString := r.URL.Query().Get("id")
	if idString != "" {
		msgID, err := strconv.ParseInt(idString, 10, 64)
		if err != nil {
			slog.Error("error converting twilio callback id to integer", "error", err, "id", idString)
		} else {
			status = h.Backend().NewStatusUpdate(channel, courier.MsgID(msgID), msgStatus, clog)
		}
	}

	// if we have no status, then build it from the external (twilio) id
	if status == nil {
		status = h.Backend().NewStatusUpdateByExternalID(channel, form.MessageSID, msgStatus, clog)
	}

	errorCode, _ := strconv.ParseInt(form.ErrorCode, 10, 64)
	if errorCode != 0 {
		if errorCode == errorStopped {
			urn, err := h.parseURN(channel, form.To, "")
			if err != nil {
				return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
			}

			// create a stop channel event
			channelEvent := h.Backend().NewChannelEvent(channel, courier.EventTypeStopContact, urn, clog)
			err = h.Backend().WriteChannelEvent(ctx, channelEvent, clog)
			if err != nil {
				return nil, err
			}
		}
		clog.Error(twilioError(errorCode))
	}

	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	// build our callback URL
	callbackDomain := msg.Channel().CallbackDomain(h.Server().Config().Domain)
	callbackURL := fmt.Sprintf("https://%s/c/%s/%s/status?id=%d&action=callback", callbackDomain, strings.ToLower(string(h.ChannelType())), msg.Channel().UUID(), msg.ID())

	accountSID := msg.Channel().StringConfigForKey(configAccountSID, "")
	if accountSID == "" {
		return nil, fmt.Errorf("missing account sid for %s channel", h.ChannelName())
	}

	accountToken := msg.Channel().StringConfigForKey(courier.ConfigAuthToken, "")
	if accountToken == "" {
		return nil, fmt.Errorf("missing account auth token for %s channel", h.ChannelName())
	}

	channel := msg.Channel()

	attachments, err := handlers.ResolveAttachments(ctx, h.Backend(), msg.Attachments(), mediaSupport, true)
	if err != nil {
		return nil, errors.Wrap(err, "error resolving attachments")
	}

	status := h.Backend().NewStatusUpdate(channel, msg.ID(), courier.MsgStatusErrored, clog)
	parts := handlers.SplitMsgByChannel(msg.Channel(), msg.Text(), maxMsgLength)
	for i, part := range parts {
		// build our request
		form := url.Values{
			"To":             []string{msg.URN().Path()},
			"Body":           []string{part},
			"StatusCallback": []string{callbackURL},
		}

		// add any attachments to the first part
		if i == 0 {
			for _, a := range attachments {
				form.Add("MediaUrl", a.URL)
			}
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

		req, err := http.NewRequest(http.MethodPost, sendURL, strings.NewReader(form.Encode()))
		if err != nil {
			return nil, err
		}
		req.SetBasicAuth(accountSID, accountToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err != nil {
			return status, nil
		}

		// see if we can parse the error if we have one
		if resp.StatusCode/100 != 2 && len(respBody) > 0 {
			errorCode, _ := jsonparser.GetInt(respBody, "code")
			if errorCode != 0 {
				if errorCode == errorStopped {
					status.SetStatus(courier.MsgStatusFailed)

					// create a stop channel event
					channelEvent := h.Backend().NewChannelEvent(msg.Channel(), courier.EventTypeStopContact, msg.URN(), clog)
					err = h.Backend().WriteChannelEvent(ctx, channelEvent, clog)
					if err != nil {
						return nil, err
					}
				}
				clog.Error(twilioError(errorCode))
				return status, nil
			}
		}

		// grab the external id
		externalID, err := jsonparser.GetString(respBody, "sid")
		if err != nil {
			clog.Error(courier.ErrorResponseValueMissing("sid"))
			return status, nil
		}

		status.SetStatus(courier.MsgStatusWired)

		// only save the first external id
		if i == 0 {
			status.SetExternalID(externalID)
		}
	}

	return status, nil
}

// BuildAttachmentRequest to download media for message attachment with Basic auth set
func (h *handler) BuildAttachmentRequest(ctx context.Context, b courier.Backend, channel courier.Channel, attachmentURL string, clog *courier.ChannelLog) (*http.Request, error) {
	accountSID := channel.StringConfigForKey(configAccountSID, "")
	if accountSID == "" {
		return nil, fmt.Errorf("missing account sid for %s channel", h.ChannelName())
	}

	accountToken := channel.StringConfigForKey(courier.ConfigAuthToken, "")
	if accountToken == "" {
		return nil, fmt.Errorf("missing account auth token for %s channel", h.ChannelName())
	}

	req, _ := http.NewRequest(http.MethodGet, attachmentURL, nil)

	if h.validateSignatures {
		// set the basic auth token as the authorization header
		req.SetBasicAuth(accountSID, accountToken)
	}
	return req, nil
}

func (h *handler) RedactValues(ch courier.Channel) []string {
	return []string{
		httpx.BasicAuth(ch.StringConfigForKey(configAccountSID, ""), ch.StringConfigForKey(courier.ConfigAuthToken, "")),
	}
}

func (h *handler) parseURN(channel courier.Channel, text, country string) (urns.URN, error) {
	if channel.IsScheme(urns.WhatsAppScheme) {
		// Twilio Whatsapp from is in the form: whatsapp:+12211414154 or +12211414154
		var fromTel string
		parts := strings.Split(text, ":")
		if len(parts) > 1 {
			fromTel = parts[1]
		} else {
			fromTel = parts[0]
		}

		// trim off left +, official whatsapp IDs dont have that
		return urns.NewWhatsAppURN(strings.TrimLeft(fromTel, "+"))
	}

	return urns.NewTelURNForCountry(text, country)
}

func (h *handler) baseURL(c courier.Channel) string {
	// Twilio channels use the Twili base URL
	if c.ChannelType() == "T" || c.ChannelType() == "TMS" || c.ChannelType() == "TWA" {
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
func (h *handler) WriteMsgSuccessResponse(ctx context.Context, w http.ResponseWriter, msgs []courier.MsgIn) error {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(200)
	_, err := fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><Response/>`)
	return err
}

// WriteRequestIgnored writes our response in TWIML format
func (h *handler) WriteRequestIgnored(ctx context.Context, w http.ResponseWriter, details string) error {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(200)
	_, err := fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><!-- %s --><Response/>`, details)
	return err
}

// https://www.twilio.com/docs/api/errors
func twilioError(code int64) *courier.ChannelError {
	codeAsStr := strconv.Itoa(int(code))
	errMsg, _ := jsonparser.GetString(errorCodes, codeAsStr)
	return courier.ErrorExternal(codeAsStr, errMsg)
}
