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
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/courier/v26/utils"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/svclogs"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/goflow/core/events"
)

const (
	configAccountSID          = "account_sid"
	configMessagingServiceSID = "messaging_service_sid"
	configSendURL             = "send_url"
	configBaseURL             = "base_url"
	configIgnoreDLRs          = "ignore_dlrs"
	configLinkShortening      = "link_shortening"

	signatureHeader     = "X-Twilio-Signature"
	forwardedPathHeader = "X-Forwarded-Path"
)

var (
	maxMsgLength  = 1600
	twilioBaseURL = "https://api.twilio.com"

	typingIndicatorURL = "https://messaging.twilio.com/v3/Indicators/Typing.json"

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
const errorThrottled = 63018

type handler struct {
	handlers.BaseHandler
	validateSignatures bool
}

func newTWIMLHandler(channelType models.ChannelType, name string, validateSignatures bool) channels.Handler {
	return &handler{handlers.NewBaseHandler(channelType, name), validateSignatures}
}

func init() {
	channels.RegisterHandler(newTWIMLHandler("TW", "TWIML API", true))
	channels.RegisterHandler(newTWIMLHandler("T", "Twilio", true))
	channels.RegisterHandler(newTWIMLHandler("TMS", "Twilio Messaging Service", true))
	channels.RegisterHandler(newTWIMLHandler("TWA", "Twilio Whatsapp", true))
	channels.RegisterHandler(newTWIMLHandler("SW", "SignalWire", false))
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(r *channels.Routes) error {
	r.Add(h, http.MethodPost, "receive", models.ChannelLogTypeMsgReceive, h.receiveMessage)
	r.Add(h, http.MethodPost, "status", models.ChannelLogTypeMsgStatus, h.receiveStatus)
	return nil
}

type moForm struct {
	MessageSID     string `validate:"required"`
	AccountSID     string `validate:"required"`
	From           string `validate:"required"`
	FromCountry    string
	To             string `validate:"required"`
	ExternalUserId string
	ToCountry      string
	Body           string
	ButtonText     string
	NumMedia       int
}

type statusForm struct {
	MessageSID    string `validate:"required"`
	MessageStatus string `validate:"required"`
	ErrorCode     string
	To            string
}

var statusMapping = map[string]models.MsgStatus{
	"queued":      models.MsgStatusSent,
	"failed":      models.MsgStatusFailed,
	"sent":        models.MsgStatusSent,
	"delivered":   models.MsgStatusDelivered,
	"read":        models.MsgStatusRead,
	"undelivered": models.MsgStatusFailed,
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel *models.Channel, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
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

	urn, err := h.parseURN(channel, form.From, i18n.Country(form.FromCountry))
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	if form.Body != "" {
		// Twilio sometimes sends concatenated sms as base64 encoded MMS
		form.Body = handlers.DecodePossibleBase64(form.Body)
	}

	text := form.Body
	if channel.IsScheme(urns.WhatsApp) && form.ButtonText != "" {
		text = form.ButtonText
	}

	// build our msg
	msg := models.NewIncomingMsg(channel, urn, text, form.MessageSID, clog)

	if form.ExternalUserId != "" && channel.IsScheme(urns.WhatsApp) {
		userIDURN, urnErr := h.parseURN(channel, form.ExternalUserId, i18n.Country(form.FromCountry))

		if urnErr == nil {
			if userIDURN != urn {
				msg.WithNewURN(userIDURN, models.NewURNAppend)
			}
		} else {
			slog.Warn("ignoring invalid ExternalUserId for message", "ExternalUserId", form.ExternalUserId, "MessageSID", form.MessageSID, "Error", urnErr.Error())

		}
	}

	// process any attached media
	for i := 0; i < form.NumMedia; i++ {
		mediaURL := r.PostForm.Get(fmt.Sprintf("MediaUrl%d", i))
		msg.WithAttachment(mediaURL)
	}
	return handlers.WriteMsgsAndResponse(ctx, h, []*models.MsgIn{msg}, w, r, clog)
}

// receiveStatus is our HTTP handler function for status updates
func (h *handler) receiveStatus(ctx context.Context, channel *models.Channel, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
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
	if channel.BoolConfigForKey(configIgnoreDLRs, false) && msgStatus != models.MsgStatusFailed {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring non error delivery report")
	}

	var status *models.StatusUpdate
	if uuidString := r.URL.Query().Get("uuid"); uuids.Is(uuidString) {
		// if the message UUID was passed explicitely, use that
		status = models.NewStatusUpdate(channel, models.MsgUUID(uuidString), msgStatus, clog)
	} else {
		status = models.NewStatusUpdateByExternalID(channel, form.MessageSID, msgStatus, clog)
	}

	var stopEvent *models.ChannelEvent

	errorCode, _ := strconv.ParseInt(form.ErrorCode, 10, 64)
	if errorCode != 0 {
		if errorCode == errorStopped {
			urn, err := h.parseURN(channel, form.To, "")
			if err != nil {
				return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
			}

			// create a stop channel event
			stopEvent = models.NewChannelEvent(channel, models.EventTypeStopContact, urn, clog)
			err = models.WriteChannelEvent(ctx, h.Runtime(), stopEvent, clog)
			if err != nil {
				return nil, err
			}
		}
		clog.Error(twilioError(errorCode))
		if errorCode == errorThrottled {
			status = models.NewStatusUpdateByExternalID(channel, form.MessageSID, models.MsgStatusErrored, clog)
		}
	}

	events, err := handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
	if stopEvent != nil {
		events = append(events, stopEvent)
	}
	return events, err
}

func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	// build our callback URL
	callbackDomain := msg.Channel().CallbackDomain(h.Runtime().Config.Domain)
	callbackURL := fmt.Sprintf("https://%s/c/%s/%s/status?uuid=%s&action=callback", callbackDomain, strings.ToLower(string(h.ChannelType())), msg.Channel().UUID(), msg.UUID())

	accountSID := msg.Channel().StringConfigForKey(configAccountSID, "")
	accountToken := msg.Channel().StringConfigForKey(models.ConfigAuthToken, "")
	if accountSID == "" || accountToken == "" {
		return channels.ErrChannelConfig
	}

	channel := msg.Channel()

	attachments, err := handlers.ResolveAttachments(ctx, h.Runtime(), msg.Attachments(), mediaSupport, true, clog)
	if err != nil {
		return err
	}

	// do we have a template and support whatsapp scheme?
	if msg.Templating() != nil && channel.IsScheme(urns.WhatsApp) {
		if msg.Templating().ExternalID == "" {
			return channels.ErrMessageInvalid
		}

		form := url.Values{
			"To":             []string{whatsAppAddress(msg.URN())},
			"StatusCallback": []string{callbackURL},
			"ContentSid":     []string{msg.Templating().ExternalID},
		}

		// set our from, either as a messaging service or from our address
		serviceSID := channel.StringConfigForKey(configMessagingServiceSID, "")
		if serviceSID != "" {
			form["MessagingServiceSid"] = []string{serviceSID}
		}

		if channel.Address() != "" {
			form["From"] = []string{fmt.Sprintf("%s:%s", urns.WhatsApp.Prefix, channel.Address())}
		}

		contentVariables := make(map[string]string, len(msg.Templating().Variables))

		for _, comp := range msg.Templating().Components {
			for varKey, varIndex := range comp.Variables {
				value := msg.Templating().Variables[varIndex].Value

				if msg.Templating().Variables[varIndex].Type != "text" {
					_, value = handlers.SplitAttachment(value)
				}

				contentVariables[varKey] = value
			}
		}

		contentVariablesJson := jsonx.MustMarshal(contentVariables)
		if len(contentVariables) > 0 {
			form["ContentVariables"] = []string{string(contentVariablesJson)}
		}
		// build our URL
		baseURL := h.baseURL(channel)
		if baseURL == "" {
			return channels.ErrChannelConfig
		}

		sendURL, err := utils.AddURLPath(baseURL, "2010-04-01", "Accounts", accountSID, "Messages.json")
		if err != nil {
			return err
		}

		req, err := http.NewRequest(http.MethodPost, sendURL, strings.NewReader(form.Encode()))
		if err != nil {
			return err
		}
		req.SetBasicAuth(accountSID, accountToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 == 5 {
			return channels.ErrConnectionFailed
		}
		// Twilio rate limits with a 429 (error 20429) and those requests aren't processed, so retry
		if handlers.IsThrottled(resp) {
			return channels.ErrConnectionThrottled
		}

		// see if we can parse the error if we have one
		if resp.StatusCode/100 != 2 && len(respBody) > 0 {
			errorCode, _ := jsonparser.GetInt(respBody, "code")
			if errorCode != 0 {
				if errorCode == errorStopped {
					return channels.ErrContactStopped
				}
				codeAsStr := strconv.Itoa(int(errorCode))
				errMsg, err := jsonparser.GetString(errorCodes, codeAsStr)
				if err != nil {
					errMsg = fmt.Sprintf("Service specific error: %s.", codeAsStr)
				}
				return channels.ErrFailedWithReason(codeAsStr, errMsg)
			}

			return channels.ErrResponseStatus
		}

		// grab the external id
		externalID, err := jsonparser.GetString(respBody, "sid")
		if err != nil {
			clog.Error(models.ErrorResponseValueMissing("sid"))
		} else {
			res.AddExternalID(externalID)
		}

	} else {

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
				linkShortening := channel.BoolConfigForKey(configLinkShortening, false)
				if linkShortening {
					form["ShortenUrls"] = []string{"true"}
				}

				form["MessagingServiceSid"] = []string{serviceSID}
			}

			if channel.Address() != "" {
				form["From"] = []string{channel.Address()}
			}

			// for whatsapp channels, we have to prepend whatsapp to the To and From
			if channel.IsScheme(urns.WhatsApp) {
				form["To"][0] = whatsAppAddress(msg.URN())
				form["From"][0] = fmt.Sprintf("%s:%s", urns.WhatsApp.Prefix, form["From"][0])
			}

			// build our URL
			baseURL := h.baseURL(channel)
			if baseURL == "" {
				return channels.ErrChannelConfig
			}

			sendURL, err := utils.AddURLPath(baseURL, "2010-04-01", "Accounts", accountSID, "Messages.json")
			if err != nil {
				return err
			}

			req, err := http.NewRequest(http.MethodPost, sendURL, strings.NewReader(form.Encode()))
			if err != nil {
				return err
			}
			req.SetBasicAuth(accountSID, accountToken)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Accept", "application/json")

			resp, respBody, err := h.RequestHTTP(req, clog)
			if err != nil || resp.StatusCode/100 == 5 {
				return channels.ErrConnectionFailed
			}
			// Twilio rate limits with a 429 (error 20429) and those requests aren't processed, so retry
			if handlers.IsThrottled(resp) {
				return channels.ErrConnectionThrottled
			}

			// see if we can parse the error if we have one
			if resp.StatusCode/100 != 2 && len(respBody) > 0 {
				errorCode, _ := jsonparser.GetInt(respBody, "code")
				if errorCode != 0 {
					if errorCode == errorStopped {
						return channels.ErrContactStopped
					}
					codeAsStr := strconv.Itoa(int(errorCode))
					errMsg, err := jsonparser.GetString(errorCodes, codeAsStr)
					if err != nil {
						errMsg = fmt.Sprintf("Service specific error: %s.", codeAsStr)
					}
					return channels.ErrFailedWithReason(codeAsStr, errMsg)
				}

				return channels.ErrResponseStatus
			}

			// grab the external id
			externalID, err := jsonparser.GetString(respBody, "sid")
			if err != nil {
				clog.Error(models.ErrorResponseValueMissing("sid"))
			} else {
				res.AddExternalID(externalID)
			}
		}

	}

	return nil
}

