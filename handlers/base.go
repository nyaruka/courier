package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/utils"
	"github.com/nyaruka/goflow/core/events"
)

var defaultRedactConfigKeys = []string{models.ConfigAuthToken, models.ConfigAPIKey, models.ConfigSecret, models.ConfigPassword, models.ConfigSendAuthorization}

// BaseHandler is the base class for most handlers, it just stored the runtime, name and channel type for the handler
type BaseHandler struct {
	channelType        models.ChannelType
	name               string
	rt                 *runtime.Runtime
	uuidChannelRouting bool
	storeChannelLogs   bool
	redactConfigKeys   []string
}

// NewBaseHandler returns a newly constructed BaseHandler with the passed in parameters
func NewBaseHandler(channelType models.ChannelType, name string, options ...func(*BaseHandler)) BaseHandler {
	h := &BaseHandler{
		channelType:        channelType,
		name:               name,
		uuidChannelRouting: true,
		storeChannelLogs:   true,
		redactConfigKeys:   defaultRedactConfigKeys,
	}
	for _, o := range options {
		o(h)
	}
	return *h
}

func DisableUUIDRouting() func(*BaseHandler) {
	return func(s *BaseHandler) {
		s.uuidChannelRouting = false
	}
}

// DisableChannelLogStorage is for channel types whose traffic is internal to the platform - their channel logs
// wouldn't describe anything users could act on, so they aren't persisted for them to view
func DisableChannelLogStorage() func(*BaseHandler) {
	return func(s *BaseHandler) {
		s.storeChannelLogs = false
	}
}

func WithRedactConfigKeys(keys ...string) func(*BaseHandler) {
	return func(s *BaseHandler) {
		s.redactConfigKeys = keys
	}
}

// SetRuntime gives the handler the runtime it should use, and is called before Initialize
func (h *BaseHandler) SetRuntime(rt *runtime.Runtime) {
	h.rt = rt
}

// Runtime returns the runtime instance on the BaseHandler
func (h *BaseHandler) Runtime() *runtime.Runtime {
	return h.rt
}

// ChannelType returns the channel type that this handler deals with
func (h *BaseHandler) ChannelType() models.ChannelType {
	return h.channelType
}

// ChannelName returns the name of the channel this handler deals with
func (h *BaseHandler) ChannelName() string {
	return h.name
}

// UseChannelRouteUUID returns whether the router should use the channel UUID in the URL path
func (h *BaseHandler) UseChannelRouteUUID() bool {
	return h.uuidChannelRouting
}

// StoreChannelLogs returns whether channel logs for this handler's channels should be persisted for users to view
func (h *BaseHandler) StoreChannelLogs() bool {
	return h.storeChannelLogs
}

func (h *BaseHandler) RedactValues(ch *models.Channel) []string {
	if ch == nil {
		return nil
	}

	vals := make([]string, 0, len(h.redactConfigKeys))
	for _, k := range h.redactConfigKeys {
		v := ch.StringConfigForKey(k, "")
		if v != "" {
			vals = append(vals, v)
		}
	}
	return vals
}

// SendableEvents declares no support for sending any events - handlers that can send them should override
func (h *BaseHandler) SendableEvents(*models.Channel) map[string]time.Duration {
	return nil
}

// SendEvent is a stub for handlers that can't send events and shouldn't be reachable because
// SendableEvents declares no support
func (h *BaseHandler) SendEvent(ctx context.Context, ch *models.Channel, event events.Event, clog *models.ChannelLog) error {
	return fmt.Errorf("event sending not supported by %s handler", h.channelType)
}

// GetChannel returns the channel
func (h *BaseHandler) GetChannel(ctx context.Context, r *http.Request) (*models.Channel, error) {
	uuid := models.ChannelUUID(r.PathValue("uuid"))
	return models.GetChannel(ctx, h.ChannelType(), uuid)
}

// RequestHTTP does the given request, logging the trace, and returns the response
func (h *BaseHandler) RequestHTTP(req *http.Request, clog *models.ChannelLog) (*http.Response, []byte, error) {
	return h.requestHTTP(h.rt.HTTP.Default, req, clog)
}

// RequestHTTPProxied is like RequestHTTP but routes through the configured outbound proxy
// (SendProxyURL) when one is set. Use this for handlers that send to user-configured URLs.
func (h *BaseHandler) RequestHTTPProxied(req *http.Request, clog *models.ChannelLog) (*http.Response, []byte, error) {
	return h.requestHTTP(h.rt.HTTP.Proxied, req, clog)
}

// requestHTTP does the given request using the given client, logging the trace, and returns the response
func (h *BaseHandler) requestHTTP(client *http.Client, req *http.Request, clog *models.ChannelLog) (*http.Response, []byte, error) {
	req.Header.Set("User-Agent", userAgent(h.rt.Config.Version))

	// the client's transport is shared and already enforces access control (the SSRF blocklist)
	trace, resp, err := utils.DoTraced(client, req)

	var body []byte
	if trace != nil {
		clog.HTTP(trace)
		body = trace.ResponseBody
	}
	if err != nil {
		return nil, nil, err
	}

	return resp, body, nil
}

// userAgent returns the User-Agent header value for handler HTTP calls. Only the major.minor
// portion of the version is included to avoid leaking specific build details.
func userAgent(version string) string {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) >= 2 {
		return "Courier/" + parts[0] + "." + parts[1]
	}
	return "Courier/" + version
}

// WriteStatusSuccessResponse writes a success response for the statuses
func (h *BaseHandler) WriteStatusSuccessResponse(ctx context.Context, w http.ResponseWriter, statuses []*models.StatusUpdate) error {
	return channels.WriteStatusSuccess(w, statuses)
}

// WriteMsgSuccessResponse writes a success response for the messages
func (h *BaseHandler) WriteMsgSuccessResponse(ctx context.Context, w http.ResponseWriter, msgs []*models.MsgIn) error {
	return channels.WriteMsgSuccess(w, msgs)
}

// WriteRequestError writes the passed in error to our response writer
func (h *BaseHandler) WriteRequestError(ctx context.Context, w http.ResponseWriter, err error) error {
	return channels.WriteError(w, http.StatusBadRequest, err)
}

// WriteRequestIgnored writes an ignored payload to our response writer
func (h *BaseHandler) WriteRequestIgnored(ctx context.Context, w http.ResponseWriter, details string) error {
	return channels.WriteIgnored(w, details)
}

// WithValkeyConn is a utility to execute some code with a valkey connection
func (h *BaseHandler) WithValkeyConn(fn func(rc redis.Conn)) {
	rc := h.Runtime().VK.Get()
	defer rc.Close()
	fn(rc)
}
