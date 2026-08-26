package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/gocommon/centrifugo"
	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
)

// These endpoints are the channel proxies the realtime messaging server calls to authorize a webchat visitor's
// subscription to their conversation's chat socket ("chat:<channel-uuid>:<chat-id>"): subscribe when the browser
// first asks to subscribe, and sub_refresh periodically before each subscription's expire_at so authorization is
// re-checked. Both are default-deny: the socket name must parse, the channel must be an active WebChat channel,
// and the chat ID must be one we've actually issued (i.e. its webchat URN exists in the channel's org).
//
// On allow, the subscription is recorded as a presence key in valkey so publishers know the socket has a live
// subscriber - the key name and TTL semantics are a contract shared with the other services that read and write
// these keys (see gocommon's centrifugo.SubscriptionKey).

// how far ahead (seconds) we set a subscription's expire_at, scheduling the realtime server to call sub_refresh
// before it lapses so we can re-authorize
const chatSubscriptionWindow = 60

// how long (seconds) a socket's presence key survives without a refresh. It must comfortably exceed
// chatSubscriptionWindow plus the realtime server's refresh delay (which can be up to ~1 minute), and there's no
// unsubscribe callback so this TTL is the only garbage collection.
const chatSubscriptionTTL = 150

// the socket names we authorize: chat:<channel-uuid>:<chat-id>, where the chat ID is the 24 alphanumeric chars of
// a webchat URN path. Like a URL conf, this does all the shape validation - anything that doesn't fully match is
// denied before we look anything up.
var chatSocketRegex = regexp.MustCompile(`^chat:([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}):([a-zA-Z0-9]{24})$`)

// the parts of the proxy request bodies we care about: the requested socket name, in the realtime server's own
// protocol field naming ("channel" there means a pub/sub channel, i.e. what we call a socket)
type chatProxyRequest struct {
	Channel string `json:"channel"`
}

func (s *Server) handleChatSubscribe(w http.ResponseWriter, r *http.Request) {
	s.handleChatProxy(w, r, false)
}

func (s *Server) handleChatSubRefresh(w http.ResponseWriter, r *http.Request) {
	s.handleChatProxy(w, r, true)
}

// handles a subscribe or sub_refresh proxy request. Both run the same authorization; they differ in how a denial
// is reported: a denied subscribe is a forbidden error, while a denied refresh just reports the subscription
// expired (only the result is acted on for refreshes).
func (s *Server) handleChatProxy(w http.ResponseWriter, r *http.Request, refresh bool) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Second*15)
	defer cancel()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		channels.WriteError(w, http.StatusBadRequest, fmt.Errorf("error reading request body: %w", err))
		return
	}

	req := &chatProxyRequest{}
	if err := json.Unmarshal(body, req); err != nil {
		channels.WriteError(w, http.StatusBadRequest, fmt.Errorf("error unmarshalling request: %w", err))
		return
	}

	if s.chatSubscriptionAllowed(ctx, req.Channel) {
		if err := s.recordChatSubscription(ctx, req.Channel); err != nil {
			// without the presence key nothing would be published to the socket, so a subscription we can't
			// record is an error rather than a success that silently receives nothing
			slog.Error("error recording chat subscription", "error", err, "socket", req.Channel)
			channels.WriteError(w, http.StatusInternalServerError, fmt.Errorf("error recording subscription"))
			return
		}

		writeChatProxyResponse(w, map[string]any{"result": map[string]any{"expire_at": dates.Now().Unix() + chatSubscriptionWindow}})
	} else if refresh {
		writeChatProxyResponse(w, map[string]any{"result": map[string]any{"expired": true}})
	} else {
		writeChatProxyResponse(w, map[string]any{"error": map[string]any{"code": 403, "message": "forbidden"}})
	}
}

// default-deny authorization of a requested chat socket
func (s *Server) chatSubscriptionAllowed(ctx context.Context, socket string) bool {
	m := chatSocketRegex.FindStringSubmatch(socket)
	if m == nil {
		return false
	}

	// the channel must exist, be active and be a webchat channel
	ch, err := models.GetChannel(ctx, models.ChannelType("WCH"), models.ChannelUUID(m[1]))
	if err != nil {
		return false
	}

	// and the chat ID must be one issued on that channel, i.e. its URN exists in the channel's org
	urn, err := urns.NewFromParts(urns.WebChat.Prefix, m[2], nil, "")
	if err != nil {
		return false
	}

	var exists bool
	if err := s.rt.DB.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM contacts_contacturn WHERE org_id = $1 AND identity = $2)`, ch.OrgID(), urn.Identity()); err != nil {
		slog.Error("error looking up webchat URN", "error", err, "socket", socket)
		return false
	}
	return exists
}

// marks the socket as having at least one active subscriber by (re)setting its presence key - every subscribe and
// sub_refresh re-arms it, and it expires once the last subscriber stops refreshing
func (s *Server) recordChatSubscription(ctx context.Context, socket string) error {
	rc := s.rt.VK.Get()
	defer rc.Close()

	_, err := rc.Do("SET", centrifugo.SubscriptionKey(socket), "1", "EX", chatSubscriptionTTL)
	return err
}

func writeChatProxyResponse(w http.ResponseWriter, resp map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonx.MustMarshal(resp))
}
