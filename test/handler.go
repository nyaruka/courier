package test

import (
	"context"
	"net/http"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/pkg/errors"
)

func init() {
	courier.RegisterHandler(NewMockHandler())
}

type mockHandler struct {
	server  courier.Server
	backend courier.Backend
}

// NewMockHandler returns a new mock handler
func NewMockHandler() courier.ChannelHandler {
	return &mockHandler{}
}

func (h *mockHandler) Server() courier.Server                { return h.server }
func (h *mockHandler) ChannelName() string                   { return "Mock Handler" }
func (h *mockHandler) ChannelType() courier.ChannelType      { return courier.ChannelType("MCK") }
func (h *mockHandler) UseChannelRouteUUID() bool             { return true }
func (h *mockHandler) RedactValues(courier.Channel) []string { return []string{"sesame"} }

func (h *mockHandler) GetChannel(ctx context.Context, r *http.Request) (courier.Channel, error) {
	dmChannel := NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230", "MCK", "2020", "US", map[string]any{})
	return dmChannel, nil
}

// Initialize is called by the engine once everything is loaded
func (h *mockHandler) Initialize(s courier.Server) error {
	h.server = s
	h.backend = s.Backend()
	s.AddHandlerRoute(h, http.MethodGet, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMsg)
	return nil
}

// Send sends the given message, logging any HTTP calls or errors
func (h *mockHandler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	// log a request that contains a header value that should be redacted
	req, _ := httpx.NewRequest("GET", "http://mock.com/send", nil, map[string]string{"Authorization": "Token sesame"})
	trace, _ := httpx.DoTrace(http.DefaultClient, req, nil, nil, 1024)
	clog.HTTP(trace)

	// log an error than contains a value that should be redacted
	clog.Error(courier.NewChannelError("seeds", "", "contains sesame seeds"))

	return h.backend.NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusSent, clog), nil
}

func (h *mockHandler) WriteStatusSuccessResponse(ctx context.Context, w http.ResponseWriter, statuses []courier.StatusUpdate) error {
	return courier.WriteStatusSuccess(w, statuses)
}

func (h *mockHandler) WriteMsgSuccessResponse(ctx context.Context, w http.ResponseWriter, msgs []courier.MsgIn) error {
	return courier.WriteMsgSuccess(w, msgs)
}

func (h *mockHandler) WriteRequestError(ctx context.Context, w http.ResponseWriter, err error) error {
	return courier.WriteError(w, http.StatusBadRequest, err)
}

func (h *mockHandler) WriteRequestIgnored(ctx context.Context, w http.ResponseWriter, details string) error {
	return courier.WriteIgnored(w, details)
}

// ReceiveMsg sends the passed in message, returning any error
func (h *mockHandler) receiveMsg(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	r.ParseForm()
	from := r.Form.Get("from")
	text := r.Form.Get("text")
	if from == "" || text == "" {
		return nil, errors.New("missing from or text")
	}

	msg := h.backend.NewIncomingMsg(channel, urns.URN("tel:"+from), text, "", clog)
	w.WriteHeader(200)
	w.Write([]byte("ok"))
	h.backend.WriteMsg(ctx, msg, clog)
	return []courier.Event{msg}, nil
}
