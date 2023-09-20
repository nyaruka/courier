package courier_test

import (
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/stretchr/testify/assert"
)

func testConfig() *courier.Config {
	config := courier.NewConfig()
	config.DB = "postgres://courier_test:temba@localhost:5432/courier_test?sslmode=disable"
	config.Redis = "redis://localhost:6379/0"
	return config
}

func TestHandling(t *testing.T) {
	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]*httpx.MockResponse{
		"http://mock.com/send": {
			httpx.NewMockResponse(200, nil, []byte(`SENT`)),
		},
	}))

	assert := assert.New(t)

	// create our backend and server
	mb := test.NewMockBackend()
	s := courier.NewServer(testConfig(), mb)

	// start everything
	s.Start()
	defer s.Stop()

	time.Sleep(100 * time.Millisecond)

	// create and add a new outgoing message
	brokenChannel := test.NewMockChannel("53e5aafa-8155-449d-9009-fcb30d54bd26", "XX", "2020", "US", map[string]any{})
	mockChannel := test.NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230", "MCK", "2020", "US", map[string]any{})
	mb.AddChannel(mockChannel)

	msg := test.NewMockMsg(courier.MsgID(101), courier.NilMsgUUID, brokenChannel, "tel:+250788383383", "test message", nil)
	mb.PushOutgoingMsg(msg)

	// sleep a second, sender should take care of it in that time
	time.Sleep(time.Second)

	// message should have failed because we don't have a registered handler
	assert.Equal(1, len(mb.WrittenMsgStatuses()))
	assert.Equal(msg.ID(), mb.WrittenMsgStatuses()[0].MsgID())
	assert.Equal(courier.MsgStatusFailed, mb.WrittenMsgStatuses()[0].Status())
	assert.Equal(1, len(mb.WrittenChannelLogs()))

	mb.Reset()

	// change our channel to our dummy channel
	msg = test.NewMockMsg(courier.MsgID(102), courier.NilMsgUUID, mockChannel, "tel:+250788383383", "test message 2", nil)

	// send it
	mb.PushOutgoingMsg(msg)
	time.Sleep(time.Second)

	// message should be marked as wired
	assert.Len(mb.WrittenMsgStatuses(), 1)
	status := mb.WrittenMsgStatuses()[0]
	assert.Equal(msg.ID(), status.MsgID())
	assert.Equal(courier.MsgStatusSent, status.Status())

	assert.Len(mb.WrittenChannelLogs(), 1)
	clog := mb.WrittenChannelLogs()[0]
	assert.Equal([]*courier.ChannelError{courier.NewChannelError("seeds", "", "contains ********** seeds")}, clog.Errors())

	assert.Len(clog.HTTPLogs(), 1)

	hlog := clog.HTTPLogs()[0]
	assert.Equal("http://mock.com/send", hlog.URL)
	assert.Equal("GET /send HTTP/1.1\r\nHost: mock.com\r\nUser-Agent: Go-http-client/1.1\r\nAuthorization: Token **********\r\nAccept-Encoding: gzip\r\n\r\n", hlog.Request)

	mb.Reset()

	// send the message again, should be skipped but again marked as wired
	mb.PushOutgoingMsg(msg)
	time.Sleep(time.Second)

	// message should be marked as wired
	assert.Equal(1, len(mb.WrittenMsgStatuses()))
	assert.Equal(msg.ID(), mb.WrittenMsgStatuses()[0].MsgID())
	assert.Equal(courier.MsgStatusWired, mb.WrittenMsgStatuses()[0].Status())

	// try to receive a message instead
	resp, err := http.Get("http://localhost:8080/c/mck/e4bb1578-29da-4fa5-a214-9da19dd24230/receive")
	assert.NoError(err)
	assert.Equal(400, resp.StatusCode)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(string(body), "missing from or text")

	req, _ := http.NewRequest("GET", "http://localhost:8080/c/mck/e4bb1578-29da-4fa5-a214-9da19dd24230/receive?from=2065551212&text=hello", nil)
	req.Header.Set("Cookie", "secret")
	resp, err = http.DefaultClient.Do(req)
	assert.NoError(err)
	assert.Equal(200, resp.StatusCode)
	defer resp.Body.Close()
	body, _ = io.ReadAll(resp.Body)
	assert.Contains(string(body), "ok")
}
