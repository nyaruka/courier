package webchat

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/h2non/filetype"
	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/gocommon/centrifugo"
	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/random"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
)

const (
	chatIDLength = 24

	// optional channel config: the host[:port] values (no scheme) of the sites the widget may be embedded on -
	// empty or absent means unrestricted
	configAllowedDomains = "allowed_domains"

	// the types of the message events a chat client sees - published to a conversation's chat socket for each
	// outgoing message, and returned by the history endpoint for messages in both directions
	eventTypeMsgOut = "msg_out"
	eventTypeMsgIn  = "msg_in"

	// how many chats a single IP can start on a channel per window - generous for a real visitor (who starts
	// one chat, ever) while capping how fast anyone can mint contacts
	startLimit       = 10
	startLimitWindow = 60 // seconds

	// how many messages a history request returns, and how many requests a single chat can make per window -
	// enough for a reconnecting client to catch up without letting anyone hammer the database
	historyPageSize    = 25
	historyLimit       = 10
	historyLimitWindow = 60 // seconds

	// the most a visitor can upload in one file - matching what the platform accepts for media uploads
	// elsewhere - and how many uploads a single chat can make per window
	maxUploadBytes    = 25 * 1024 * 1024
	uploadLimit       = 5
	uploadLimitWindow = 60 // seconds

	// how many uploads a single IP can make on a channel per window - a backstop above the per-chat limit
	// (so visitors sharing a NAT aren't broken) that a caller can't reset by rotating chat IDs. It also
	// bounds upload memory per source: the worst case is this many bodies buffered per IP per window.
	uploadIPLimit       = 10
	uploadIPLimitWindow = 60 // seconds

	// slack on the request body cap for the multipart framing and fields around the file itself
	uploadFormOverheadBytes = 64 * 1024
)

var chatIDChars = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

// the media types a visitor may upload - the same kinds of media the platform handles on other channels.
// Entries ending in / allow a whole family of types.
var allowedUploadTypes = []string{"image/", "audio/", "video/", "application/pdf"}

// uploadTypeAllowed returns whether the given media type is one visitors may upload
func uploadTypeAllowed(mime string) bool {
	return mime != "" && slices.ContainsFunc(allowedUploadTypes, func(a string) bool {
		return mime == a || (strings.HasSuffix(a, "/") && strings.HasPrefix(mime, a))
	})
}

func init() {
	channels.RegisterHandler(newHandler)
}

type handler struct {
	handlers.BaseHandler
}

func newHandler(rt *runtime.Runtime, r *channels.Routes) channels.Handler {
	// webchat has no external provider - sends are publishes to our own realtime server and start/receive are
	// our own public endpoints - so its channel logs would only describe internal infrastructure and aren't
	// stored for users
	h := &handler{handlers.NewBaseHandler(rt, models.ChannelType("WCH"), "WebChat", handlers.DisableChannelLogStorage())}

	r.Add(h, http.MethodPost, "start", models.ChannelLogTypeChatStart, withCORS(h.start))

	// this can't use AddReceive because the CORS headers have to wrap the seam rather than sit inside it, so
	// the kind is named twice - once for what the route serves, once for what its log is called
	receive := channels.Receive(h, channels.ReceiveKindMsg, handlers.JSONPayload(h.receiveMessage))
	r.Add(h, http.MethodPost, "receive", channels.ReceiveKindMsg.LogType(), withCORS(receive))

	r.Add(h, http.MethodGet, "history", models.ChannelLogTypeChatHistory, withCORS(h.history))

	r.Add(h, http.MethodPost, "upload", models.ChannelLogTypeChatUpload, withCORS(h.upload))

	// the chat widget runs on arbitrary third-party websites, so all the endpoints need CORS preflight support
	r.Add(h, http.MethodOptions, "start", models.ChannelLogTypeUnknown, h.preflight)
	r.Add(h, http.MethodOptions, "receive", models.ChannelLogTypeUnknown, h.preflight)
	r.Add(h, http.MethodOptions, "history", models.ChannelLogTypeUnknown, h.preflight)
	r.Add(h, http.MethodOptions, "upload", models.ChannelLogTypeUnknown, h.preflight)
	return h
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
				return nil, channels.RespondError(w, http.StatusForbidden, fmt.Errorf("origin not allowed"))
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
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	// allowing Content-Type covers any value it takes, so this accommodates both the JSON endpoints and the
	// upload endpoint's multipart posts
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
		return nil, channels.RespondError(w, http.StatusTooManyRequests, fmt.Errorf("rate limit exceeded"))
	}

	// a chat ID is a bearer credential - possession is the only thing that identifies a webchat visitor - so it
	// comes from a CSPRNG (24 chars of [a-zA-Z0-9] is ~143 bits of entropy)
	chatID := random.SecureString(chatIDLength, chatIDChars)
	urn, err := urns.NewFromParts(urns.WebChat.Prefix, chatID, nil, "")
	if err != nil {
		return nil, fmt.Errorf("error creating webchat URN: %w", err)
	}

	if _, err := models.GetContact(ctx, h.Runtime(), channel, urn, nil, "", true, clog); err != nil {
		var limitErr *models.LimitReachedError
		if errors.As(err, &limitErr) {
			channels.LogRequestError(r, channel, err)
			return nil, channels.RespondError(w, http.StatusUnprocessableEntity, err)
		}
		return nil, fmt.Errorf("error creating contact: %w", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonx.MustMarshal(&startResponse{ChatID: chatID}))
	return nil, nil
}

