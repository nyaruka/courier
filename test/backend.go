package test

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"

	"github.com/gomodule/redigo/redis"
	_ "github.com/lib/pq" // postgres driver
	"github.com/pkg/errors"
)

func init() {
	courier.RegisterBackend("mock", buildMockBackend)
}

// MockBackend is a mocked version of a backend which doesn't require a real database or cache
type MockBackend struct {
	channels          map[courier.ChannelUUID]courier.Channel
	channelsByAddress map[courier.ChannelAddress]courier.Channel
	contacts          map[urns.URN]courier.Contact
	media             map[string]courier.Media // url -> Media
	errorOnQueue      bool

	mutex     sync.RWMutex
	redisPool *redis.Pool
	lastMsgID courier.MsgID

	queueMsgs     []courier.Msg
	outgoingMsgs  []courier.Msg
	msgStatuses   []courier.MsgStatus
	channelEvents []courier.ChannelEvent
	channelLogs   []*courier.ChannelLog

	lastContactName string
	sentMsgs        map[courier.MsgID]bool
	seenExternalIDs []string
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

	return &MockBackend{
		channels:          make(map[courier.ChannelUUID]courier.Channel),
		channelsByAddress: make(map[courier.ChannelAddress]courier.Channel),
		contacts:          make(map[urns.URN]courier.Contact),
		media:             make(map[string]courier.Media),
		sentMsgs:          make(map[courier.MsgID]bool),
		redisPool:         redisPool,
	}
}

func (mb *MockBackend) ChannelLogs() []*courier.ChannelLog { return mb.channelLogs }
func (mb *MockBackend) MsgStatuses() []courier.MsgStatus   { return mb.msgStatuses }
func (mb *MockBackend) ClearMsgStatuses()                  { mb.msgStatuses = nil }

// GetLastQueueMsg returns the last message queued to the server
func (mb *MockBackend) GetLastQueueMsg() (courier.Msg, error) {
	if len(mb.queueMsgs) == 0 {
		return nil, courier.ErrMsgNotFound
	}
	return mb.queueMsgs[len(mb.queueMsgs)-1], nil
}

// GetLastChannelEvent returns the last event written to the server
func (mb *MockBackend) GetLastChannelEvent() (courier.ChannelEvent, error) {
	if len(mb.channelEvents) == 0 {
		return nil, errors.New("no channel events")
	}
	return mb.channelEvents[len(mb.channelEvents)-1], nil
}

// GetLastMsgStatus returns the last status written to the server
func (mb *MockBackend) GetLastMsgStatus() (courier.MsgStatus, error) {
	if len(mb.msgStatuses) == 0 {
		return nil, errors.New("no msg statuses")
	}
	return mb.msgStatuses[len(mb.msgStatuses)-1], nil
}

// GetLastContactName returns the contact name set on the last msg or channel event written
func (mb *MockBackend) GetLastContactName() string {
	return mb.lastContactName
}

// MockMedia adds the given media to the mocked backend
func (mb *MockBackend) MockMedia(media courier.Media) {
	mb.media[media.URL()] = media
}

// DeleteMsgWithExternalID delete a message we receive an event that it should be deleted
func (mb *MockBackend) DeleteMsgWithExternalID(ctx context.Context, channel courier.Channel, externalID string) error {
	return nil
}

// NewIncomingMsg creates a new message from the given params
func (mb *MockBackend) NewIncomingMsg(channel courier.Channel, urn urns.URN, text string, clog *courier.ChannelLog) courier.Msg {
	return &mockMsg{channel: channel, urn: urn, text: text}
}

// NewOutgoingMsg creates a new outgoing message from the given params
func (mb *MockBackend) NewOutgoingMsg(channel courier.Channel, id courier.MsgID, urn urns.URN, text string, highPriority bool, quickReplies []string, topic string, responseToExternalID string) courier.Msg {
	return &mockMsg{channel: channel, id: id, urn: urn, text: text, highPriority: highPriority, quickReplies: quickReplies, topic: topic, responseToExternalID: responseToExternalID}
}

