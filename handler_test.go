package courier

import (
	"context"
	"errors"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
)

func init() {
	RegisterHandler(NewHandler())
}

type dummyHandler struct {
	server  Server
	backend Backend
}

// NewHandler returns a new Dummy handler
func NewHandler() ChannelHandler {
	return &dummyHandler{}
}

func (h *dummyHandler) ChannelName() string      { return "Dummy Handler" }
func (h *dummyHandler) ChannelType() ChannelType { return ChannelType("DM") }

// Initialize is called by the engine once everything is loaded
func (h *dummyHandler) Initialize(s Server) error {
	h.server = s
	h.backend = s.Backend()
	s.AddHandlerRoute(h, http.MethodGet, "receive", h.receiveMsg)
	return nil
}

// SendMsg sends the passed in message, returning any error
func (h *dummyHandler) SendMsg(ctx context.Context, msg Msg) (MsgStatus, error) {
	return h.backend.NewMsgStatusForID(msg.Channel(), msg.ID(), MsgSent), nil
}

// ReceiveMsg sends the passed in message, returning any error
func (h *dummyHandler) receiveMsg(ctx context.Context, channel Channel, w http.ResponseWriter, r *http.Request) ([]Event, error) {
	r.ParseForm()
	from := r.Form.Get("from")
	text := r.Form.Get("text")
	if from == "" || text == "" {
		return nil, errors.New("missing from or text")
	}

	msg := h.backend.NewIncomingMsg(channel, urns.URN("tel:"+from), text)
	w.WriteHeader(200)
	w.Write([]byte("ok"))
	h.backend.WriteMsg(ctx, msg)
	return []Event{msg}, nil
}

func testConfig() *Config {
	config := NewConfig()
	config.DB = "postgres://courier:courier@localhost:5432/courier_test?sslmode=disable"
	config.Redis = "redis://localhost:6379/0"
	return config
}

func TestHandling(t *testing.T) {
	assert := assert.New(t)

	// create our backend and server
	mb := NewMockBackend()
	s := NewServer(testConfig(), mb)

	// start everything
	s.Start()
	defer s.Stop()

	time.Sleep(100 * time.Millisecond)

	// create and add a new outgoing message
	xxChannel := NewMockChannel("53e5aafa-8155-449d-9009-fcb30d54bd26", "XX", "2020", "US", map[string]interface{}{})
	dmChannel := NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230", "DM", "2020", "US", map[string]interface{}{})
	mb.AddChannel(dmChannel)

	msg := &mockMsg{
		channel: xxChannel,
		id:      NewMsgID(101),
		uuid:    NilMsgUUID,
		text:    "test message",
		urn:     "tel:+250788383383",
	}
	mb.PushOutgoingMsg(msg)

	// sleep a second, sender should take care of it in that time
	time.Sleep(time.Second)

	// message should have errored because we have registered handlers
	assert.Equal(1, len(mb.msgStatuses))
	assert.Equal(msg.ID(), mb.msgStatuses[0].ID())
	assert.Equal(MsgErrored, mb.msgStatuses[0].Status())
	assert.Equal(1, len(mb.msgStatuses[0].Logs()))

	// clear our statuses
	mb.msgStatuses = nil

	// change our channel to our dummy channel
	msg = &mockMsg{
		channel: dmChannel,
		id:      NewMsgID(102),
		uuid:    NilMsgUUID,
		text:    "test message 2",
		urn:     "tel:+250788383383",
	}

	// send it
	mb.PushOutgoingMsg(msg)
	time.Sleep(time.Second)

	// message should be marked as wired
	assert.Equal(1, len(mb.msgStatuses))
	assert.Equal(msg.ID(), mb.msgStatuses[0].ID())
	assert.Equal(MsgSent, mb.msgStatuses[0].Status())

	// clear our statuses
	mb.msgStatuses = nil

	// send the message again, should be skipped but again marked as wired
	mb.PushOutgoingMsg(msg)
	time.Sleep(time.Second)

	// message should be marked as wired
	assert.Equal(1, len(mb.msgStatuses))
	assert.Equal(msg.ID(), mb.msgStatuses[0].ID())
	assert.Equal(MsgWired, mb.msgStatuses[0].Status())

	// try to receive a message instead
	resp, err := http.Get("http://localhost:8080/c/dm/e4bb1578-29da-4fa5-a214-9da19dd24230/receive")
	assert.NoError(err)
	assert.Equal(400, resp.StatusCode)
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	assert.Contains(string(body), "missing from or text")

	req, _ := http.NewRequest("GET", "http://localhost:8080/c/dm/e4bb1578-29da-4fa5-a214-9da19dd24230/receive?from=2065551212&text=hello", nil)
	req.Header.Set("Cookie", "secret")
	resp, err = http.DefaultClient.Do(req)
	assert.NoError(err)
	assert.Equal(200, resp.StatusCode)
	defer resp.Body.Close()
	body, _ = ioutil.ReadAll(resp.Body)
	assert.Contains(string(body), "ok")

	// cookie stripped
	log, _ := mb.GetLastChannelLog()
	assert.NotContains(log.Request, "secret")
}
