package test

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/utils"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/svclogs"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/core/events"
)

func init() {
	channels.RegisterHandler(NewMockHandler())
}

type mockHandler struct {
	rt *runtime.Runtime
}

// NewMockHandler returns a new mock handler
func NewMockHandler() channels.Handler {
	return &mockHandler{}
}

func (h *mockHandler) Runtime() *runtime.Runtime             { return h.rt }
func (h *mockHandler) SetRuntime(rt *runtime.Runtime)        { h.rt = rt }
func (h *mockHandler) ChannelName() string                   { return "Mock Handler" }
func (h *mockHandler) ChannelType() models.ChannelType       { return models.ChannelType("MCK") }
func (h *mockHandler) UseChannelRouteUUID() bool             { return true }
func (h *mockHandler) RedactValues(*models.Channel) []string { return []string{"sesame"} }

func (h *mockHandler) GetChannel(ctx context.Context, r *http.Request) (*models.Channel, error) {
	return models.GetChannel(ctx, "MCK", "e4bb1578-29da-4fa5-a214-9da19dd24230")
}

// Initialize is called by the engine once everything is loaded
func (h *mockHandler) Initialize(r *channels.Routes) error {
	r.Add(h, http.MethodGet, "receive", models.ChannelLogTypeMsgReceive, h.receiveMsg)
	return nil
}

// Send sends the given message, logging any HTTP calls or errors
func (h *mockHandler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	// log a request that contains a header value that should be redacted; goes through the runtime's
	// HTTP client so tests can intercept it with a mocking transport
	req, _ := httpx.NewRequest(ctx, "GET", "http://mock.com/send", nil, map[string]string{"Authorization": "Token sesame"})
	trace, resp, err := utils.DoTraced(h.rt.HTTP.Default, req)
	if trace != nil {
		clog.HTTP(trace)
	}

	if err != nil || resp.StatusCode/100 == 5 {
		return channels.ErrConnectionFailed
	} else if resp.StatusCode == 403 {
		return channels.ErrContactStopped
	} else if resp.StatusCode == 429 {
		return channels.ErrConnectionThrottled
	}

	// log an error than contains a value that should be redacted
	clog.Error(&svclogs.Error{Code: "seeds", Message: "contains sesame seeds"})

	if msg.Text() == "err:config" {
		return channels.ErrChannelConfig
	}

	return nil
}

// SendEvent sends the given event, logging any HTTP calls or errors
func (h *mockHandler) SendEvent(ctx context.Context, ch *models.Channel, event events.Event, clog *models.ChannelLog) error {
	req, _ := httpx.NewRequest(ctx, "POST", "http://mock.com/action", nil, nil)
	trace, resp, err := utils.DoTraced(h.rt.HTTP.Default, req)
	if trace != nil {
		clog.HTTP(trace)
	}

	if err != nil || resp.StatusCode/100 != 2 {
		return channels.ErrConnectionFailed
	}
	return nil
}

// SendableEvents declares support for typing started with a 10 second resend interval, plus typing
// stopped for channels configured with supports_stop - so tests can cover both capability cases
func (h *mockHandler) SendableEvents(ch *models.Channel) map[string]time.Duration {
	if ch.BoolConfigForKey("supports_stop", false) {
		return map[string]time.Duration{events.TypeTypingStarted: 10 * time.Second, events.TypeTypingStopped: 0}
	}
	return map[string]time.Duration{events.TypeTypingStarted: 10 * time.Second}
}

func (h *mockHandler) WriteStatusSuccessResponse(ctx context.Context, w http.ResponseWriter, statuses []*models.StatusUpdate) error {
	return channels.WriteStatusSuccess(w, statuses)
}

func (h *mockHandler) WriteMsgSuccessResponse(ctx context.Context, w http.ResponseWriter, msgs []*models.MsgIn) error {
	return channels.WriteMsgSuccess(w, msgs)
}

func (h *mockHandler) WriteRequestError(ctx context.Context, w http.ResponseWriter, err error) error {
	return channels.WriteError(w, http.StatusBadRequest, err)
}

func (h *mockHandler) WriteRequestIgnored(ctx context.Context, w http.ResponseWriter, details string) error {
	return channels.WriteIgnored(w, details)
}

// ReceiveMsg sends the passed in message, returning any error
func (h *mockHandler) receiveMsg(ctx context.Context, channel *models.Channel, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
	r.ParseForm()
	from := r.Form.Get("from")
	text := r.Form.Get("text")
	if from == "" || text == "" {
		return nil, errors.New("missing from or text")
	}

	msg := models.NewIncomingMsg(channel, urns.URN("tel:"+from), text, "", clog)
	w.WriteHeader(200)
	w.Write([]byte("ok"))
	models.WriteMsg(ctx, h.rt, msg, clog)
	return []channels.Event{msg}, nil
}
