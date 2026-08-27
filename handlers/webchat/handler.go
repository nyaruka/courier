package webchat

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/gocommon/centrifugo"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/random"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/core/events"
)

const (
	chatIDLength = 24

	// optional channel config: the host[:port] values (no scheme) of the sites the widget may be embedded on -
	// empty or absent means unrestricted
	configAllowedDomains = "allowed_domains"

	// the type of the event published to a conversation's chat socket for each outgoing message
	eventTypeMsgOut = "msg_out"

	// how many chats a single IP can start on a channel per window - generous for a real visitor (who starts
	// one chat, ever) while capping how fast anyone can mint contacts
	startLimit       = 10
	startLimitWindow = 60 // seconds
)

var chatIDChars = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

func init() {
	channels.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() channels.Handler {
	// webchat has no external provider - sends are publishes to our own realtime server and start/receive are
	// our own public endpoints - so its channel logs would only describe internal infrastructure and aren't
	// stored for users
	return &handler{handlers.NewBaseHandler(models.ChannelType("WCH"), "WebChat", handlers.DisableChannelLogStorage())}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(r *channels.Routes) error {
	r.Add(h, http.MethodPost, "start", models.ChannelLogTypeChatStart, withCORS(h.start))
	r.Add(h, http.MethodPost, "receive", models.ChannelLogTypeMsgReceive, withCORS(handlers.JSONPayload(h, h.receive)))

	// the chat widget runs on arbitrary third-party websites, so both endpoints need CORS preflight support
	r.Add(h, http.MethodOptions, "start", models.ChannelLogTypeUnknown, h.preflight)
	r.Add(h, http.MethodOptions, "receive", models.ChannelLogTypeUnknown, h.preflight)
	return nil
}

// GetChannel returns the channel - except for CORS preflight requests, which don't need it and shouldn't
// generate channel logs
func (h *handler) GetChannel(ctx context.Context, r *http.Request) (*models.Channel, error) {
	if r.Method == http.MethodOptions {
		return nil, nil
	}
	return h.BaseHandler.GetChannel(ctx, r)
}

// withCORS wraps a handler function to enforce the channel's allowed domains and set the CORS header that all
// responses on our public endpoints need - error responses included, or the widget's browser couldn't read
// them - because the widget calls from third-party origins.
//
// Allowed domains are an anti-embedding control for real browsers, which send a genuine Origin header - not an
// anti-abuse control, since scripted clients can spoof or omit Origin freely (the start rate limit and edge
// protection cover those) - so a request without an Origin header always passes.
func withCORS(fn channels.HandleFunc) channels.HandleFunc {
	return func(ctx context.Context, channel *models.Channel, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
		domains := allowedDomains(channel)
		origin := r.Header.Get("Origin")

		if len(domains) > 0 {
			// every branch below picks the allow-origin value from the request's origin so caches must vary on it
			w.Header().Add("Vary", "Origin")
		}

		if len(domains) > 0 && origin != "" {
			if !originAllowed(origin, domains) {
				// deliberately no allow-origin header on this response, so the embedding page's browser also
				// blocks it from reading the error
				channels.LogRequestError(r, channel, fmt.Errorf("origin not allowed: %s", origin))
				return nil, channels.WriteError(w, http.StatusForbidden, fmt.Errorf("origin not allowed"))
			}

			// reflect the specific origin instead of * so only pages on allowed domains get readable responses
			w.Header().Set("Access-Control-Allow-Origin", origin)
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}

		return fn(ctx, channel, w, r, clog)
	}
}

// allowedDomains reads the channel's allowed_domains config as a list of host[:port] strings
func allowedDomains(channel *models.Channel) []string {
	vals, _ := channel.ConfigForKey(configAllowedDomains, nil).([]any)
	domains := make([]string, 0, len(vals))
	for _, val := range vals {
		if s, ok := val.(string); ok {
			domains = append(domains, s)
		}
	}
	return domains
}

// originAllowed parses an Origin header value (scheme://host[:port]) and checks whether its host[:port]
// matches one of the configured domains, case-insensitively
func originAllowed(origin string, domains []string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return slices.ContainsFunc(domains, func(d string) bool { return strings.EqualFold(d, u.Host) })
}

// preflight is our HTTP handler for CORS preflight requests to the start and receive endpoints. The channel
// deliberately isn't loaded here (see GetChannel) so preflights can't check allowed domains and stay
// permissive - that's fine because the browser is gated by the allow-origin header on the actual POST
// response, which does check them.
func (h *handler) preflight(ctx context.Context, channel *models.Channel, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Max-Age", "86400")
	w.WriteHeader(http.StatusNoContent)
	return nil, nil
}

type startResponse struct {
	ChatID string `json:"chat_id"`
}

// start is our HTTP handler for a visitor opening a chat for the first time: it mints them a new chat ID and
// creates the contact behind it.
//
// It's necessarily unauthenticated - it's what hands a brand new visitor their credential, and the channel UUID
// it's addressed by is public in the widget's JS - so each request creating a contact is an abuse surface. The
// per-IP throttle below caps how fast a single caller can mint contacts - but only if the IP is trustworthy:
// the server's RealIP middleware takes it from forwarded headers without a trusted-proxy allowlist, so this
// relies on the edge overwriting client-supplied headers. Spoofed-header and distributed floods are left to
// edge-level protection like any other unauthenticated endpoint.
func (h *handler) start(ctx context.Context, channel *models.Channel, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
	if !h.allowStart(channel, r) {
		channels.LogRequestError(r, channel, fmt.Errorf("rate limit exceeded"))
		return nil, channels.WriteError(w, http.StatusTooManyRequests, fmt.Errorf("rate limit exceeded"))
	}

	// a chat ID is a bearer credential - possession is the only thing that identifies a webchat visitor - so it
	// comes from a CSPRNG (24 chars of [a-zA-Z0-9] is ~143 bits of entropy)
	chatID := random.SecureString(chatIDLength, chatIDChars)
	urn, err := urns.NewFromParts(urns.WebChat.Prefix, chatID, nil, "")
	if err != nil {
		return nil, fmt.Errorf("error creating webchat URN: %w", err)
	}

	if _, err := models.GetContact(ctx, h.Runtime(), channel, urn, nil, "", true, clog); err != nil {
		return nil, fmt.Errorf("error creating contact: %w", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonx.MustMarshal(&startResponse{ChatID: chatID}))
	return nil, nil
}

// allowStart checks the requesting IP against the channel's start rate limit, counting requests in a valkey key
// whose TTL slides with each request and expires one window after the last
func (h *handler) allowStart(channel *models.Channel, r *http.Request) bool {
	// the server's RealIP middleware has already resolved forwarded headers into RemoteAddr, which may or may
	// not still carry a port
	ip := r.RemoteAddr
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}

	key := fmt.Sprintf("chat-starts:%s|%s", channel.UUID(), ip)

	rc := h.Runtime().VK.Get()
	defer rc.Close()

	count, err := redis.Int(rc.Do("INCR", key))
	if err != nil {
		// a valkey problem shouldn't stop visitors starting chats so proceed unthrottled
		slog.Error("error checking chat start rate limit", "error", err, "key", key)
		return true
	}
	// re-arm the TTL on every request rather than only the first: INCR + EXPIRE isn't atomic, and a key left
	// behind by a lost EXPIRE would otherwise count forever and permanently block the IP. The result is a
	// sliding window - continuous callers stay throttled, which is fine for a start-flood cap.
	if _, err := rc.Do("EXPIRE", key, startLimitWindow); err != nil {
		slog.Error("error setting chat start rate limit expiry", "error", err, "key", key)
	}

	return count <= startLimit
}

type receivePayload struct {
	ChatID string `json:"chat_id" validate:"required"`
	Text   string `json:"text"    validate:"required"`
}

// receive is our HTTP handler function for incoming messages
func (h *handler) receive(ctx context.Context, channel *models.Channel, w http.ResponseWriter, r *http.Request, payload *receivePayload, clog *models.ChannelLog) ([]channels.Event, error) {
	urn, err := urns.NewFromParts(urns.WebChat.Prefix, payload.ChatID, nil, "")
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("invalid chat id: %s", payload.ChatID))
	}

	// chat IDs are only ever minted by the start endpoint, so a URN we've never seen is a bad request rather
	// than a new contact
	contact, err := models.GetContact(ctx, h.Runtime(), channel, urn, nil, "", false, clog)
	if err != nil {
		return nil, fmt.Errorf("error looking up contact: %w", err)
	}
	if contact == nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unknown chat id: %s", payload.ChatID))
	}

	msg := models.NewIncomingMsg(channel, urn, payload.Text, "", clog)
	return handlers.WriteMsgsAndResponse(ctx, h, []*models.MsgIn{msg}, w, r, clog)
}

// msgOutEvent is the event published to the conversation's chat socket for an outgoing message - the same
// uuid/type/created_on shape as the engine events published to history sockets
type msgOutEvent struct {
	events.BaseEvent

	MsgUUID models.MsgUUID `json:"msg_uuid"`
	Text    string         `json:"text"`
}

func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	socket := models.ChatSocket(msg.Channel().UUID(), msg.URN().Path())
	event := &msgOutEvent{BaseEvent: events.NewBaseEvent(eventTypeMsgOut), MsgUUID: msg.UUID(), Text: msg.Text()}

	// like all socket publishes this is presence-aware and best-effort: if the visitor doesn't currently have
	// the chat open the publish is dropped, and the message is still considered sent
	if err := h.Runtime().Centrifugo.Publish(ctx, &centrifugo.Publication{Channel: socket, Data: event}); err != nil {
		return fmt.Errorf("error publishing message event: %w", err)
	}

	return nil
}
