package external

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/antchfx/xmlquery"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/gsm7"
	"github.com/nyaruka/gocommon/urns"
	"github.com/pkg/errors"
)

const (
	contentURLEncoded = "urlencoded"
	contentJSON       = "json"
	contentXML        = "xml"

	configFromXPath = "from_xpath"
	configTextXPath = "text_xpath"

	configMOFromField = "mo_from_field"
	configMOTextField = "mo_text_field"
	configMODateField = "mo_date_field"

	configMOResponseContentType = "mo_response_content_type"
	configMOResponse            = "mo_response"

	configMTResponseCheck = "mt_response_check"
	configEncoding        = "encoding"
	encodingDefault       = "D"
	encodingSmart         = "S"
)

var defaultFromFields = []string{"from", "sender"}
var defaultTextFields = []string{"text"}
var defaultDateFields = []string{"date", "time"}

var contentTypeMappings = map[string]string{
	contentURLEncoded: "application/x-www-form-urlencoded",
	contentJSON:       "application/json",
	contentXML:        "text/xml; charset=utf-8",
}

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("EX"), "External")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodGet, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)

	sentHandler := h.buildStatusHandler("sent")
	s.AddHandlerRoute(h, http.MethodGet, "sent", courier.ChannelLogTypeMsgStatus, sentHandler)
	s.AddHandlerRoute(h, http.MethodPost, "sent", courier.ChannelLogTypeMsgStatus, sentHandler)

	deliveredHandler := h.buildStatusHandler("delivered")
	s.AddHandlerRoute(h, http.MethodGet, "delivered", courier.ChannelLogTypeMsgStatus, deliveredHandler)
	s.AddHandlerRoute(h, http.MethodPost, "delivered", courier.ChannelLogTypeMsgStatus, deliveredHandler)

	failedHandler := h.buildStatusHandler("failed")
	s.AddHandlerRoute(h, http.MethodGet, "failed", courier.ChannelLogTypeMsgStatus, failedHandler)
	s.AddHandlerRoute(h, http.MethodPost, "failed", courier.ChannelLogTypeMsgStatus, failedHandler)

	s.AddHandlerRoute(h, http.MethodPost, "stopped", courier.ChannelLogTypeEventReceive, h.receiveStopContact)
	s.AddHandlerRoute(h, http.MethodGet, "stopped", courier.ChannelLogTypeEventReceive, h.receiveStopContact)

	return nil
}

type stopContactForm struct {
	From string `validate:"required" name:"from"`
}

func (h *handler) receiveStopContact(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	form := &stopContactForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// create our URN
	urn := urns.NilURN
	if channel.Schemes()[0] == urns.TelScheme {
		urn, err = handlers.StrictTelForCountry(form.From, channel.Country())
	} else {
		urn, err = urns.NewURNFromParts(channel.Schemes()[0], form.From, "", "")
	}
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}
	urn = urn.Normalize("")

	// create a stop channel event
	channelEvent := h.Backend().NewChannelEvent(channel, courier.EventTypeStopContact, urn, clog)
	err = h.Backend().WriteChannelEvent(ctx, channelEvent, clog)
	if err != nil {
		return nil, err
	}
	return []courier.Event{channelEvent}, courier.WriteChannelEventSuccess(w, channelEvent)
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
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	var err error

	var from, dateString, text string

	fromXPath := channel.StringConfigForKey(configFromXPath, "")
	textXPath := channel.StringConfigForKey(configTextXPath, "")

	if fromXPath != "" && textXPath != "" {
		// we are reading from an XML body, pull out our fields
		body, err := io.ReadAll(io.LimitReader(r.Body, 100000))
		defer r.Body.Close()
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unable to read request body: %s", err))
		}

		doc, err := xmlquery.Parse(strings.NewReader(string(body)))
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unable to parse request XML: %s", err))
		}
		fromNode := xmlquery.FindOne(doc, fromXPath)
		textNode := xmlquery.FindOne(doc, textXPath)
		if fromNode == nil || textNode == nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("missing from at: %s or text at: %s node", fromXPath, textXPath))
		}

		from = fromNode.InnerText()
		text = textNode.InnerText()
	} else {
		// parse our form
		contentType := r.Header.Get("Content-Type")
		var err error
		if strings.Contains(contentType, "multipart/form-data") {
			err = r.ParseMultipartForm(10000000)
		} else {
			err = r.ParseForm()
		}
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, errors.Wrapf(err, "invalid request"))
		}

		from = getFormField(r.Form, defaultFromFields, channel.StringConfigForKey(configMOFromField, ""))
		text = getFormField(r.Form, defaultTextFields, channel.StringConfigForKey(configMOTextField, ""))
		dateString = getFormField(r.Form, defaultDateFields, channel.StringConfigForKey(configMODateField, ""))
	}

	// must have from field
	if from == "" {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("must have one of 'sender' or 'from' set"))
	}

	// if we have a date, parse it
	date := time.Now()
	if dateString != "" {
		date, err = time.Parse(time.RFC3339Nano, dateString)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("invalid date format, must be RFC 3339"))
		}
	}

	// create our URN
	urn := urns.NilURN
	if channel.Schemes()[0] == urns.TelScheme {
		urn, err = handlers.StrictTelForCountry(from, channel.Country())
	} else {
		urn, err = urns.NewURNFromParts(channel.Schemes()[0], from, "", "")
	}
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}
	urn = urn.Normalize(channel.Country())

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, text, "", clog).WithReceivedOn(date)

	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
}