// allowStart checks the requesting IP against the channel's start rate limit
func (h *handler) allowStart(channel *models.Channel, r *http.Request) bool {
	return h.allow(fmt.Sprintf("chat-starts:%s|%s", channel.UUID(), requestIP(r)), startLimit, startLimitWindow)
}

// requestIP returns the requesting IP. The server's RealIP middleware has already resolved forwarded headers
// into RemoteAddr, which may or may not still carry a port.
func requestIP(r *http.Request) string {
	ip := r.RemoteAddr
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	return ip
}

// allow checks a rate limit by counting requests in a valkey key whose TTL slides with each request and expires
// one window after the last
func (h *handler) allow(key string, limit, window int) bool {
	rc := h.Runtime().VK.Get()
	defer rc.Close()

	count, err := redis.Int(rc.Do("INCR", key))
	if err != nil {
		// a valkey problem shouldn't stop visitors using their chats so proceed unthrottled
		slog.Error("error checking chat rate limit", "error", err, "key", key)
		return true
	}
	// re-arm the TTL on every request rather than only the first: INCR + EXPIRE isn't atomic, and a key left
	// behind by a lost EXPIRE would otherwise count forever and permanently block the caller. The result is a
	// sliding window - continuous callers stay throttled, which is fine for an abuse cap.
	if _, err := rc.Do("EXPIRE", key, window); err != nil {
		slog.Error("error setting chat rate limit expiry", "error", err, "key", key)
	}

	return count <= limit
}

type receivePayload struct {
	ChatID string `json:"chat_id" validate:"required"`
	// max counts runes, and we reject rather than truncate because we control the widget - an over-limit
	// message is a client bug or abuse. Text is optional when the message carries attachments.
	Text string `json:"text" validate:"required_without=Attachments,max=1000"`
	// attachments the visitor previously uploaded, as the content-type:url values the upload endpoint returned
	Attachments []string `json:"attachments" validate:"max=10"`
}

// receiveMessage is our receive function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel *models.Channel, r *http.Request, payload *receivePayload, in *channels.Received, clog *models.ChannelLog) error {
	urn, err := urns.NewFromParts(urns.WebChat.Prefix, payload.ChatID, nil, "")
	if err != nil {
		return fmt.Errorf("invalid chat id: %s", payload.ChatID)
	}

	// attachments may only reference files uploaded to the channel workspace's own attachment storage -
	// accepting arbitrary URLs would let anyone use the platform as a fetch proxy, and attach content we
	// never vetted - and may only carry the media types the upload endpoint allows, since the type label
	// here is client-supplied and is what's stored on the message
	prefix := h.uploadURLPrefix(channel)
	for _, att := range payload.Attachments {
		if ct, u := handlers.SplitAttachment(att); !uploadTypeAllowed(ct) || !strings.HasPrefix(u, prefix) {
			return fmt.Errorf("invalid attachment: %s", att)
		}
	}

	// chat IDs are only ever minted by the start endpoint, so a URN we've never seen is a bad request rather
	// than a new contact
	contact, err := models.GetContact(ctx, h.Runtime(), channel, urn, nil, "", false, clog)
	if err != nil {
		return fmt.Errorf("error looking up contact: %w", err)
	}
	if contact == nil {
		return fmt.Errorf("unknown chat id: %s", payload.ChatID)
	}

	msg := models.NewIncomingMsg(channel, urn, payload.Text, "", clog)
	for _, att := range payload.Attachments {
		msg.WithAttachment(att)
	}
	in.Msg(msg)
	return nil
}

type uploadResponse struct {
	Attachment string `json:"attachment"`
}

