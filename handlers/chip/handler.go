package chip

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/utils"
	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/svclogs"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/core/events"
)

const (
	configEmailFrom     = "email_from"
	configMailgunDomain = "mailgun_domain"
	configMailgunRegion = "mailgun_region"
	configMailgunAPIKey = "mailgun_api_key"
)

var mailgunBaseURLs = map[string]string{
	"us": "https://api.mailgun.net",
	"eu": "https://api.eu.mailgun.net",
}

// a visitor who messaged within this long is assumed to still have the chat open, so isn't emailed as well
const liveChatWindow = 5 * time.Minute

// WebChat is the platform's own chat widget, embedded on websites. Its provider is chip, the webchat service, which
// the widget talks to: it mints chat IDs, takes visitors' messages and uploads, serves their history and delivers what
// we send over the realtime server. Like any provider it posts what it receives to this handler's endpoints and we
// post what the platform sends to it - the difference being that those endpoints only answer to it.

func init() {
	channels.RegisterHandler(newHandler)
}

type handler struct {
	handlers.BaseHandler
}

func newHandler(rt *runtime.Runtime, r *channels.Routes) channels.Handler {
	// the transport is our own webchat service, so channel logs would only describe internal infrastructure and
	// aren't stored for users
	h := &handler{handlers.NewBaseHandler(rt, models.ChannelType("CHP"), "WebChat", handlers.DisableChannelLogStorage())}

	r.Add(h, http.MethodPost, "start", models.ChannelLogTypeChatStart, h.withAuth(h.start))

	// these can't use AddReceive because the auth check has to wrap the seam rather than sit inside it, so the
	// kind is named twice - once for what the route serves, once for what its log is called
	receive := channels.Receive(h, channels.ReceiveKindMsg, handlers.JSONPayload(h.receiveMessage))
	r.Add(h, http.MethodPost, "receive", channels.ReceiveKindMsg.LogType(), h.withAuth(receive))

	status := channels.Receive(h, channels.ReceiveKindStatus, handlers.JSONPayload(h.receiveStatus))
	r.Add(h, http.MethodPost, "status", channels.ReceiveKindStatus.LogType(), h.withAuth(status))

	return h
}

// withAuth wraps a handler function to refuse requests that aren't from the webchat service. The widget's requests
// come to us through it, which is where they're rate limited and checked against the channel's allowed domains, so
// nothing may reach these endpoints directly - and with no token configured nothing can, rather than everything.
func (h *handler) withAuth(fn channels.HandleFunc) channels.HandleFunc {
	return func(ctx context.Context, channel *models.Channel, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
		token := h.Runtime().Config.ChipAuthToken
		auth := r.Header.Get("Authorization")
		if token == "" || !strings.HasPrefix(auth, "Token ") || !utils.SecretEqual(auth[6:], token) {
			return nil, channels.RespondError(w, http.StatusUnauthorized, errors.New("invalid or missing authorization header"))
		}
		return fn(ctx, channel, w, r, clog)
	}
}

type startPayload struct {
	ChatID string `json:"chat_id" validate:"required"`
	// the host of the site the visitor started the chat from - carried for routing by where a visitor came from,
	// since one channel may serve several sites
	Origin string `json:"origin"`
}

// start is our handler for the webchat service starting a chat for a new visitor: the service has minted them a chat
// ID - their bearer credential - and we create the contact behind it
func (h *handler) start(ctx context.Context, channel *models.Channel, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
	payload := &startPayload{}
	if err := handlers.DecodeAndValidateJSON(payload, r); err != nil {
		return nil, channels.RespondError(w, http.StatusBadRequest, err)
	}

	urn, err := urns.NewFromParts(urns.WebChat.Prefix, payload.ChatID, nil, "")
	if err != nil {
		return nil, channels.RespondError(w, http.StatusBadRequest, fmt.Errorf("invalid chat id: %s", payload.ChatID))
	}

	if _, err := models.GetContact(ctx, h.Runtime(), channel, urn, nil, "", true, clog); err != nil {
		var limitErr *models.LimitReachedError
		if errors.As(err, &limitErr) {
			channels.LogRequestError(r, channel, err)
			return nil, channels.RespondError(w, http.StatusUnprocessableEntity, err)
		}
		return nil, fmt.Errorf("error creating contact: %w", err)
	}

	return nil, channels.RespondData(w, http.StatusOK, "Contact Created", []any{})
}

type receivePayload struct {
	ChatID string `json:"chat_id" validate:"required"`
	// the widget's own ID for the message, so that a send it retries isn't recorded twice
	ExternalID string `json:"external_id"`
	// text is optional when the message carries attachments
	Text        string   `json:"text" validate:"required_without=Attachments"`
	Attachments []string `json:"attachments"`
}