// WriteMsgSuccessResponse writes our response in TWIML format
func (h *handler) WriteMsgSuccessResponse(ctx context.Context, w http.ResponseWriter, msgs []courier.MsgIn) error {
	moResponse := msgs[0].Channel().StringConfigForKey(configMOResponse, "")
	if moResponse == "" {
		return courier.WriteMsgSuccess(w, msgs)
	}
	moResponseContentType := msgs[0].Channel().StringConfigForKey(configMOResponseContentType, "")
	if moResponseContentType != "" {
		w.Header().Set("Content-Type", moResponseContentType)
	}
	w.WriteHeader(200)
	_, err := fmt.Fprint(w, moResponse)
	return err
}

// buildStatusHandler deals with building a handler that takes what status is received in the URL
func (h *handler) buildStatusHandler(status string) courier.ChannelHandleFunc {
	return func(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
		return h.receiveStatus(ctx, status, channel, w, r, clog)
	}
}

type statusForm struct {
	ID int64 `name:"id" validate:"required"`
}

var statusMappings = map[string]courier.MsgStatus{
	"failed":    courier.MsgStatusFailed,
	"sent":      courier.MsgStatusSent,
	"delivered": courier.MsgStatusDelivered,
}

// receiveStatus is our HTTP handler function for status updates
func (h *handler) receiveStatus(ctx context.Context, statusString string, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
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
	status := h.Backend().NewStatusUpdate(channel, courier.MsgID(form.ID), msgStatus, clog)
	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	channel := msg.Channel()

	sendURL := channel.StringConfigForKey(courier.ConfigSendURL, "")
	if sendURL == "" {
		return nil, fmt.Errorf("no send url set for EX channel")
	}

	// figure out what encoding to tell kannel to send as
	encoding := channel.StringConfigForKey(configEncoding, encodingDefault)
	responseCheck := channel.StringConfigForKey(configMTResponseCheck, "")
	sendMethod := channel.StringConfigForKey(courier.ConfigSendMethod, http.MethodPost)
	sendBody := channel.StringConfigForKey(courier.ConfigSendBody, "")
	sendMaxLength := channel.IntConfigForKey(courier.ConfigMaxLength, 160)
	contentType := channel.StringConfigForKey(courier.ConfigContentType, contentURLEncoded)
	contentTypeHeader := contentTypeMappings[contentType]
	if contentTypeHeader == "" {
		contentTypeHeader = contentType
	}

	status := h.Backend().NewStatusUpdate(channel, msg.ID(), courier.MsgStatusErrored, clog)
	parts := handlers.SplitMsgByChannel(channel, handlers.GetTextAndAttachments(msg), sendMaxLength)
	for i, part := range parts {
		// build our request
		form := map[string]string{
			"id":             msg.ID().String(),
			"text":           part,
			"to":             msg.URN().Path(),
			"to_no_plus":     strings.TrimPrefix(msg.URN().Path(), "+"),
			"from":           channel.Address(),
			"from_no_plus":   strings.TrimPrefix(channel.Address(), "+"),
			"channel":        string(channel.UUID()),
			"session_status": msg.SessionStatus(),
		}

		useNationalStr := channel.ConfigForKey(courier.ConfigUseNational, false)
		useNational, _ := useNationalStr.(bool)

		// if we are meant to use national formatting (no country code) pull that out
		if useNational {
			nationalTo := msg.URN().Localize(channel.Country())
			form["to"] = nationalTo.Path()
			form["to_no_plus"] = nationalTo.Path()
		}

		// if we are smart, first try to convert to GSM7 chars
		if encoding == encodingSmart {
			replaced := gsm7.ReplaceSubstitutions(part)
			if gsm7.IsValid(replaced) {
				form["text"] = replaced
			}
		}

		formEncoded := encodeVariables(form, contentURLEncoded)

		// put quick replies on last message part
		if i == len(parts)-1 {
			formEncoded["quick_replies"] = buildQuickRepliesResponse(msg.QuickReplies(), sendMethod, contentURLEncoded)
		} else {
			formEncoded["quick_replies"] = buildQuickRepliesResponse([]string{}, sendMethod, contentURLEncoded)
		}
		url := replaceVariables(sendURL, formEncoded)

		var body io.Reader
		if sendMethod == http.MethodPost || sendMethod == http.MethodPut {
			formEncoded = encodeVariables(form, contentType)

			if i == len(parts)-1 {
				formEncoded["quick_replies"] = buildQuickRepliesResponse(msg.QuickReplies(), sendMethod, contentType)
			} else {
				formEncoded["quick_replies"] = buildQuickRepliesResponse([]string{}, sendMethod, contentType)
			}
			body = strings.NewReader(replaceVariables(sendBody, formEncoded))
		}

		req, err := http.NewRequest(sendMethod, url, body)

		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", contentTypeHeader)

		// TODO can drop this when channels have been migrated to use ConfigSendHeaders
		authorization := channel.StringConfigForKey(courier.ConfigSendAuthorization, "")
		if authorization != "" {
			req.Header.Set("Authorization", authorization)
		}

		headers := channel.ConfigForKey(courier.ConfigSendHeaders, map[string]any{}).(map[string]any)
		for hKey, hValue := range headers {
			req.Header.Set(hKey, fmt.Sprint(hValue))
		}

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 != 2 {
			return status, nil
		}

		if responseCheck == "" || strings.Contains(string(respBody), responseCheck) {
			status.SetStatus(courier.MsgStatusWired)
		} else {
			clog.Error(courier.ErrorResponseUnexpected(responseCheck))
		}
	}

	return status, nil
}

type quickReplyXMLItem struct {
	XMLName xml.Name `xml:"item"`
	Value   string   `xml:",chardata"`
}

func buildQuickRepliesResponse(quickReplies []string, sendMethod string, contentType string) string {
	if quickReplies == nil {
		quickReplies = []string{}
	}
	if (sendMethod == http.MethodPost || sendMethod == http.MethodPut) && contentType == contentJSON {
		marshalled, _ := json.Marshal(quickReplies)
		return string(marshalled)
	} else if (sendMethod == http.MethodPost || sendMethod == http.MethodPut) && contentType == contentXML {
		items := make([]quickReplyXMLItem, len(quickReplies))

		for i, v := range quickReplies {
			items[i] = quickReplyXMLItem{Value: v}
		}
		marshalled, _ := xml.Marshal(items)
		return string(marshalled)
	} else {
		response := bytes.Buffer{}

		for _, reply := range quickReplies {
			reply = url.QueryEscape(reply)
			response.WriteString(fmt.Sprintf("&quick_reply=%s", reply))
		}
		return response.String()
	}
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

		case contentXML:
			buf := &bytes.Buffer{}
			xml.EscapeText(buf, []byte(v))
			v = buf.String()
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
