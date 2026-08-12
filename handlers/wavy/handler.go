package wavy

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
)

var (
	sendURL = "https://api-messaging.movile.com/v1/send-sms"
)

type handler struct {
	handlers.BaseHandler
}

func newHandler() channels.Handler {
	return &handler{handlers.NewBaseHandler(models.ChannelType("WV"), "Wavy")}
}

func init() {
	channels.RegisterHandler(newHandler())
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(r *channels.Routes) error {
	r.Add(h, http.MethodPost, "receive", models.ChannelLogTypeMsgReceive, handlers.JSONPayload(h, h.receiveMessage))
	r.Add(h, http.MethodPost, "sent", models.ChannelLogTypeMsgStatus, handlers.JSONPayload(h, h.sentStatusMessage))
	r.Add(h, http.MethodPost, "delivered", models.ChannelLogTypeMsgStatus, handlers.JSONPayload(h, h.deliveredStatusMessage))
	return nil
}

var statusMapping = map[int]models.MsgStatus{
	2:   models.MsgStatusSent,
	4:   models.MsgStatusDelivered,
	101: models.MsgStatusFailed,
	102: models.MsgStatusFailed,
	103: models.MsgStatusFailed,
	104: models.MsgStatusSent,
	201: models.MsgStatusFailed,
	202: models.MsgStatusFailed,
	203: models.MsgStatusFailed,
	204: models.MsgStatusFailed,
	205: models.MsgStatusFailed,
	207: models.MsgStatusFailed,
	301: models.MsgStatusErrored,
}

type sentStatusPayload struct {
	CollerationID  string `json:"correlationId"    validate:"required"`
	SentStatusCode int    `json:"sentStatusCode"   validate:"required"`
}

// sentStatusMessage is our HTTP handler function for status updates
func (h *handler) sentStatusMessage(ctx context.Context, channel *models.Channel, w http.ResponseWriter, r *http.Request, payload *sentStatusPayload, clog *models.ChannelLog) ([]channels.Event, error) {
	msgStatus, found := statusMapping[payload.SentStatusCode]
	if !found {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unknown sent status code '%d', must be one of 2, 101, 102, 103, 201, 202, 203, 204, 205, 207 or 301 ", payload.SentStatusCode))
	}

	// write our status
	status := models.NewStatusUpdateByExternalID(channel, payload.CollerationID, msgStatus, clog)
	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

type deliveredStatusPayload struct {
	CollerationID       string `json:"correlationId"          validate:"required"`
	DeliveredStatusCode int    `json:"deliveredStatusCode"    validate:"required"`
}

// sentStatusMessage is our HTTP handler function for status updates
func (h *handler) deliveredStatusMessage(ctx context.Context, channel *models.Channel, w http.ResponseWriter, r *http.Request, payload *deliveredStatusPayload, clog *models.ChannelLog) ([]channels.Event, error) {
	msgStatus, found := statusMapping[payload.DeliveredStatusCode]
	if !found {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unknown delivered status code '%d', must be 4 or 104", payload.DeliveredStatusCode))
	}

	// write our status
	status := models.NewStatusUpdateByExternalID(channel, payload.CollerationID, msgStatus, clog)
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
func (h *handler) receiveMessage(ctx context.Context, channel *models.Channel, w http.ResponseWriter, r *http.Request, payload *moPayload, clog *models.ChannelLog) ([]channels.Event, error) {
	date := time.Unix(0, int64(payload.Timestamp*1000000)).UTC()

	// create our URN
	urn, err := urns.ParsePhone(payload.From, channel.Country(), true, false)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}
	// build our msg
	msg := models.NewIncomingMsg(channel, urn, payload.Message, payload.ID, clog).WithReceivedOn(date.UTC())

	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []*models.MsgIn{msg}, w, r, clog)

}

type mtPayload struct {
	Destination string `json:"destination"`
	Message     string `json:"messageText"`
}

func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	username := msg.Channel().StringConfigForKey(models.ConfigUsername, "")
	token := msg.Channel().StringConfigForKey(models.ConfigAuthToken, "")
	if username == "" || token == "" {
		return channels.ErrChannelConfig
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
	if err := handlers.ErrorFromResponse(resp, err); err != nil {
		return err
	}

	externalID, _ := jsonparser.GetString(respBody, "id")
	if externalID != "" {
		res.AddExternalID(externalID)
	}

	return nil
}
