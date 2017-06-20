package courier

import (
	"errors"

	_ "github.com/lib/pq" // postgres driver
	"github.com/nyaruka/courier/config"
)

//-----------------------------------------------------------------------------
// Mock backend implementation
//-----------------------------------------------------------------------------

// MockBackend is a mocked version of a backend which doesn't require a real database or cache
type MockBackend struct {
	channels     map[ChannelUUID]Channel
	queueMsgs    []*Msg
	errorOnQueue bool
}

// NewMockBackend returns a new mock backend suitable for testing
func NewMockBackend() *MockBackend {
	return &MockBackend{channels: make(map[ChannelUUID]Channel)}
}

// GetLastQueueMsg returns the last message queued to the server
func (mb *MockBackend) GetLastQueueMsg() (*Msg, error) {
	if len(mb.queueMsgs) == 0 {
		return nil, ErrMsgNotFound
	}
	return mb.queueMsgs[len(mb.queueMsgs)-1], nil
}

// PopNextOutgoingMsg returns the next message that should be sent, or nil if there are none to send
func (mb *MockBackend) PopNextOutgoingMsg() (*Msg, error) {
	return nil, nil
}

// MarkOutgoingMsgComplete marks the passed msg as having been dealt with
func (mb *MockBackend) MarkOutgoingMsgComplete(m *Msg) {
}

// WriteChannelLogs writes the passed in channel logs to the DB
func (mb *MockBackend) WriteChannelLogs(logs []*ChannelLog) error {
	return nil
}

// SetErrorOnQueue is a mock method which makes the QueueMsg call throw the passed in error on next call
func (mb *MockBackend) SetErrorOnQueue(shouldError bool) {
	mb.errorOnQueue = shouldError
}

// WriteMsg queues the passed in message internally
func (mb *MockBackend) WriteMsg(m *Msg) error {
	if mb.errorOnQueue {
		return errors.New("unable to queue message")
	}

	mb.queueMsgs = append(mb.queueMsgs, m)
	return nil
}

// WriteMsgStatus writes the status update to our queue
func (mb *MockBackend) WriteMsgStatus(status *MsgStatusUpdate) error {
	return nil
}

// GetChannel returns the channel with the passed in type and channel uuid
func (mb *MockBackend) GetChannel(cType ChannelType, uuid ChannelUUID) (Channel, error) {
	channel, found := mb.channels[uuid]
	if !found {
		return nil, ErrChannelNotFound
	}
	return channel, nil
}

// AddChannel adds a test channel to the test server
func (mb *MockBackend) AddChannel(channel Channel) {
	mb.channels[channel.UUID()] = channel
}

// ClearChannels is a utility function on our mock server to clear all added channels
func (mb *MockBackend) ClearChannels() {
	mb.channels = nil
}

// Start starts our mock backend
func (mb *MockBackend) Start() error { return nil }

// Stop stops our mock backend
func (mb *MockBackend) Stop() error { return nil }

// ClearQueueMsgs clears our mock msg queue
func (mb *MockBackend) ClearQueueMsgs() {
	mb.queueMsgs = nil
}

// Health gives a string representing our health, empty for our mock
func (mb *MockBackend) Health() string {
	return ""
}

func buildMockBackend(config *config.Courier) Backend {
	return NewMockBackend()
}

func init() {
	RegisterBackend("mock", buildMockBackend)
}

//-----------------------------------------------------------------------------
// Mock channel implementation
//-----------------------------------------------------------------------------

type mockChannel struct {
	uuid        ChannelUUID
	channelType ChannelType
	address     string
	country     string
	config      map[string]interface{}
}

func (c *mockChannel) UUID() ChannelUUID        { return c.uuid }
func (c *mockChannel) ChannelType() ChannelType { return c.channelType }
func (c *mockChannel) Address() string          { return c.address }
func (c *mockChannel) Country() string          { return c.country }
func (c *mockChannel) ConfigForKey(key string, defaultValue interface{}) interface{} {
	value, found := c.config[key]
	if !found {
		return defaultValue
	}
	return value
}

func (c *mockChannel) StringConfigForKey(key string, defaultValue string) string {
	val := c.ConfigForKey(key, defaultValue)
	str, isStr := val.(string)
	if !isStr {
		return defaultValue
	}
	return str
}

// NewMockChannel creates a new mock channel for the passed in type, address, country and config
func NewMockChannel(uuid string, channelType string, address string, country string, config map[string]interface{}) Channel {
	cUUID, _ := NewChannelUUID(uuid)

	channel := &mockChannel{
		uuid:        cUUID,
		channelType: ChannelType(channelType),
		address:     address,
		country:     country,
		config:      config,
	}
	return channel
}