// receiveMessage is our receive function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel *models.Channel, r *http.Request, payload *receivePayload, in *channels.Received, clog *models.ChannelLog) error {
	urn, err := urns.NewFromParts(urns.WebChat.Prefix, payload.ChatID, nil, "")
	if err != nil {
		return fmt.Errorf("invalid chat id: %s", payload.ChatID)
	}

	// chat IDs are only ever minted when a chat starts, so a URN we've never seen is a bad request rather than a
	// new contact
	contact, err := models.GetContact(ctx, h.Runtime(), channel, urn, nil, "", false, clog)
	if err != nil {
		return fmt.Errorf("error looking up contact: %w", err)
	}
	if contact == nil {
		return fmt.Errorf("unknown chat id: %s", payload.ChatID)
	}

	msg := models.NewIncomingMsg(channel, urn, payload.Text, payload.ExternalID, clog)
	for _, att := range payload.Attachments {
		msg.WithAttachment(att)
	}
	in.Msg(msg)
	return nil
}

// the statuses a visitor's browser can report for the messages it's been sent - never failure, which is ours to decide
var reportableStatuses = map[string]models.MsgStatus{
	"delivered": models.MsgStatusDelivered,
	"read":      models.MsgStatusRead,
}

type statusPayload struct {
	ChatID   string           `json:"chat_id"   validate:"required"`
	Status   string           `json:"status"    validate:"required,oneof=delivered read"`
	MsgUUIDs []models.MsgUUID `json:"msg_uuids" validate:"required,min=1,dive,uuid"`
}

// receiveStatus is our receive function for a visitor's browser reporting that it received or displayed messages
// it was sent
func (h *handler) receiveStatus(ctx context.Context, channel *models.Channel, r *http.Request, payload *statusPayload, in *channels.Received, clog *models.ChannelLog) error {
	urn, err := urns.NewFromParts(urns.WebChat.Prefix, payload.ChatID, nil, "")
	if err != nil {
		return fmt.Errorf("invalid chat id: %s", payload.ChatID)
	}

	contact, err := models.GetContact(ctx, h.Runtime(), channel, urn, nil, "", false, clog)
	if err != nil {
		return fmt.Errorf("error looking up contact: %w", err)
	}
	if contact == nil {
		return fmt.Errorf("unknown chat id: %s", payload.ChatID)
	}

	// UUIDs aren't checked against this chat's conversation: status writes only touch outgoing messages on the
	// channel, and message UUIDs are unguessable, so like any status callback the exposure is a leaked UUID
	status := reportableStatuses[payload.Status]
	for _, msgUUID := range payload.MsgUUIDs {
		in.Status(models.NewStatusUpdate(channel, msgUUID, status, clog))
	}
	return nil
}

// msgUser is a message's sender as the webchat service shows them to the visitor
type msgUser struct {
	UUID   models.UserUUID `json:"uuid"`
	Name   string          `json:"name"`
	Avatar string          `json:"avatar,omitempty"`
}

// sendPayload is an outgoing message as we hand it to the webchat service to deliver
type sendPayload struct {
	ChannelUUID  models.ChannelUUID  `json:"channel_uuid"`
	ChatID       string              `json:"chat_id"`
	MsgUUID      models.MsgUUID      `json:"msg_uuid"`
	Text         string              `json:"text"`
	Attachments  []string            `json:"attachments,omitempty"`
	QuickReplies []models.QuickReply `json:"quick_replies,omitempty"`
	User         *msgUser            `json:"user,omitempty"`
}

// sender looks up the user who sent a message, if it was sent by one. Attribution is best-effort: a lookup that
// fails is logged and the message still goes out, just unattributed.
func (h *handler) sender(ctx context.Context, msg *models.MsgOut) *msgUser {
	if msg.UserID() == models.NilUserID {
		return nil
	}
	user, err := models.GetUser(ctx, msg.UserID())
	if err != nil {
		slog.Error("error looking up message sender", "error", err, "msg_uuid", msg.UUID())
		return nil
	}
	if user == nil {
		return nil
	}
	return &msgUser{UUID: user.UUID(), Name: user.Name(), Avatar: user.AvatarURL(h.Runtime())}
}

func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	payload := &sendPayload{
		ChannelUUID: msg.Channel().UUID(),
		ChatID:      msg.URN().Path(),
		MsgUUID:     msg.UUID(),
		Text:        msg.Text(),
		Attachments: msg.Attachments(),
		// the widget only renders text quick replies, so like any other channel we filter to what's supported
		QuickReplies: handlers.FilterQuickRepliesByType(msg.QuickReplies(), models.QuickReplyTypeText),
		User:         h.sender(ctx, msg),
	}

	if err := h.post(ctx, "send", payload, clog); err != nil {
		return err
	}

	if h.shouldEmail(msg) {
		h.sendEmail(ctx, msg, clog)
	}
	return nil
}

// shouldEmail returns whether a message should also be emailed to the visitor, which needs the channel to have email
// configured and the contact to have an address. Email is how a visitor who has left hears from us, so a visitor
// in the middle of a conversation isn't emailed too.
func (h *handler) shouldEmail(msg *models.MsgOut) bool {
	if msg.Channel().StringConfigForKey(configMailgunDomain, "") == "" || msg.Contact().Email == "" {
		return false
	}
	lastSeen := msg.Contact().LastSeenOn
	return lastSeen == nil || dates.Since(*lastSeen) > liveChatWindow
}

