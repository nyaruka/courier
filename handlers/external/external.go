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
	"time"

	"strings"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
)

const (
	contentURLEncoded = "application/x-www-form-urlencoded"
	contentJSON       = "application/json"
	contentXML        = "text/xml; charset=utf-8"
)

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

	return nil
}

type moForm struct {
	From   string `name:"from"`
	Sender string `name:"sender"`
	Text   string `validate:"required" name:"text"`
	Date   string `name:"date"`
	Time   string `name:"time"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	form := &moForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	// must have one of from or sender set, error if neither
	sender := form.Sender
	if sender == "" {
		sender = form.From
	}
	if sender == "" {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf("must have one of 'sender' or 'from' set"))
	}

	// if we have a date, parse it
	dateString := form.Date
	if dateString == "" {
		dateString = form.Time
	}

	date := time.Now()
	if dateString != "" {
		date, err = time.Parse(time.RFC3339Nano, dateString)
		if err != nil {
			return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf("invalid date format, must be RFC 3339"))
		}
	}

	// create our URN
	urn := urns.NilURN
	if channel.Schemes()[0] == urns.TelScheme {
		urn, err = urns.NewTelURNForCountry(sender, channel.Country())
	} else {
		urn, err = urns.NewURNFromParts(channel.Schemes()[0], sender, "")
	}
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}
	urn, _ = urn.Normalize("")

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, form.Text).WithReceivedOn(date)

	// and finally write our message
	return handlers.WriteMsgAndResponse(ctx, h, msg, w, r)
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
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	// get our status
	msgStatus, found := statusMappings[strings.ToLower(statusString)]
	if !found {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf("unknown status '%s', must be one failed, sent or delivered", statusString))
	}

	// write our status
	status := h.Backend().NewMsgStatusForID(channel, courier.NewMsgID(form.ID), msgStatus)
	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	sendURL := msg.Channel().StringConfigForKey(courier.ConfigSendURL, "")
	if sendURL == "" {
		return nil, fmt.Errorf("no send url set for EX channel")
	}

	sendMethod := msg.Channel().StringConfigForKey(courier.ConfigSendMethod, http.MethodPost)
	sendBody := msg.Channel().StringConfigForKey(courier.ConfigSendBody, "")
	contentType := msg.Channel().StringConfigForKey(courier.ConfigContentType, contentURLEncoded)

	maxLength := msg.Channel().IntConfigForKey(courier.ConfigMaxLength, 160)
	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	parts := handlers.SplitMsg(handlers.GetTextAndAttachments(msg), maxLength)
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

		url := replaceVariables(sendURL, form, contentURLEncoded)
		var body io.Reader
		if sendMethod == http.MethodPost || sendMethod == http.MethodPut {
			body = strings.NewReader(replaceVariables(sendBody, form, contentType))
		}

		req, err := http.NewRequest(sendMethod, url, body)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", contentType)

		authorization := msg.Channel().StringConfigForKey(courier.ConfigSendAuthorization, "")
		if authorization != "" {
			req.Header.Set("Authorization", authorization)
		}

		rr, err := utils.MakeHTTPRequest(req)

		// record our status and log
		status.AddLog(courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err))
		if err != nil {
			return status, nil
		}

		status.SetStatus(courier.MsgWired)
	}

	return status, nil
}

func replaceVariables(text string, variables map[string]string, contentType string) string {
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

		text = strings.Replace(text, fmt.Sprintf("{{%s}}", k), v, -1)
	}
	return text
}

const defaultSendBody = `id={{id}}&text={{text}}&to={{to}}&to_no_plus={{to_no_plus}}&from={{from}}&from_no_plus={{from_no_plus}}&channel={{channel}}`
