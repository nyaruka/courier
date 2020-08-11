package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"strings"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
	"github.com/pkg/errors"
)

const (
	contentURLEncoded = "urlencoded"
	contentJSON       = "json"

	configMOFromField = "mo_from_field"
	configMOTextField = "mo_text_field"
	configMODateField = "mo_date_field"

	configMOResponseContentType = "mo_response_content_type"
	configMOResponse            = "mo_response"
)

var defaultFromFields = []string{"from", "sender"}
var defaultTextFields = []string{"text"}
var defaultDateFields = []string{"date", "time"}

var contentTypeMappings = map[string]string{
	contentURLEncoded: "application/x-www-form-urlencoded",
	contentJSON:       "application/json",
}

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("DS"), "Discord")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodGet, "receive", h.receiveMessage)

	sentHandler := h.buildStatusHandler("sent")
	s.AddHandlerRoute(h, http.MethodGet, "sent", sentHandler)
	s.AddHandlerRoute(h, http.MethodPost, "sent", sentHandler)

	deliveredHandler := h.buildStatusHandler("delivered")
	s.AddHandlerRoute(h, http.MethodGet, "delivered", deliveredHandler)
	s.AddHandlerRoute(h, http.MethodPost, "delivered", deliveredHandler)

	failedHandler := h.buildStatusHandler("failed")
	s.AddHandlerRoute(h, http.MethodGet, "failed", failedHandler)
	s.AddHandlerRoute(h, http.MethodPost, "failed", failedHandler)

	s.AddHandlerRoute(h, http.MethodPost, "stopped", h.receiveStopContact)
	s.AddHandlerRoute(h, http.MethodGet, "stopped", h.receiveStopContact)

	return nil
}

type stopContactForm struct {
	From string `validate:"required" name:"from"`
}

func (h *handler) receiveStopContact(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	form := &stopContactForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// create our URN
	urn := urns.NilURN
	urn, err = urns.NewURNFromParts(urns.DiscordScheme, form.From, "", "")
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}
	urn = urn.Normalize("")

	// create a stop channel event
	channelEvent := h.Backend().NewChannelEvent(channel, courier.StopContact, urn)
	err = h.Backend().WriteChannelEvent(ctx, channelEvent)
	if err != nil {
		return nil, err
	}
	return []courier.Event{channelEvent}, courier.WriteChannelEventSuccess(ctx, w, r, channelEvent)
}

// utility function to grab the form value for either the passed in name (if non-empty) or the first set
// value from defaultNames
func getFormField(form url.Values, defaultNames []string, name string) string {
	if name != "" {
		values, found := form[name]
		if found {
			return values[0]
		}
	}

	for _, name := range defaultNames {
		values, found := form[name]
		if found {
			return values[0]
		}
	}

	return ""
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	var err error

	var from, text string

	// parse our form
	err = r.ParseForm()
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, errors.Wrapf(err, "invalid request"))
	}

	from = getFormField(r.Form, defaultFromFields, channel.StringConfigForKey(configMOFromField, ""))
	text = getFormField(r.Form, defaultTextFields, channel.StringConfigForKey(configMOTextField, ""))

	// must have from field
	if from == "" {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("must have one of 'sender' or 'from' set"))
	}
	if text == "" {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("must have 'text' set"))
	}

	// if we have a date, parse it
	date := time.Now()

	// create our URN
	urn := urns.NilURN
	urn, err = urns.NewURNFromParts(urns.DiscordScheme, from, "", "")
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, text).WithReceivedOn(date)

	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r)
}

// WriteMsgSuccessResponse writes our response in TWIML format
func (h *handler) WriteMsgSuccessResponse(ctx context.Context, w http.ResponseWriter, r *http.Request, msgs []courier.Msg) error {
	return courier.WriteMsgSuccess(ctx, w, r, msgs)
}

// buildStatusHandler deals with building a handler that takes what status is received in the URL
func (h *handler) buildStatusHandler(status string) courier.ChannelHandleFunc {
	return func(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
		return h.receiveStatus(ctx, status, channel, w, r)
	}
}

type statusForm struct {
	ID int64 `name:"id" validate:"required"`
}

var statusMappings = map[string]courier.MsgStatusValue{
	"failed":    courier.MsgFailed,
	"sent":      courier.MsgSent,
	"delivered": courier.MsgDelivered,
}

// receiveStatus is our HTTP handler function for status updates
func (h *handler) receiveStatus(ctx context.Context, statusString string, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	form := &statusForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// get our status
	msgStatus, found := statusMappings[strings.ToLower(statusString)]
	if !found {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unknown status '%s', must be one failed, sent or delivered", statusString))
	}

	// write our status
	status := h.Backend().NewMsgStatusForID(channel, courier.NewMsgID(form.ID), msgStatus)
	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	sendURL := msg.Channel().StringConfigForKey(courier.ConfigSendURL, "")
	if sendURL == "" {
		return nil, fmt.Errorf("no send url set for DS channel")
	}

	// figure out what encoding to tell kannel to send as
	sendMethod := http.MethodPost
	// sendBody := msg.Channel().StringConfigForKey(courier.ConfigSendBody, "")
	contentType := contentJSON
	contentTypeHeader := contentTypeMappings[contentType]
	if contentTypeHeader == "" {
		contentTypeHeader = contentType
	}

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	parts := handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), 160)
	for _, part := range parts {
		// build our request
		form := map[string]string{
			"id":           msg.ID().String(),
			"text":         part,
			"to":           msg.URN().Path(),
			"to_no_plus":   strings.TrimPrefix(msg.URN().Path(), "+"),
			"from":         msg.Channel().Address(),
			"from_no_plus": strings.TrimPrefix(msg.Channel().Address(), "+"),
			"channel":      msg.Channel().UUID().String(),
		}

		formEncoded := encodeVariables(form, contentType)

		url := replaceVariables(sendURL, formEncoded)

		var body io.Reader
		if sendMethod == http.MethodPost || sendMethod == http.MethodPut {
			formEncoded = encodeVariables(form, contentType)

			body = strings.NewReader(replaceVariables(sendBody, formEncoded))
		}

		req, err := http.NewRequest(sendMethod, url, body)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", contentTypeHeader)

		authorization := msg.Channel().StringConfigForKey(courier.ConfigSendAuthorization, "")
		if authorization != "" {
			req.Header.Set("Authorization", authorization)
		}

		rr, err := utils.MakeHTTPRequest(req)

		// record our status and log
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
		status.AddLog(log)
		if err != nil {
			return status, nil
		}
		// If we don't have an error, set the message as wired and move on
		status.SetStatus(courier.MsgWired)

	}

	return status, nil
}

func encodeVariables(variables map[string]string, contentType string) map[string]string {
	encoded := make(map[string]string)

	for k, v := range variables {
		// encode according to our content type
		switch contentType {
		case contentJSON:
			marshalled, _ := json.Marshal(v)
			v = string(marshalled)

		case contentURLEncoded:
			v = url.QueryEscape(v)

		}
		encoded[k] = v
	}
	return encoded
}

func replaceVariables(text string, variables map[string]string) string {
	for k, v := range variables {
		text = strings.Replace(text, fmt.Sprintf("{{%s}}", k), v, -1)
	}
	return text
}

// const defaultSendBody = `id={{id}}&text={{text}}&to={{to}}&to_no_plus={{to_no_plus}}&from={{from}}&from_no_plus={{from_no_plus}}&channel={{channel}}`
const sendBody = `{"id": {{id}}, "text": {{text}}, "to":{{to}}, "channel": {{channel}}}`