// sendEmail sends a message to the visitor's email address via Mailgun. Email is a best-effort extra on top of the
// chat itself, which has already delivered the message, so a failure is logged rather than failing the send - which
// would have the message retried and redelivered to the chat.
func (h *handler) sendEmail(ctx context.Context, msg *models.MsgOut, clog *models.ChannelLog) {
	ch := msg.Channel()
	log := slog.With("channel_uuid", ch.UUID(), "msg_uuid", msg.UUID())
	domain := ch.StringConfigForKey(configMailgunDomain, "")
	from := ch.StringConfigForKey(configEmailFrom, "")
	region := ch.StringConfigForKey(configMailgunRegion, "")
	if region == "" {
		region = "us"
	}

	// a channel with a domain but a bad region or no sender address would fail every email at Mailgun, so say why
	baseURL, ok := mailgunBaseURLs[region]
	if !ok || from == "" {
		clog.Error(&svclogs.Error{Code: "email_config", Message: "email is enabled but its region or sender address is invalid"})
		log.Error("invalid webchat email config", "mailgun_region", region, "email_from", from)
		return
	}

	form := url.Values{
		"from":    {from},
		"to":      {msg.Contact().Email},
		"subject": {fmt.Sprintf("New message from %s", ch.Name())},
		"text":    {emailText(msg)},
	}

	sendURL := fmt.Sprintf("%s/v3/%s/messages", baseURL, url.PathEscape(domain))
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, sendURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("api", ch.StringConfigForKey(configMailgunAPIKey, ""))

	resp, _, err := h.RequestHTTP(req, clog)
	if err != nil {
		clog.Error(channels.ErrConnectionFailed.(*channels.SendError).ClogError())
		log.Error("error sending webchat message by email", "error", err)
	} else if resp.StatusCode/100 != 2 {
		clog.Error(models.ErrorResponseStatusCode())
		log.Error("error sending webchat message by email", "status", resp.StatusCode)
	}
}

// emailText is a message as plain text, with what the chat would show as media or buttons listed after it
func emailText(msg *models.MsgOut) string {
	lines := []string{msg.Text()}
	for _, a := range msg.Attachments() {
		_, attURL := handlers.SplitAttachment(a)
		lines = append(lines, attURL)
	}
	for _, qr := range msg.QuickReplies() {
		if qr.Type == models.QuickReplyTypeText {
			lines = append(lines, "- "+qr.Text)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// RedactValues redacts the Mailgun API key, including as part of the basic auth it's sent as
func (h *handler) RedactValues(ch *models.Channel) []string {
	key := ch.StringConfigForKey(configMailgunAPIKey, "")
	if key == "" {
		return nil
	}
	return []string{key, httpx.BasicAuth("api", key)}
}

// eventPayload is something other than a message we have the webchat service show the visitor - a user typing
type eventPayload struct {
	ChannelUUID models.ChannelUUID `json:"channel_uuid"`
	ChatID      string             `json:"chat_id"`
	Type        string             `json:"type"`
	User        *msgUser           `json:"user,omitempty"`
}

// the service publishes typing_started often enough to stay ahead of a client decaying the indicator, and typing_stopped
// is cheap enough to carry too
var sendableEvents = map[string]time.Duration{events.TypeTypingStarted: 5 * time.Second, events.TypeTypingStopped: 0}

// SendableEvents declares support for typing indicators
func (h *handler) SendableEvents(*models.Channel) map[string]time.Duration {
	return sendableEvents
}

// SendEvent hands a user's typing to the webchat service to show the visitor
func (h *handler) SendEvent(ctx context.Context, ch *models.Channel, event events.Event, clog *models.ChannelLog) error {
	var urn urns.URN
	var user *assets.UserReference
	switch typed := event.(type) {
	case *events.TypingStarted:
		urn, user = typed.URN, typed.User_
	case *events.TypingStopped:
		urn, user = typed.URN, typed.User_
	default:
		return fmt.Errorf("unsupported event type: %s", event.Type())
	}

	payload := &eventPayload{ChannelUUID: ch.UUID(), ChatID: urn.Path(), Type: event.Type()}
	if user != nil {
		payload.User = &msgUser{UUID: models.UserUUID(user.UUID), Name: user.Name}
	}

	return h.post(ctx, "event", payload, clog)
}

// post posts the given payload to one of the webchat service's internal endpoints. Anything but success is an
// error, so that a message is retried like any other failed send - with the service being unreachable told apart
// from it refusing, since the service is our own infrastructure.
func (h *handler) post(ctx context.Context, action string, payload any, clog *models.ChannelLog) error {
	cfg := h.Runtime().Config
	if cfg.ChipURL == "" || cfg.ChipAuthToken == "" {
		return channels.ErrChannelConfig
	}

	url := fmt.Sprintf("%s/wi/%s", strings.TrimSuffix(cfg.ChipURL, "/"), action)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonx.MustMarshal(payload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Token "+cfg.ChipAuthToken)

	resp, _, err := h.RequestHTTP(req, clog)
	if err != nil {
		return channels.ErrConnectionFailed
	}
	if resp.StatusCode/100 != 2 {
		return channels.ErrResponseStatus
	}
	return nil
}
