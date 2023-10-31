package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/urns"
	"github.com/pkg/errors"
)

const (
	jsonMimeTypeType   = "application/json"
	urlEncodedMimeType = "application/x-www-form-urlencoded"
)

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
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)

	sentHandler := h.buildStatusHandler("sent")
	s.AddHandlerRoute(h, http.MethodPost, "sent", courier.ChannelLogTypeMsgStatus, sentHandler)

	deliveredHandler := h.buildStatusHandler("delivered")
	s.AddHandlerRoute(h, http.MethodPost, "delivered", courier.ChannelLogTypeMsgStatus, deliveredHandler)

	failedHandler := h.buildStatusHandler("failed")
	s.AddHandlerRoute(h, http.MethodPost, "failed", courier.ChannelLogTypeMsgStatus, failedHandler)

	return nil
}

// utility function to grab the form value for either the passed in name (if non-empty) or the first set
// value from defaultNames
func getFormField(form url.Values, name string) string {
	if name != "" {
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

	var from, text string

	// parse our form
	err = r.ParseForm()
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, errors.Wrapf(err, "invalid request"))
	}

	from = getFormField(r.Form, "from")
	text = getFormField(r.Form, "text")

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
	urn, err := urns.NewURNFromParts(urns.DiscordScheme, from, "", "")
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, text, "", clog).WithReceivedOn(date)

	for _, attachment := range r.Form["attachments"] {
		msg.WithAttachment(attachment)
	}

	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
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
	sendURL := msg.Channel().StringConfigForKey(courier.ConfigSendURL, "")
	if sendURL == "" {
		return nil, fmt.Errorf("no send url set for DS channel")
	}

	// figure out what encoding to tell kannel to send as
	sendMethod := http.MethodPost
	// sendBody := msg.Channel().StringConfigForKey(courier.ConfigSendBody, "")
	contentTypeHeader := jsonMimeTypeType

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)
	attachmentURLs := []string{}
	for _, attachment := range msg.Attachments() {
		_, attachmentURL := handlers.SplitAttachment(attachment)
		attachmentURLs = append(attachmentURLs, attachmentURL)
	}
	// build our request
	type OutputMessage struct {
		ID           string   `json:"id"`
		Text         string   `json:"text"`
		To           string   `json:"to"`
		Channel      string   `json:"channel"`
		Attachments  []string `json:"attachments"`
		QuickReplies []string `json:"quick_replies"`
	}

	ourMessage := OutputMessage{
		ID:           msg.ID().String(),
		Text:         msg.Text(),
		To:           msg.URN().Path(),
		Channel:      string(msg.Channel().UUID()),
		Attachments:  attachmentURLs,
		QuickReplies: msg.QuickReplies(),
	}

	var body io.Reader
	marshalled, err := json.Marshal(ourMessage)
	if err != nil {
		return nil, err
	}
	body = bytes.NewReader(marshalled)

	req, err := http.NewRequest(sendMethod, sendURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentTypeHeader)

	authorization := msg.Channel().StringConfigForKey(courier.ConfigSendAuthorization, "")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}

	resp, _, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return status, nil
	}

	// If we don't have an error, set the message as wired and move on
	status.SetStatus(courier.MsgStatusWired)

	return status, nil
}