// upload is our HTTP handler for a visitor adding a file to their chat: the file is saved to attachment storage
// and the visitor gets back its content-type:url attachment value, to be referenced in a subsequent message to
// the receive endpoint. Like receive, possession of the chat ID is what authenticates the caller.
func (h *handler) upload(ctx context.Context, channel *models.Channel, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
	// the per-chat throttle below can't run until the form is parsed, so on its own it lets a caller buffer
	// bodies freely by rotating chat IDs. This per-IP cap doesn't need the body, so it runs first and a
	// throttled request is rejected before anything is buffered. Like start, distributed floods are left to
	// edge protection.
	if !h.allow(fmt.Sprintf("chat-uploads-ip:%s|%s", channel.UUID(), requestIP(r)), uploadIPLimit, uploadIPLimitWindow) {
		channels.LogRequestError(r, channel, fmt.Errorf("rate limit exceeded"))
		return nil, channels.RespondError(w, http.StatusTooManyRequests, fmt.Errorf("rate limit exceeded"))
	}

	// cap the body before parsing so an oversize upload is cut off rather than buffered in full
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+uploadFormOverheadBytes)

	// the whole cap fits in memory, so nothing spills to temp files
	if err := r.ParseMultipartForm(maxUploadBytes + uploadFormOverheadBytes); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			channels.LogRequestError(r, channel, fmt.Errorf("upload too large"))
			return nil, channels.RespondError(w, http.StatusRequestEntityTooLarge, fmt.Errorf("upload too large (max %d bytes)", maxUploadBytes))
		}
		channels.LogRequestError(r, channel, err)
		return nil, channels.RespondError(w, http.StatusBadRequest, fmt.Errorf("invalid multipart form"))
	}

	chatID := r.FormValue("chat_id")

	// validated before the throttle so malformed chat IDs can't mint valkey keys or share one empty-ID key
	urn, err := urns.NewFromParts(urns.WebChat.Prefix, chatID, nil, "")
	if err != nil {
		channels.LogRequestError(r, channel, fmt.Errorf("invalid chat id: %s", chatID))
		return nil, channels.RespondError(w, http.StatusBadRequest, fmt.Errorf("invalid chat id"))
	}

	// throttled per chat rather than per IP - a visitor attaching files only needs a handful at a time, and
	// like start, anything distributed is left to edge protection
	if !h.allow(fmt.Sprintf("chat-uploads:%s|%s", channel.UUID(), chatID), uploadLimit, uploadLimitWindow) {
		channels.LogRequestError(r, channel, fmt.Errorf("rate limit exceeded"))
		return nil, channels.RespondError(w, http.StatusTooManyRequests, fmt.Errorf("rate limit exceeded"))
	}

	// like receive, a chat ID we've never minted is a bad request rather than a new contact
	contact, err := models.GetContact(ctx, h.Runtime(), channel, urn, nil, "", false, clog)
	if err != nil {
		return nil, fmt.Errorf("error looking up contact: %w", err)
	}
	if contact == nil {
		channels.LogRequestError(r, channel, fmt.Errorf("unknown chat id: %s", chatID))
		return nil, channels.RespondError(w, http.StatusBadRequest, fmt.Errorf("unknown chat id"))
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		channels.LogRequestError(r, channel, fmt.Errorf("missing file part"))
		return nil, channels.RespondError(w, http.StatusBadRequest, fmt.Errorf("missing file part"))
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("error reading uploaded file: %w", err)
	}

	// the content type comes from sniffing the bytes rather than trusting the client's declared type - an
	// unrecognized file is rejected along with recognized-but-disallowed types
	fileType, _ := filetype.Match(data)
	if fileType == filetype.Unknown || !uploadTypeAllowed(fileType.MIME.Value) {
		channels.LogRequestError(r, channel, fmt.Errorf("unsupported file type"))
		return nil, channels.RespondError(w, http.StatusBadRequest, fmt.Errorf("unsupported file type"))
	}

	storageURL, err := models.SaveAttachment(ctx, h.Runtime(), channel, fileType.MIME.Value, data, fileType.Extension)
	if err != nil {
		return nil, fmt.Errorf("error saving attachment: %w", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonx.MustMarshal(&uploadResponse{Attachment: fmt.Sprintf("%s:%s", fileType.MIME.Value, storageURL)}))
	return nil, nil
}

// uploadURLPrefix is what the storage URLs of this channel's uploaded attachments all start with - which
// scopes a message's attachment references to files uploaded for this channel's workspace
func (h *handler) uploadURLPrefix(channel *models.Channel) string {
	rt := h.Runtime()
	return rt.S3.ObjectURL(rt.Config.S3AttachmentsBucket, fmt.Sprintf("attachments/%d/", channel.OrgID()))
}

// historyResponse is the response to a history request: a page of message events, newest first, and - when the
// page was full - the cursor that fetches the next page back
type historyResponse struct {
	Events []*msgEvent `json:"events"`
	Next   string      `json:"next,omitempty"`
}

