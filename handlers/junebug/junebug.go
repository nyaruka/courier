package junebug

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/pkg/errors"
)

var (
	maxMsgLength = 1530
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("JN"), "Junebug")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "event", h.receiveEvent)
	s.AddHandlerRoute(h, http.MethodPost, "inbound", h.receiveMessage)
	return nil
}

// {
//   "from": "+27123456789",
//   "timestamp": "2017-01-01 00:00:00.00",
//   "content": "content",
//   "to": "to-addr",
//   "reply_to": null,
//   "message_id": "message-id"
// }
type moPayload struct {
	From      string `json:"from"       validate:"required"`
	Timestamp string `json:"timestamp"  validate:"required"`
	Content   string `json:"content"`
	To        string `json:"to"         validate:"required"`
	ReplyTo   string `json:"reply_to"`
	MessageID string `json:"message_id" validate:"required"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, c courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	payload := &moPayload{}
	err := handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
	}

	// check authentication
	secret := c.StringConfigForKey(courier.ConfigSecret, "")
	if secret != "" {
		authorization := r.Header.Get("Authorization")
		if authorization != fmt.Sprintf("Token %s", secret) {
			return nil, courier.WriteAndLogUnauthorized(ctx, w, r, c, fmt.Errorf("invalid Authorization header"))
		}
	}

	// parse our date
	date, err := time.Parse("2006-01-02 15:04:05", payload.Timestamp)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, fmt.Errorf("unable to parse date: %s", payload.Timestamp))
	}

	urn, err := handlers.StrictTelForCountry(payload.From, c.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
	}

	msg := h.Backend().NewIncomingMsg(c, urn, payload.Content).WithExternalID(payload.MessageID).WithReceivedOn(date.UTC())

	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r)
}

// {
//   'event_type': 'submitted',
//   'message_id': 'message-id',
//   'timestamp': '2017-01-01 00:00:00+0000',
// }
type eventPayload struct {
	EventType string `json:"event_type" validate:"required"`
	MessageID string `json:"message_id" validate:"required"`
}

var statusMapping = map[string]courier.MsgStatusValue{
	"submitted":          courier.MsgSent,
	"delivery_pending":   courier.MsgWired,
	"delivery_succeeded": courier.MsgDelivered,
	"delivery_failed":    courier.MsgFailed,
	"rejected":           courier.MsgFailed,
}

// receiveEvent is our HTTP handler function for incoming events
func (h *handler) receiveEvent(ctx context.Context, c courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	payload := &eventPayload{}
	err := handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
	}

	// check authentication
	secret := c.StringConfigForKey(courier.ConfigSecret, "")
	if secret != "" {
		authorization := r.Header.Get("Authorization")
		if authorization != fmt.Sprintf("Token %s", secret) {
			return nil, courier.WriteAndLogUnauthorized(ctx, w, r, c, fmt.Errorf("invalid Authorization header"))
		}
	}

	// look up our status
	msgStatus, found := statusMapping[payload.EventType]
	if !found {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, c, w, r, "ignoring unknown event_type")
	}

	// ignore pending, same status we are already in
	if msgStatus == courier.MsgWired {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, c, w, r, "ignoring existing pending status")
	}

	status := h.Backend().NewMsgStatusForExternalID(c, payload.MessageID, msgStatus)
	return handlers.WriteMsgStatusAndResponse(ctx, h, c, status, w, r)
}

// {
//     "event_url": "https://callback.com/event",
//     "content": "hello world",
//     "from": "2020",
//     "to": "+250788383383",
//     "event_auth_token": "secret",
// }
type mtPayload struct {
	EventURL       string `json:"event_url"`
	Content        string `json:"content"`
	From           string `json:"from"`
	To             string `json:"to"`
	EventAuthToken string `json:"event_auth_token,omitempty"`
}

func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	sendURL := msg.Channel().StringConfigForKey(courier.ConfigSendURL, "")
	if sendURL == "" {
		return nil, fmt.Errorf("No send_url set for JN channel")
	}

	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if username == "" || password == "" {
		return nil, fmt.Errorf("Missing username or password for JN channel")
	}

	secret := msg.Channel().StringConfigForKey(courier.ConfigSecret, "")

	callbackDomain := msg.Channel().CallbackDomain(h.Server().Config().Domain)
	eventURL := fmt.Sprintf("https://%s/c/jn/%s/event", callbackDomain, msg.Channel().UUID())

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	for i, part := range handlers.SplitMsg(handlers.GetTextAndAttachments(msg), maxMsgLength) {
		payload := mtPayload{
			EventURL: eventURL,
			Content:  part,
			From:     msg.Channel().Address(),
			To:       msg.URN().Path(),
		}

		if secret != "" {
			payload.EventAuthToken = secret
		}

		jsonBody, err := json.Marshal(payload)
		if err != nil {
			return status, err
		}

		req, _ := http.NewRequest(http.MethodPost, sendURL, bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.SetBasicAuth(username, password)
		rr, err := utils.MakeHTTPRequest(req)

		// record our status and log
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
		status.AddLog(log)
		if err != nil {
			return status, nil
		}

		externalID, err := jsonparser.GetString(rr.Body, "result", "message_id")
		if err != nil {
			log.WithError("Message Send Error", errors.Errorf("unable to get result.message_id from body"))
			return status, nil
		}

		// if this is our first message, record the external id
		if i == 0 {
			status.SetExternalID(externalID)
		}
	}

	// this was wired successfully
	status.SetStatus(courier.MsgWired)
	return status, nil
}
