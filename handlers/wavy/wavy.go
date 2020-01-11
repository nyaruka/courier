package wavy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
)

var (
	sendURL      = "https://api-messaging.movile.com/v1/send-sms"
	maxMsgLength = 160
)

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("WV"), "Wavy")}
}

func init() {
	courier.RegisterHandler(newHandler())
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodPost, "sent", h.sentStatusMessage)
	s.AddHandlerRoute(h, http.MethodPost, "delivered", h.deliveredStatusMessage)
	return nil
}

var statusMapping = map[int]courier.MsgStatusValue{
	2:   courier.MsgSent,
	4:   courier.MsgDelivered,
	101: courier.MsgFailed,
	102: courier.MsgFailed,
	103: courier.MsgFailed,
	104: courier.MsgSent,
	201: courier.MsgFailed,
	202: courier.MsgFailed,
	203: courier.MsgFailed,
	204: courier.MsgFailed,
	205: courier.MsgFailed,
	207: courier.MsgFailed,
	301: courier.MsgErrored,
}

type sentStatusPayload struct {
	CollerationID  string `json:"correlationId"    validate:"required"`
	SentStatusCode int    `json:"sentStatusCode"   validate:"required"`
}

// sentStatusMessage is our HTTP handler function for status updates
func (h *handler) sentStatusMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	payload := &sentStatusPayload{}
	err := handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	msgStatus, found := statusMapping[payload.SentStatusCode]
	if !found {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unknown sent status code '%d', must be one of 2, 101, 102, 103, 201, 202, 203, 204, 205, 207 or 301 ", payload.SentStatusCode))
	}

	// write our status
	status := h.Backend().NewMsgStatusForExternalID(channel, payload.CollerationID, msgStatus)
	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

type deliveredStatusPayload struct {
	CollerationID       string `json:"correlationId"          validate:"required"`
	DeliveredStatusCode int    `json:"deliveredStatusCode"    validate:"required"`
}

// sentStatusMessage is our HTTP handler function for status updates
func (h *handler) deliveredStatusMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	payload := &deliveredStatusPayload{}
	err := handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	msgStatus, found := statusMapping[payload.DeliveredStatusCode]
	if !found {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unknown delivered status code '%d', must be 4 or 104", payload.DeliveredStatusCode))
	}

	// write our status
	status := h.Backend().NewMsgStatusForExternalID(channel, payload.CollerationID, msgStatus)
	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

type moPayload struct {
	ID        string `json:"id"            validate:"required"`
	From      string `json:"source"        validate:"required"`
	To        string `json:"shortCode"     validate:"required"`
	Message   string `json:"messageText"   validate:"required"`
	Timestamp int64  `json:"receivedAt"    validate:"required"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	payload := &moPayload{}
	err := handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	date := time.Unix(0, int64(payload.Timestamp*1000000)).UTC()

	// create our URN
	urn, err := handlers.StrictTelForCountry(payload.From, channel.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}
	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, payload.Message).WithExternalID(payload.ID).WithReceivedOn(date.UTC())

	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r)

}

type mtPayload struct {
	Destination string `json:"destination"`
	Message     string `json:"messageText"`
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for %s channel", msg.Channel().ChannelType())
	}

	token := msg.Channel().StringConfigForKey(courier.ConfigAuthToken, "")
	if token == "" {
		return nil, fmt.Errorf("no token set for %s channel", msg.Channel().ChannelType())
	}

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)

	payload := mtPayload{}
	payload.Destination = strings.TrimPrefix(msg.URN().Path(), "+")
	payload.Message = handlers.GetTextAndAttachments(msg)

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, _ := http.NewRequest(http.MethodPost, sendURL, bytes.NewReader(jsonPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("username", username)
	req.Header.Set("authenticationtoken", token)
	rr, err := utils.MakeHTTPRequest(req)

	// record our status and log
	status.AddLog(courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err))
	if err != nil {
		return status, nil
	}

	externalID, _ := jsonparser.GetString(rr.Body, "id")
	if externalID != "" {
		status.SetExternalID(externalID)
	}

	status.SetStatus(courier.MsgWired)
	return status, nil
}
