package courier

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	"time"

	"github.com/garyburd/redigo/redis"
	_ "github.com/lib/pq" // postgres driver
	"github.com/nyaruka/courier/config"
	"github.com/nyaruka/gocommon/urns"
)

//-----------------------------------------------------------------------------
// Mock backend implementation
//-----------------------------------------------------------------------------

// MockBackend is a mocked version of a backend which doesn't require a real database or cache
type MockBackend struct {
	channels     map[ChannelUUID]Channel
	queueMsgs    []Msg
	errorOnQueue bool

	mutex           sync.RWMutex
	outgoingMsgs    []Msg
	msgStatuses     []MsgStatus
	channelEvents   []ChannelEvent
	lastContactName string

	stoppedMsgContacts []Msg
	sentMsgs           map[MsgID]bool
	redisPool          *redis.Pool
}

// NewMockBackend returns a new mock backend suitable for testing
func NewMockBackend() *MockBackend {
	redisPool := &redis.Pool{
		Wait:        true,              // makes callers wait for a connection
		MaxActive:   5,                 // only open this many concurrent connections at once
		MaxIdle:     2,                 // only keep up to 2 idle
		IdleTimeout: 240 * time.Second, // how long to wait before reaping a connection
		Dial: func() (redis.Conn, error) {
			conn, err := redis.Dial("tcp", "localhost:6379")
			if err != nil {
				return nil, err
			}
			_, err = conn.Do("SELECT", 0)
			return conn, err
		},
	}
	conn := redisPool.Get()
	defer conn.Close()

	_, err := conn.Do("FLUSHDB")
	if err != nil {
		log.Fatal(err)
	}

	_, err = conn.Do("Set", "jiochat_channel_access_token:8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ACCESS_TOKEN")
	if err != nil {
		log.Fatal(err)
	}

	return &MockBackend{
		channels:  make(map[ChannelUUID]Channel),
		sentMsgs:  make(map[MsgID]bool),
		redisPool: redisPool,
	}
}

// GetLastQueueMsg returns the last message queued to the server
func (mb *MockBackend) GetLastQueueMsg() (Msg, error) {
	if len(mb.queueMsgs) == 0 {
		return nil, ErrMsgNotFound
	}
	return mb.queueMsgs[len(mb.queueMsgs)-1], nil
}

// GetLastChannelEvent returns the last event written to the server
func (mb *MockBackend) GetLastChannelEvent() (ChannelEvent, error) {
	if len(mb.channelEvents) == 0 {
		return nil, errors.New("no channel events")
	}
	return mb.channelEvents[len(mb.channelEvents)-1], nil
}

// GetLastMsgStatus returns the last status written to the server
func (mb *MockBackend) GetLastMsgStatus() (MsgStatus, error) {
	if len(mb.msgStatuses) == 0 {
		return nil, errors.New("no msg statuses")
	}
	return mb.msgStatuses[len(mb.msgStatuses)-1], nil
}

// GetLastContactName returns the contact name set on the last msg or channel event written
func (mb *MockBackend) GetLastContactName() string {
	return mb.lastContactName
}

// NewIncomingMsg creates a new message from the given params
func (mb *MockBackend) NewIncomingMsg(channel Channel, urn urns.URN, text string) Msg {
	return &mockMsg{channel: channel, urn: urn, text: text}
}

// NewOutgoingMsg creates a new outgoing message from the given params
func (mb *MockBackend) NewOutgoingMsg(channel Channel, id MsgID, urn urns.URN, text string, highPriority bool, replies []string) Msg {
	return &mockMsg{channel: channel, id: id, urn: urn, text: text, highPriority: highPriority, quickReplies: replies}
}

// PushOutgoingMsg is a test method to add a message to our queue of messages to send
func (mb *MockBackend) PushOutgoingMsg(msg Msg) {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()

	mb.outgoingMsgs = append(mb.outgoingMsgs, msg)
}

