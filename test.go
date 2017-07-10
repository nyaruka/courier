package courier

import (
	"errors"

	"time"

	_ "github.com/lib/pq" // postgres driver
	"github.com/nyaruka/courier/config"
)

//-----------------------------------------------------------------------------
// Mock backend implementation
//-----------------------------------------------------------------------------

// MockBackend is a mocked version of a backend which doesn't require a real database or cache
type MockBackend struct {
	channels     map[ChannelUUID]Channel
	queueMsgs    []Msg
	errorOnQueue bool
}

// NewMockBackend returns a new mock backend suitable for testing
func NewMockBackend() *MockBackend {
	return &MockBackend{channels: make(map[ChannelUUID]Channel)}
}

// GetLastQueueMsg returns the last message queued to the server
func (mb *MockBackend) GetLastQueueMsg() (Msg, error) {
	if len(mb.queueMsgs) == 0 {
		return nil, ErrMsgNotFound
	}
	return mb.queueMsgs[len(mb.queueMsgs)-1], nil
}

// NewIncomingMsg creates a new message from the given params
func (mb *MockBackend) NewIncomingMsg(channel Channel, urn URN, text string) Msg {
	return &mockMsg{channel: channel, urn: urn, text: text}
}

// NewOutgoingMsg creates a new outgoing message from the given params
func (mb *MockBackend) NewOutgoingMsg(channel Channel, urn URN, text string) Msg {
	return &mockMsg{channel: channel, urn: urn, text: text}
}

// PopNextOutgoingMsg returns the next message that should be sent, or nil if there are none to send
func (mb *MockBackend) PopNextOutgoingMsg() (Msg, error) {
	return nil, nil
}

// MarkOutgoingMsgComplete marks the passed msg as having been dealt with
func (mb *MockBackend) MarkOutgoingMsgComplete(m Msg) {
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
func (mb *MockBackend) WriteMsg(m Msg) error {
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

// MockChannel implements the Channel interface and is used in our tests
type MockChannel struct {
	uuid        ChannelUUID
	channelType ChannelType
	scheme      string
	address     string
	country     string
	config      map[string]interface{}
}

// UUID returns the uuid for this channel
func (c *MockChannel) UUID() ChannelUUID { return c.uuid }

// ChannelType returns the type of this channel
func (c *MockChannel) ChannelType() ChannelType { return c.channelType }

// Scheme returns the scheme of this channel
func (c *MockChannel) Scheme() string { return c.scheme }

// Address returns the address of this channel
func (c *MockChannel) Address() string { return c.address }

// Country returns the country this channel is for (if any)
func (c *MockChannel) Country() string { return c.country }

// SetConfig sets the passed in config parameter
func (c *MockChannel) SetConfig(key string, value interface{}) {
	c.config[key] = value
}

// ConfigForKey returns the config value for the passed in key
func (c *MockChannel) ConfigForKey(key string, defaultValue interface{}) interface{} {
	value, found := c.config[key]
	if !found {
		return defaultValue
	}
	return value
}

// StringConfigForKey returns the config value for the passed in key
func (c *MockChannel) StringConfigForKey(key string, defaultValue string) string {
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

	channel := &MockChannel{
		uuid:        cUUID,
		channelType: ChannelType(channelType),
		scheme:      TelScheme,
		address:     address,
		country:     country,
		config:      config,
	}
	return channel
}

//-----------------------------------------------------------------------------
// Mock msg implementation
//-----------------------------------------------------------------------------

type mockMsg struct {
	channel     Channel
	id          MsgID
	uuid        MsgUUID
	text        string
	attachments []string
	externalID  string
	urn         URN
	contactName string

	receivedOn *time.Time
	sentOn     *time.Time
	wiredOn    *time.Time
}

func (m *mockMsg) Channel() Channel      { return m.channel }
func (m *mockMsg) ID() MsgID             { return m.id }
func (m *mockMsg) UUID() MsgUUID         { return m.uuid }
func (m *mockMsg) Text() string          { return m.text }
func (m *mockMsg) Attachments() []string { return m.attachments }
func (m *mockMsg) ExternalID() string    { return m.externalID }
func (m *mockMsg) URN() URN              { return m.urn }
func (m *mockMsg) ContactName() string   { return m.contactName }

func (m *mockMsg) ReceivedOn() *time.Time { return m.receivedOn }
func (m *mockMsg) SentOn() *time.Time     { return m.sentOn }
func (m *mockMsg) WiredOn() *time.Time    { return m.wiredOn }

func (m *mockMsg) WithContactName(name string) Msg   { m.contactName = name; return m }
func (m *mockMsg) WithReceivedOn(date time.Time) Msg { m.receivedOn = &date; return m }
func (m *mockMsg) WithExternalID(id string) Msg      { m.externalID = id; return m }
func (m *mockMsg) WithID(id MsgID) Msg               { m.id = id; return m }
func (m *mockMsg) WithUUID(uuid MsgUUID) Msg         { m.uuid = uuid; return m }
func (m *mockMsg) WithAttachment(url string) Msg     { m.attachments = append(m.attachments, url); return m }