// history is our HTTP handler for a chat client fetching the recent messages in its conversation. Socket
// publishes are dropped when the visitor doesn't have the chat open, so a client that reconnects calls this to
// recover what it missed. Like receive, possession of the chat ID is what authenticates the caller.
func (h *handler) history(ctx context.Context, channel *models.Channel, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
	chatID := r.URL.Query().Get("chat_id")

	// the paging cursor is a message UUID - v7, so UUID order is message order
	var before models.MsgUUID
	if v := r.URL.Query().Get("before"); v != "" {
		if !uuids.Is(v) {
			channels.LogRequestError(r, channel, fmt.Errorf("invalid before parameter: %s", v))
			return nil, channels.RespondError(w, http.StatusBadRequest, fmt.Errorf("invalid before parameter"))
		}
		before = models.MsgUUID(v)
	}

	// validated before the throttle so malformed chat IDs can't mint valkey keys or share one empty-ID key
	urn, err := urns.NewFromParts(urns.WebChat.Prefix, chatID, nil, "")
	if err != nil {
		channels.LogRequestError(r, channel, fmt.Errorf("invalid chat id: %s", chatID))
		return nil, channels.RespondError(w, http.StatusBadRequest, fmt.Errorf("invalid chat id"))
	}

	// throttled per chat rather than per IP - a real client only needs a small burst at reconnect, and like
	// start, anything distributed is left to edge protection
	if !h.allow(fmt.Sprintf("chat-history:%s|%s", channel.UUID(), chatID), historyLimit, historyLimitWindow) {
		channels.LogRequestError(r, channel, fmt.Errorf("rate limit exceeded"))
		return nil, channels.RespondError(w, http.StatusTooManyRequests, fmt.Errorf("rate limit exceeded"))
	}

	// like receive, a chat ID we've never minted is a bad request rather than an empty conversation
	contact, err := models.GetContact(ctx, h.Runtime(), channel, urn, nil, "", false, clog)
	if err != nil {
		return nil, fmt.Errorf("error looking up contact: %w", err)
	}
	if contact == nil {
		channels.LogRequestError(r, channel, fmt.Errorf("unknown chat id: %s", chatID))
		return nil, channels.RespondError(w, http.StatusBadRequest, fmt.Errorf("unknown chat id"))
	}

	msgs, err := models.GetChatMsgs(ctx, h.Runtime().DB, channel, contact.URNID_, before, historyPageSize)
	if err != nil {
		return nil, fmt.Errorf("error loading chat messages: %w", err)
	}

	resp := &historyResponse{Events: make([]*msgEvent, len(msgs))}
	for i, m := range msgs {
		typ := eventTypeMsgOut
		if m.Direction == models.MsgIncoming {
			typ = eventTypeMsgIn
		}
		resp.Events[i] = &msgEvent{
			Type:         typ,
			CreatedOn:    m.CreatedOn.UTC(), // live events are stamped in UTC so history should look the same
			MsgUUID:      m.UUID,
			Text:         m.Text,
			Attachments:  m.Attachments,
			QuickReplies: handlers.FilterQuickRepliesByType(m.QuickReplies, models.QuickReplyTypeText),
		}
	}

	// a full page may have older messages behind it, and its oldest item's UUID is the cursor to them
	if len(msgs) == historyPageSize {
		resp.Next = string(msgs[len(msgs)-1].UUID)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonx.MustMarshal(resp))
	return nil, nil
}

// msgEvent is how a chat client sees a message: published to the conversation's chat socket as each outgoing
// message is sent, and returned by the history endpoint for messages in both directions - one shape for the
// client to parse whether a message arrives live or is fetched later. Chat events are their own client-centric
// vocabulary rather than engine events, though the content fields keep the same shapes as the engine's msg
// events - attachments as content-type:url strings, quick replies as objects.
type msgEvent struct {
	Type         string              `json:"type"`
	CreatedOn    time.Time           `json:"created_on"`
	MsgUUID      models.MsgUUID      `json:"msg_uuid"`
	Text         string              `json:"text"`
	Attachments  []string            `json:"attachments,omitempty"`
	QuickReplies []models.QuickReply `json:"quick_replies,omitempty"`
}

func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	socket := models.ChatSocket(msg.Channel().UUID(), msg.URN().Path())
	// the widget only renders text quick replies, so like any other channel we filter to what's supported
	event := &msgEvent{
		Type:         eventTypeMsgOut,
		CreatedOn:    dates.Now(),
		MsgUUID:      msg.UUID(),
		Text:         msg.Text(),
		Attachments:  msg.Attachments(),
		QuickReplies: handlers.FilterQuickRepliesByType(msg.QuickReplies(), models.QuickReplyTypeText),
	}

	// like all socket publishes this is presence-aware and best-effort: if the visitor doesn't currently have
	// the chat open the publish is dropped, and the message is still considered sent
	if err := h.Runtime().Centrifugo.Publish(ctx, &centrifugo.Publication{Channel: socket, Data: event}); err != nil {
		return fmt.Errorf("error publishing message event: %w", err)
	}

	return nil
}
