package wavy

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
)

var (
	sendURL = "https://api-messaging.movile.com/v1/send-sms"
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
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, handlers.JSONPayload(h, h.receiveMessage))
	s.AddHandlerRoute(h, http.MethodPost, "sent", courier.ChannelLogTypeMsgStatus, handlers.JSONPayload(h, h.sentStatusMessage))
	s.AddHandlerRoute(h, http.MethodPost, "delivered", courier.ChannelLogTypeMsgStatus, handlers.JSONPayload(h, h.deliveredStatusMessage))
	return nil
}

var statusMapping = map[int]courier.MsgStatus{
	2:   courier.MsgStatusSent,
	4:   courier.MsgStatusDelivered,
	101: courier.MsgStatusFailed,
	102: courier.MsgStatusFailed,
	103: courier.MsgStatusFailed,
	104: courier.MsgStatusSent,
	201: courier.MsgStatusFailed,
	202: courier.MsgStatusFailed,
	203: courier.MsgStatusFailed,
	204: courier.MsgStatusFailed,
	205: courier.MsgStatusFailed,
	207: courier.MsgStatusFailed,
	301: courier.MsgStatusErrored,
}

type sentStatusPayload struct {
	CollerationID  string `json:"correlationId"    validate:"required"`
	SentStatusCode int    `json:"sentStatusCode"   validate:"required"`
}

// sentStatusMessage is our HTTP handler function for status updates
func (h *handler) sentStatusMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *sentStatusPayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	msgStatus, found := statusMapping[payload.SentStatusCode]
	if !found {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unknown sent status code '%d', must be one of 2, 101, 102, 103, 201, 202, 203, 204, 205, 207 or 301 ", payload.SentStatusCode))
	}

	// write our status
	status := h.Backend().NewStatusUpdateByExternalID(channel, payload.CollerationID, msgStatus, clog)
	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

type deliveredStatusPayload struct {
	CollerationID       string `json:"correlationId"          validate:"required"`
	DeliveredStatusCode int    `json:"deliveredStatusCode"    validate:"required"`
}

// sentStatusMessage is our HTTP handler function for status updates
func (h *handler) deliveredStatusMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *deliveredStatusPayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	msgStatus, found := statusMapping[payload.DeliveredStatusCode]
	if !found {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unknown delivered status code '%d', must be 4 or 104", payload.DeliveredStatusCode))
	}

	// write our status
	status := h.Backend().NewStatusUpdateByExternalID(channel, payload.CollerationID, msgStatus, clog)
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
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *moPayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	date := time.Unix(0, int64(payload.Timestamp*1000000)).UTC()

	// create our URN
	urn, err := urns.ParsePhone(payload.From, channel.Country(), true, false)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}
	// build our msg
	msg := h.Backend().NewIncomingMsg(ctx, channel, urn, payload.Message, payload.ID, clog).WithReceivedOn(date.UTC())

	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)

}

type mtPayload struct {
	Destination string `json:"destination"`
	Message     string `json:"messageText"`
}

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	token := msg.Channel().StringConfigForKey(courier.ConfigAuthToken, "")
	if username == "" || token == "" {
		return courier.ErrChannelConfig
	}

	payload := mtPayload{}
	payload.Destination = strings.TrimPrefix(msg.URN().Path(), "+")
	payload.Message = handlers.GetTextAndAttachments(msg)

	jsonPayload := jsonx.MustMarshal(payload)

	req, err := http.NewRequest(http.MethodPost, sendURL, bytes.NewReader(jsonPayload))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("username", username)
	req.Header.Set("authenticationtoken", token)

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 == 5 {
		return courier.ErrConnectionFailed
	} else if resp.StatusCode/100 != 2 {
		return courier.ErrResponseStatus
	}

	externalID, _ := jsonparser.GetString(respBody, "id")
	if externalID != "" {
		res.AddExternalID(externalID)
	}

	return nil
}
