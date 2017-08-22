package courier

import (
	"errors"
	"sync"

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

	mutex        sync.RWMutex
	outgoingMsgs []Msg
	msgStatuses  []MsgStatus

	stoppedMsgContacts []Msg
	sentMsgs           map[MsgID]bool
}

// NewMockBackend returns a new mock backend suitable for testing
func NewMockBackend() *MockBackend {
	return &MockBackend{
		channels: make(map[ChannelUUID]Channel),
		sentMsgs: make(map[MsgID]bool),
	}
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
	return &mockMsg{channel: channel, urn: urn, text: text, priority: DefaultPriority}
}

// NewOutgoingMsg creates a new outgoing message from the given params
func (mb *MockBackend) NewOutgoingMsg(channel Channel, urn URN, text string, priority MsgPriority) Msg {
	return &mockMsg{channel: channel, urn: urn, text: text, priority: priority}
}

// PushOutgoingMsg is a test method to add a message to our queue of messages to send
func (mb *MockBackend) PushOutgoingMsg(msg Msg) {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()

	mb.outgoingMsgs = append(mb.outgoingMsgs, msg)
}

// PopNextOutgoingMsg returns the next message that should be sent, or nil if there are none to send
func (mb *MockBackend) PopNextOutgoingMsg() (Msg, error) {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()

	if len(mb.outgoingMsgs) > 0 {
		msg, rest := mb.outgoingMsgs[0], mb.outgoingMsgs[1:]
		mb.outgoingMsgs = rest
		return msg, nil
	}

	return nil, nil
}

// WasMsgSent returns whether the passed in msg was already sent
func (mb *MockBackend) WasMsgSent(msg Msg) (bool, error) {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()

	return mb.sentMsgs[msg.ID()], nil
}

// StopMsgContact stops the contact for the passed in msg
func (mb *MockBackend) StopMsgContact(msg Msg) {
	mb.stoppedMsgContacts = append(mb.stoppedMsgContacts, msg)
}

// GetLastStoppedMsgContact returns the last msg contact
func (mb *MockBackend) GetLastStoppedMsgContact() Msg {
	if len(mb.stoppedMsgContacts) > 0 {
		return mb.stoppedMsgContacts[len(mb.stoppedMsgContacts)-1]
	}
	return nil
}

// MarkOutgoingMsgComplete marks the passed msg as having been dealt with
func (mb *MockBackend) MarkOutgoingMsgComplete(msg Msg, s MsgStatus) {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()

	mb.sentMsgs[msg.ID()] = true
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

// NewMsgStatusForID creates a new Status object for the given message id
func (mb *MockBackend) NewMsgStatusForID(channel Channel, id MsgID, status MsgStatusValue) MsgStatus {
	return &mockMsgStatus{
		channel:   channel,
		id:        id,
		status:    status,
		createdOn: time.Now().In(time.UTC),
	}
}

// NewMsgStatusForExternalID creates a new Status object for the given external id
func (mb *MockBackend) NewMsgStatusForExternalID(channel Channel, externalID string, status MsgStatusValue) MsgStatus {
	return &mockMsgStatus{
		channel:    channel,
		externalID: externalID,
		status:     status,
		createdOn:  time.Now().In(time.UTC),
	}
}

// WriteMsgStatus writes the status update to our queue
func (mb *MockBackend) WriteMsgStatus(status MsgStatus) error {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()

	mb.msgStatuses = append(mb.msgStatuses, status)
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

// Status returns a string describing the status of the service, queue size etc..
func (mb *MockBackend) Status() string {
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
	schemes     []string
	address     string
	country     string
	config      map[string]interface{}
}

// UUID returns the uuid for this channel
func (c *MockChannel) UUID() ChannelUUID { return c.uuid }

// ChannelType returns the type of this channel
func (c *MockChannel) ChannelType() ChannelType { return c.channelType }

// Schemes returns the schemes for this channel
func (c *MockChannel) Schemes() []string { return c.schemes }

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
		schemes:     []string{TelScheme},
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
	priority    MsgPriority

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
func (m *mockMsg) Priority() MsgPriority { return m.priority }

func (m *mockMsg) ReceivedOn() *time.Time { return m.receivedOn }
func (m *mockMsg) SentOn() *time.Time     { return m.sentOn }
func (m *mockMsg) WiredOn() *time.Time    { return m.wiredOn }

func (m *mockMsg) WithContactName(name string) Msg   { m.contactName = name; return m }
func (m *mockMsg) WithReceivedOn(date time.Time) Msg { m.receivedOn = &date; return m }
func (m *mockMsg) WithExternalID(id string) Msg      { m.externalID = id; return m }
func (m *mockMsg) WithID(id MsgID) Msg               { m.id = id; return m }
func (m *mockMsg) WithUUID(uuid MsgUUID) Msg         { m.uuid = uuid; return m }
func (m *mockMsg) WithAttachment(url string) Msg     { m.attachments = append(m.attachments, url); return m }

//-----------------------------------------------------------------------------
// Mock status implementation
//-----------------------------------------------------------------------------

type mockMsgStatus struct {
	channel    Channel
	id         MsgID
	externalID string
	status     MsgStatusValue
	createdOn  time.Time

	logs []*ChannelLog
}

func (m *mockMsgStatus) ChannelUUID() ChannelUUID { return m.channel.UUID() }
func (m *mockMsgStatus) ID() MsgID                { return m.id }

func (m *mockMsgStatus) ExternalID() string      { return m.externalID }
func (m *mockMsgStatus) SetExternalID(id string) { m.externalID = id }

func (m *mockMsgStatus) Status() MsgStatusValue          { return m.status }
func (m *mockMsgStatus) SetStatus(status MsgStatusValue) { m.status = status }

func (m *mockMsgStatus) Logs() []*ChannelLog    { return m.logs }
func (m *mockMsgStatus) AddLog(log *ChannelLog) { m.logs = append(m.logs, log) }
