package courier

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func testConfig() *Config {
	config := NewConfig()
	config.DB = "postgres://courier@localhost/courier_test?sslmode=disable"
	config.Redis = "redis://localhost:6379/0"
	return config
}

func TestSending(t *testing.T) {
	assert := assert.New(t)

	// create our backend and server
	mb := NewMockBackend()
	s := NewServer(testConfig(), mb)

	// start everything
	s.Start()
	defer s.Stop()

	// create and add a new outgoing message
	xxChannel := NewMockChannel("53e5aafa-8155-449d-9009-fcb30d54bd26", "XX", "2020", "US", map[string]interface{}{})
	dmChannel := NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230", "DM", "2020", "US", map[string]interface{}{})
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
}
