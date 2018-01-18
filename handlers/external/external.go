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
	"strconv"
	"time"

	"strings"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
	"github.com/pkg/errors"
)

const contentURLEncoded = "application/x-www-form-urlencoded"
const contentJSON = "application/json"
const contentXML = "text/xml; charset=utf-8"

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
	s.AddHandlerRoute(h, "POST", "receive", h.ReceiveMessage)
	s.AddHandlerRoute(h, "GET", "receive", h.ReceiveMessage)

	sentHandler := h.buildStatusHandler("sent")
	s.AddHandlerRoute(h, "GET", "sent", sentHandler)
	s.AddHandlerRoute(h, "POST", "sent", sentHandler)

	deliveredHandler := h.buildStatusHandler("delivered")
	s.AddHandlerRoute(h, "GET", "delivered", deliveredHandler)
	s.AddHandlerRoute(h, "POST", "delivered", deliveredHandler)

	failedHandler := h.buildStatusHandler("failed")
	s.AddHandlerRoute(h, "GET", "failed", failedHandler)
	s.AddHandlerRoute(h, "POST", "failed", failedHandler)

	return nil
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *handler) ReceiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	externalMessage := &externalMessage{}
	handlers.DecodeAndValidateQueryParams(externalMessage, r)

	// if this is a post, also try to parse the form body
	if r.Method == http.MethodPost {
		handlers.DecodeAndValidateForm(externalMessage, r)
	}

	// validate whether our required fields are present
	err := handlers.Validate(externalMessage)
	if err != nil {
		return nil, err
	}

	// must have one of from or sender set, error if neither
	sender := externalMessage.Sender
	if sender == "" {
		sender = externalMessage.From
	}
	if sender == "" {
		return nil, errors.New("must have one of 'sender' or 'from' set")
	}

	// if we have a date, parse it
	dateString := externalMessage.Date
	if dateString == "" {
		dateString = externalMessage.Time
	}

	date := time.Now()
	if dateString != "" {
		date, err = time.Parse(time.RFC3339Nano, dateString)
		if err != nil {
			return nil, errors.New("invalid date format, must be RFC 3339")
		}
	}

	// create our URN
	urn := urns.NewURNFromParts(channel.Schemes()[0], sender, "").Normalize("")

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, externalMessage.Text).WithReceivedOn(date)

	// and write it
	err = h.Backend().WriteMsg(ctx, msg)
	if err != nil {
		return nil, err
	}

	return []courier.Event{msg}, courier.WriteMsgSuccess(ctx, w, r, []courier.Msg{msg})
}

type externalMessage struct {
	From   string `name:"from"`
	Sender string `name:"sender"`
	Text   string `validate:"required" name:"text"`
	Date   string `name:"date"`
	Time   string `name:"time"`
}

// buildStatusHandler deals with building a handler that takes what status is received in the URL
func (h *handler) buildStatusHandler(status string) courier.ChannelHandleFunc {
	return func(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
		return h.StatusMessage(ctx, status, channel, w, r)
	}
}

// StatusMessage is our HTTP handler function for status updates
func (h *handler) StatusMessage(ctx context.Context, statusString string, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	statusForm := &statusForm{}
	handlers.DecodeAndValidateQueryParams(statusForm, r)

	// if this is a post, also try to parse the form body
	if r.Method == http.MethodPost {
		handlers.DecodeAndValidateForm(statusForm, r)
	}

	// validate whether our required fields are present
	err := handlers.Validate(statusForm)
	if err != nil {
		return nil, err
	}

	// get our id
	msgStatus, found := statusMappings[strings.ToLower(statusString)]
	if !found {
		return nil, fmt.Errorf("unknown status '%s', must be one failed, sent or delivered", statusString)
	}

	// write our status
	status := h.Backend().NewMsgStatusForID(channel, courier.NewMsgID(statusForm.ID), msgStatus)
	err = h.Backend().WriteMsgStatus(ctx, status)
	if err != nil {
		return nil, err
	}

	return []courier.Event{status}, courier.WriteStatusSuccess(ctx, w, r, []courier.MsgStatus{status})
}

type statusForm struct {
	ID int64 `name:"id" validate:"required"`
}

var statusMappings = map[string]courier.MsgStatusValue{
	"failed":    courier.MsgFailed,
	"sent":      courier.MsgSent,
	"delivered": courier.MsgDelivered,
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

	maxLengthStr := msg.Channel().StringConfigForKey(courier.ConfigMaxLength, "160")
	maxLength, err := strconv.Atoi(maxLengthStr)
	if err != nil {
		return nil, fmt.Errorf("invalid value for max length on EX channel %s: %s", msg.Channel().UUID(), maxLengthStr)
	}

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	parts := handlers.SplitMsg(courier.GetTextAndAttachments(msg), maxLength)
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