// WhatsApp displays typing indicators for up to 25 seconds or until a reply is sent - other channel
// types have no typing indicator support
var sendableEvents = map[models.ChannelType]map[string]time.Duration{
	"TWA": {events.TypeTypingStarted: 20 * time.Second},
}

// SendableEvents declares support for typing indicators on WhatsApp channels
func (h *handler) SendableEvents(*models.Channel) map[string]time.Duration {
	return sendableEvents[h.ChannelType()]
}

// SendEvent sends a typing started event as a typing indicator, which Twilio implements as marking the
// referenced incoming message as read and displaying an indicator until a reply is sent.
// See https://www.twilio.com/docs/whatsapp/api/typing-indicators-resource
func (h *handler) SendEvent(ctx context.Context, ch *models.Channel, event events.Event, clog *models.ChannelLog) error {
	typing, ok := event.(*events.TypingStarted)
	if !ok {
		return fmt.Errorf("unsupported event type: %s", event.Type())
	}
	if typing.MsgExternalID == "" {
		return fmt.Errorf("%s event requires msg_external_id", event.Type())
	}

	accountSID := ch.StringConfigForKey(configAccountSID, "")
	accountToken := ch.StringConfigForKey(models.ConfigAuthToken, "")
	if accountSID == "" || accountToken == "" {
		return channels.ErrChannelConfig
	}

	payload := &struct {
		Channel   string `json:"channel"`
		MessageID string `json:"messageId"`
	}{Channel: "WHATSAPP", MessageID: typing.MsgExternalID}

	req, err := http.NewRequest(http.MethodPost, typingIndicatorURL, bytes.NewReader(jsonx.MustMarshal(payload)))
	if err != nil {
		return err
	}
	req.SetBasicAuth(accountSID, accountToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 == 5 {
		return channels.ErrConnectionFailed
	}

	if handlers.IsThrottled(resp) {
		return channels.ErrConnectionThrottled
	}

	response := &struct {
		Success bool `json:"success"`
	}{}
	if err := json.Unmarshal(respBody, response); err != nil || resp.StatusCode/100 != 2 || !response.Success {
		return channels.ErrResponseStatus
	}

	return nil
}

// BuildAttachmentRequest to download media for message attachment with Basic auth set
func (h *handler) BuildAttachmentRequest(ctx context.Context, channel *models.Channel, attachmentURL string, clog *models.ChannelLog) (*http.Request, error) {
	accountSID := channel.StringConfigForKey(configAccountSID, "")
	if accountSID == "" {
		return nil, fmt.Errorf("missing account sid for %s channel", h.ChannelName())
	}

	accountToken := channel.StringConfigForKey(models.ConfigAuthToken, "")
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

func (h *handler) RedactValues(ch *models.Channel) []string {
	return []string{
		httpx.BasicAuth(ch.StringConfigForKey(configAccountSID, ""), ch.StringConfigForKey(models.ConfigAuthToken, "")),
	}
}

func (h *handler) parseURN(channel *models.Channel, text string, country i18n.Country) (urns.URN, error) {
	if channel.IsScheme(urns.WhatsApp) {
		// Twilio Whatsapp from is in the form: whatsapp:+12211414154 or +12211414154
		var fromTel string
		parts := strings.Split(text, ":")
		if len(parts) > 1 {
			fromTel = parts[1]
		} else {
			fromTel = parts[0]
		}

		if dot := strings.Index(fromTel, "."); dot >= 0 && dot < len(fromTel)-1 {
			// a business-scoped user ID becomes a whatsapp URN
			return urns.New(urns.WhatsApp, fromTel)
		}

		// trim off left +, official whatsapp IDs dont have that
		return urns.New(urns.WhatsApp, strings.TrimLeft(fromTel, "+"))
	}

	return urns.ParsePhone(text, country, true, true)
}

// whatsAppAddress formats the given URN as a Twilio WhatsApp address: whatsapp:+<digits> for a phone number
// or whatsapp:<CC.xxx> for a business-scoped user ID, which takes no leading +
func whatsAppAddress(urn urns.URN) string {
	if urn.Scheme() == urns.BSUID.Prefix || urns.IsWhatsAppBSUID(urn) {
		return fmt.Sprintf("%s:%s", urns.WhatsApp.Prefix, urn.Path())
	}
	return fmt.Sprintf("%s:+%s", urns.WhatsApp.Prefix, urn.Path())
}

func (h *handler) baseURL(c *models.Channel) string {
	// Twilio channels use the Twili base URL
	if c.ChannelType() == "T" || c.ChannelType() == "TMS" || c.ChannelType() == "TWA" {
		return twilioBaseURL
	}

	return c.StringConfigForKey(configSendURL, c.StringConfigForKey(configBaseURL, ""))
}

// see https://www.twilio.com/docs/api/security
func (h *handler) validateSignature(c *models.Channel, r *http.Request) error {
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

	confAuth := c.ConfigForKey(models.ConfigAuthToken, "")
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
func (h *handler) WriteMsgSuccessResponse(ctx context.Context, w http.ResponseWriter, msgs []*models.MsgIn) error {
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
func twilioError(code int64) *svclogs.Error {
	codeAsStr := strconv.Itoa(int(code))
	errMsg, _ := jsonparser.GetString(errorCodes, codeAsStr)
	return models.ErrorExternal(codeAsStr, errMsg)
}