// PopNextOutgoingMsg returns the next message that should be sent, or nil if there are none to send
func (mb *MockBackend) PopNextOutgoingMsg(ctx context.Context) (Msg, error) {
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
func (mb *MockBackend) WasMsgSent(ctx context.Context, msg Msg) (bool, error) {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()

	return mb.sentMsgs[msg.ID()], nil
}

// StopMsgContact stops the contact for the passed in msg
func (mb *MockBackend) StopMsgContact(ctx context.Context, msg Msg) {
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
func (mb *MockBackend) MarkOutgoingMsgComplete(ctx context.Context, msg Msg, s MsgStatus) {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()

	mb.sentMsgs[msg.ID()] = true
}

// WriteChannelLogs writes the passed in channel logs to the DB
func (mb *MockBackend) WriteChannelLogs(ctx context.Context, logs []*ChannelLog) error {
	return nil
}

// SetErrorOnQueue is a mock method which makes the QueueMsg call throw the passed in error on next call
func (mb *MockBackend) SetErrorOnQueue(shouldError bool) {
	mb.errorOnQueue = shouldError
}

// WriteMsg queues the passed in message internally
func (mb *MockBackend) WriteMsg(ctx context.Context, m Msg) error {
	if mb.errorOnQueue {
		return errors.New("unable to queue message")
	}

	mb.queueMsgs = append(mb.queueMsgs, m)
	mb.lastContactName = m.(*mockMsg).contactName
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
func (mb *MockBackend) WriteMsgStatus(ctx context.Context, status MsgStatus) error {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()

	mb.msgStatuses = append(mb.msgStatuses, status)
	return nil
}

// NewChannelEvent creates a new channel event with the passed in parameters
func (mb *MockBackend) NewChannelEvent(channel Channel, eventType ChannelEventType, urn urns.URN) ChannelEvent {
	return &mockChannelEvent{
		channel:   channel,
		eventType: eventType,
		urn:       urn,
	}
}

// WriteChannelEvent writes the channel event passed in
func (mb *MockBackend) WriteChannelEvent(ctx context.Context, event ChannelEvent) error {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()

	mb.channelEvents = append(mb.channelEvents, event)
	mb.lastContactName = event.(*mockChannelEvent).contactName
	return nil
}

// GetChannel returns the channel with the passed in type and channel uuid
func (mb *MockBackend) GetChannel(ctx context.Context, cType ChannelType, uuid ChannelUUID) (Channel, error) {
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

// Cleanup cleans up any connections that are open
func (mb *MockBackend) Cleanup() error { return nil }

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

// RedisPool returns the redisPool for this backend
func (mb *MockBackend) RedisPool() *redis.Pool {
	return mb.redisPool
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
	orgConfig   map[string]interface{}
}

// UUID returns the uuid for this channel
func (c *MockChannel) UUID() ChannelUUID { return c.uuid }

// Name returns the name of this channel, we just return our UUID for our mock instances
func (c *MockChannel) Name() string { return fmt.Sprintf("Channel: %s", c.uuid.String()) }

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

// CallbackDomain returns the callback domain to use for this channel
func (c *MockChannel) CallbackDomain(fallbackDomain string) string {
	value, found := c.config[ConfigCallbackDomain]
	if !found {
		return fallbackDomain
	}
	return value.(string)
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

// OrgConfigForKey returns the org config value for the passed in key
func (c *MockChannel) OrgConfigForKey(key string, defaultValue interface{}) interface{} {
	value, found := c.orgConfig[key]
	if !found {
		return defaultValue
	}
	return value
}

// NewMockChannel creates a new mock channel for the passed in type, address, country and config
func NewMockChannel(uuid string, channelType string, address string, country string, config map[string]interface{}) Channel {
	cUUID, _ := NewChannelUUID(uuid)

	channel := &MockChannel{
		uuid:        cUUID,
		channelType: ChannelType(channelType),
		schemes:     []string{urns.TelScheme},
		address:     address,
		country:     country,
		config:      config,
		orgConfig:   map[string]interface{}{},
	}
	return channel
}

//-----------------------------------------------------------------------------
// Mock msg implementation
//-----------------------------------------------------------------------------

type mockMsg struct {
	channel      Channel
	id           MsgID
	uuid         MsgUUID
	text         string
	attachments  []string
	externalID   string
	urn          urns.URN
	contactName  string
	highPriority bool
	quickReplies []string
	responseToID MsgID

	receivedOn *time.Time
	sentOn     *time.Time
	wiredOn    *time.Time
}

func (m *mockMsg) Channel() Channel       { return m.channel }
func (m *mockMsg) ID() MsgID              { return m.id }
func (m *mockMsg) EventID() int64         { return m.id.Int64 }
func (m *mockMsg) UUID() MsgUUID          { return m.uuid }
func (m *mockMsg) Text() string           { return m.text }
func (m *mockMsg) Attachments() []string  { return m.attachments }
func (m *mockMsg) ExternalID() string     { return m.externalID }
func (m *mockMsg) URN() urns.URN          { return m.urn }
func (m *mockMsg) ContactName() string    { return m.contactName }
func (m *mockMsg) HighPriority() bool     { return m.highPriority }
func (m *mockMsg) QuickReplies() []string { return m.quickReplies }
func (m *mockMsg) ResponseToID() MsgID    { return m.responseToID }

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
func (m *mockMsgStatus) EventID() int64           { return m.id.Int64 }

func (m *mockMsgStatus) ExternalID() string      { return m.externalID }
func (m *mockMsgStatus) SetExternalID(id string) { m.externalID = id }

func (m *mockMsgStatus) Status() MsgStatusValue          { return m.status }
func (m *mockMsgStatus) SetStatus(status MsgStatusValue) { m.status = status }

func (m *mockMsgStatus) Logs() []*ChannelLog    { return m.logs }
func (m *mockMsgStatus) AddLog(log *ChannelLog) { m.logs = append(m.logs, log) }

//-----------------------------------------------------------------------------
// Mock channel event implementation
//-----------------------------------------------------------------------------

type mockChannelEvent struct {
	channel    Channel
	eventType  ChannelEventType
	urn        urns.URN
	createdOn  time.Time
	occurredOn time.Time

	contactName string
	extra       map[string]interface{}

	logs []*ChannelLog
}

func (e *mockChannelEvent) EventID() int64                { return 0 }
func (e *mockChannelEvent) ChannelUUID() ChannelUUID      { return e.channel.UUID() }
func (e *mockChannelEvent) EventType() ChannelEventType   { return e.eventType }
func (e *mockChannelEvent) CreatedOn() time.Time          { return e.createdOn }
func (e *mockChannelEvent) OccurredOn() time.Time         { return e.occurredOn }
func (e *mockChannelEvent) Extra() map[string]interface{} { return e.extra }
func (e *mockChannelEvent) ContactName() string           { return e.contactName }
func (e *mockChannelEvent) URN() urns.URN                 { return e.urn }

func (e *mockChannelEvent) WithExtra(extra map[string]interface{}) ChannelEvent {
	e.extra = extra
	return e
}
func (e *mockChannelEvent) WithContactName(name string) ChannelEvent {
	e.contactName = name
	return e
}
func (e *mockChannelEvent) WithOccurredOn(time time.Time) ChannelEvent {
	e.occurredOn = time
	return e
}

func (e *mockChannelEvent) Logs() []*ChannelLog    { return e.logs }
func (e *mockChannelEvent) AddLog(log *ChannelLog) { e.logs = append(e.logs, log) }
