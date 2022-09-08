package courier_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
)

func init() {
	courier.RegisterHandler(NewHandler())
}

type dummyHandler struct {
	server  courier.Server
	backend courier.Backend
}

// NewHandler returns a new Dummy handler
func NewHandler() courier.ChannelHandler {
	return &dummyHandler{}
}

func (h *dummyHandler) ChannelName() string                   { return "Dummy Handler" }
func (h *dummyHandler) ChannelType() courier.ChannelType      { return courier.ChannelType("DM") }
func (h *dummyHandler) UseChannelRouteUUID() bool             { return true }
func (h *dummyHandler) RedactValues(courier.Channel) []string { return []string{"sesame"} }

func (h *dummyHandler) GetChannel(ctx context.Context, r *http.Request) (courier.Channel, error) {
	dmChannel := test.NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230", "DM", "2020", "US", map[string]interface{}{})
	return dmChannel, nil
}

// Initialize is called by the engine once everything is loaded
func (h *dummyHandler) Initialize(s courier.Server) error {
	h.server = s
	h.backend = s.Backend()
	s.AddHandlerRoute(h, http.MethodGet, "receive", h.receiveMsg)
	return nil
}

// Send sends the given message, logging any HTTP calls or errors
func (h *dummyHandler) Send(ctx context.Context, msg courier.Msg, clog *courier.ChannelLog) (courier.MsgStatus, error) {
	return h.backend.NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgSent, clog), nil
}

// ReceiveMsg sends the passed in message, returning any error
func (h *dummyHandler) receiveMsg(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	r.ParseForm()
	from := r.Form.Get("from")
	text := r.Form.Get("text")
	if from == "" || text == "" {
		return nil, errors.New("missing from or text")
	}

	msg := h.backend.NewIncomingMsg(channel, urns.URN("tel:"+from), text, clog)
	w.WriteHeader(200)
	w.Write([]byte("ok"))
	h.backend.WriteMsg(ctx, msg, clog)
	return []courier.Event{msg}, nil
}

func testConfig() *courier.Config {
	config := courier.NewConfig()
	config.DB = "postgres://courier:courier@localhost:5432/courier_test?sslmode=disable"
	config.Redis = "redis://localhost:6379/0"
	return config
}

func TestHandling(t *testing.T) {
	assert := assert.New(t)

	// create our backend and server
	mb := test.NewMockBackend()
	s := courier.NewServer(testConfig(), mb)

	// start everything
	s.Start()
	defer s.Stop()

	time.Sleep(100 * time.Millisecond)

	// create and add a new outgoing message
	xxChannel := test.NewMockChannel("53e5aafa-8155-449d-9009-fcb30d54bd26", "XX", "2020", "US", map[string]interface{}{})
	dmChannel := test.NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230", "DM", "2020", "US", map[string]interface{}{})
	mb.AddChannel(dmChannel)

	msg := test.NewMockMsg(courier.NewMsgID(101), courier.NilMsgUUID, xxChannel, "tel:+250788383383", "test message")
	mb.PushOutgoingMsg(msg)

	// sleep a second, sender should take care of it in that time
	time.Sleep(time.Second)

	// message should have failed because we don't have a registered handler
	assert.Equal(1, len(mb.WrittenMsgStatuses()))
	assert.Equal(msg.ID(), mb.WrittenMsgStatuses()[0].ID())
	assert.Equal(courier.MsgFailed, mb.WrittenMsgStatuses()[0].Status())
	assert.Equal(1, len(mb.WrittenChannelLogs()))

	mb.Reset()

	// change our channel to our dummy channel
	msg = test.NewMockMsg(courier.NewMsgID(102), courier.NilMsgUUID, dmChannel, "tel:+250788383383", "test message 2")

	// send it
	mb.PushOutgoingMsg(msg)
	time.Sleep(time.Second)

	// message should be marked as wired
	assert.Equal(1, len(mb.WrittenMsgStatuses()))
	assert.Equal(msg.ID(), mb.WrittenMsgStatuses()[0].ID())
	assert.Equal(courier.MsgSent, mb.WrittenMsgStatuses()[0].Status())

	mb.Reset()

	// send the message again, should be skipped but again marked as wired
	mb.PushOutgoingMsg(msg)
	time.Sleep(time.Second)

	// message should be marked as wired
	assert.Equal(1, len(mb.WrittenMsgStatuses()))
	assert.Equal(msg.ID(), mb.WrittenMsgStatuses()[0].ID())
	assert.Equal(courier.MsgWired, mb.WrittenMsgStatuses()[0].Status())

	// try to receive a message instead
	resp, err := http.Get("http://localhost:8080/c/dm/e4bb1578-29da-4fa5-a214-9da19dd24230/receive")
	assert.NoError(err)
	assert.Equal(400, resp.StatusCode)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(string(body), "missing from or text")

	req, _ := http.NewRequest("GET", "http://localhost:8080/c/dm/e4bb1578-29da-4fa5-a214-9da19dd24230/receive?from=2065551212&text=hello", nil)
	req.Header.Set("Cookie", "secret")
	resp, err = http.DefaultClient.Do(req)
	assert.NoError(err)
	assert.Equal(200, resp.StatusCode)
	defer resp.Body.Close()
	body, _ = io.ReadAll(resp.Body)
	assert.Contains(string(body), "ok")
}