// PushOutgoingMsg is a test method to add a message to our queue of messages to send
func (mb *MockBackend) PushOutgoingMsg(msg courier.Msg) {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()

	mb.outgoingMsgs = append(mb.outgoingMsgs, msg)
}

// PopNextOutgoingMsg returns the next message that should be sent, or nil if there are none to send
func (mb *MockBackend) PopNextOutgoingMsg(ctx context.Context) (courier.Msg, error) {
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
func (mb *MockBackend) WasMsgSent(ctx context.Context, id courier.MsgID) (bool, error) {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()

	return mb.sentMsgs[id], nil
}

func (mb *MockBackend) ClearMsgSent(ctx context.Context, id courier.MsgID) error {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()

	delete(mb.sentMsgs, id)
	return nil
}

// MarkOutgoingMsgComplete marks the passed msg as having been dealt with
func (mb *MockBackend) MarkOutgoingMsgComplete(ctx context.Context, msg courier.Msg, s courier.MsgStatus) {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()

	mb.sentMsgs[msg.ID()] = true
}

// WriteChannelLog writes the passed in channel log to the DB
func (mb *MockBackend) WriteChannelLog(ctx context.Context, clog *courier.ChannelLog) error {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()

	mb.channelLogs = append(mb.channelLogs, clog)
	return nil
}

// SetErrorOnQueue is a mock method which makes the QueueMsg call throw the passed in error on next call
func (mb *MockBackend) SetErrorOnQueue(shouldError bool) {
	mb.errorOnQueue = shouldError
}

// WriteMsg queues the passed in message internally
func (mb *MockBackend) WriteMsg(ctx context.Context, m courier.Msg, clog *courier.ChannelLog) error {
	mock := m.(*mockMsg)

	// this msg has already been written (we received it twice), we are a no op
	if mock.alreadyWritten {
		return nil
	}

	mb.lastMsgID++
	mock.id = mb.lastMsgID

	if mb.errorOnQueue {
		return errors.New("unable to queue message")
	}

	mb.queueMsgs = append(mb.queueMsgs, m)
	mb.lastContactName = m.(*mockMsg).contactName
	return nil
}

// NewMsgStatusForID creates a new Status object for the given message id
func (mb *MockBackend) NewMsgStatusForID(channel courier.Channel, id courier.MsgID, status courier.MsgStatusValue, clog *courier.ChannelLog) courier.MsgStatus {
	return &mockMsgStatus{
		channel:   channel,
		id:        id,
		status:    status,
		createdOn: time.Now().In(time.UTC),
	}
}

// NewMsgStatusForExternalID creates a new Status object for the given external id
func (mb *MockBackend) NewMsgStatusForExternalID(channel courier.Channel, externalID string, status courier.MsgStatusValue, clog *courier.ChannelLog) courier.MsgStatus {
	return &mockMsgStatus{
		channel:    channel,
		externalID: externalID,
		status:     status,
		createdOn:  time.Now().In(time.UTC),
	}
}

// WriteMsgStatus writes the status update to our queue
func (mb *MockBackend) WriteMsgStatus(ctx context.Context, status courier.MsgStatus) error {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()

	mb.msgStatuses = append(mb.msgStatuses, status)
	return nil
}

// NewChannelEvent creates a new channel event with the passed in parameters
func (mb *MockBackend) NewChannelEvent(channel courier.Channel, eventType courier.ChannelEventType, urn urns.URN, clog *courier.ChannelLog) courier.ChannelEvent {
	return &mockChannelEvent{
		channel:   channel,
		eventType: eventType,
		urn:       urn,
	}
}

// WriteChannelEvent writes the channel event passed in
func (mb *MockBackend) WriteChannelEvent(ctx context.Context, event courier.ChannelEvent, clog *courier.ChannelLog) error {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()

	mb.channelEvents = append(mb.channelEvents, event)
	mb.lastContactName = event.(*mockChannelEvent).contactName
	return nil
}

// GetChannel returns the channel with the passed in type and channel uuid
func (mb *MockBackend) GetChannel(ctx context.Context, cType courier.ChannelType, uuid courier.ChannelUUID) (courier.Channel, error) {
	channel, found := mb.channels[uuid]
	if !found {
		return nil, courier.ErrChannelNotFound
	}
	return channel, nil
}

// GetChannelByAddress returns the channel with the passed in type and channel address
func (mb *MockBackend) GetChannelByAddress(ctx context.Context, cType courier.ChannelType, address courier.ChannelAddress) (courier.Channel, error) {
	channel, found := mb.channelsByAddress[address]
	if !found {
		return nil, courier.ErrChannelNotFound
	}
	return channel, nil
}

// GetContact creates a new contact with the passed in channel and URN
func (mb *MockBackend) GetContact(ctx context.Context, channel courier.Channel, urn urns.URN, auth, name string, clog *courier.ChannelLog) (courier.Contact, error) {
	contact, found := mb.contacts[urn]
	if !found {
		uuid, _ := courier.NewContactUUID(string(uuids.New()))
		contact = &mockContact{channel, urn, auth, uuid}
		mb.contacts[urn] = contact
	}
	return contact, nil
}

// AddURNtoContact adds a URN to the passed in contact
func (mb *MockBackend) AddURNtoContact(context context.Context, channel courier.Channel, contact courier.Contact, urn urns.URN) (urns.URN, error) {
	mb.contacts[urn] = contact
	return urn, nil
}

// RemoveURNFromcontact removes a URN from the passed in contact
func (mb *MockBackend) RemoveURNfromContact(context context.Context, channel courier.Channel, contact courier.Contact, urn urns.URN) (urns.URN, error) {
	contact, found := mb.contacts[urn]
	if found {
		delete(mb.contacts, urn)
	}
	return urn, nil
}

// AddChannel adds a test channel to the test server
func (mb *MockBackend) AddChannel(channel courier.Channel) {
	mb.channels[channel.UUID()] = channel
	mb.channelsByAddress[channel.ChannelAddress()] = channel
}

// ClearChannels is a utility function on our mock server to clear all added channels
func (mb *MockBackend) ClearChannels() {
	mb.channels = nil
	mb.channelsByAddress = nil
}

// Start starts our mock backend
func (mb *MockBackend) Start() error { return nil }

// Stop stops our mock backend
func (mb *MockBackend) Stop() error { return nil }

// Cleanup cleans up any connections that are open
func (mb *MockBackend) Cleanup() error { return nil }

// Reset clears our queued messages, seen external IDs, and channel logs
func (mb *MockBackend) Reset() {
	mb.lastMsgID = courier.NilMsgID
	mb.queueMsgs = nil
	mb.seenExternalIDs = nil
	mb.channelLogs = nil
}

// LenQueuedMsgs Get the length of queued msgs
func (mb *MockBackend) LenQueuedMsgs() int {
	return len(mb.queueMsgs)
}

// CheckExternalIDSeen checks if external ID has been seen in a period
func (mb *MockBackend) CheckExternalIDSeen(msg courier.Msg) courier.Msg {
	m := msg.(*mockMsg)

	for _, b := range mb.seenExternalIDs {
		if b == msg.ExternalID() {
			m.alreadyWritten = true
			return m
		}
	}
	return m
}

// WriteExternalIDSeen marks a external ID as seen for a period
func (mb *MockBackend) WriteExternalIDSeen(msg courier.Msg) {
	mb.seenExternalIDs = append(mb.seenExternalIDs, msg.ExternalID())
}

// ResolveMedia resolves the passed in media URL to a media object
func (mb *MockBackend) ResolveMedia(ctx context.Context, mediaUrl string) (courier.Media, error) {
	media := mb.media[mediaUrl]
	if media == nil {
		return nil, nil
	}

	return media, nil
}

// Health gives a string representing our health, empty for our mock
func (mb *MockBackend) Health() string {
	return ""
}

// Status returns a string describing the status of the service, queue size etc..
func (mb *MockBackend) Status() string {
	return ""
}

// Heartbeat is a noop for our mock backend
func (mb *MockBackend) Heartbeat() error {
	return nil
}

// RedisPool returns the redisPool for this backend
func (mb *MockBackend) RedisPool() *redis.Pool {
	return mb.redisPool
}

func buildMockBackend(config *courier.Config) courier.Backend {
	return NewMockBackend()
}
