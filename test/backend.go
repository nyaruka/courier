package test

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gomodule/redigo/redis"
	_ "github.com/lib/pq"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/pkg/errors"
)

func init() {
	courier.RegisterBackend("mock", buildMockBackend)
}

func buildMockBackend(config *courier.Config) courier.Backend {
	return NewMockBackend()
}

type SavedAttachment struct {
	Channel     courier.Channel
	ContentType string
	Data        []byte
	Extension   string
}

// MockBackend is a mocked version of a backend which doesn't require a real database or cache
type MockBackend struct {
	channels          map[courier.ChannelUUID]courier.Channel
	channelsByAddress map[courier.ChannelAddress]courier.Channel
	contacts          map[urns.URN]courier.Contact
	outgoingMsgs      []courier.MsgOut
	media             map[string]courier.Media // url -> Media
	errorOnQueue      bool

	mutex     sync.RWMutex
	redisPool *redis.Pool

	writtenMsgs          []courier.MsgIn
	writtenMsgStatuses   []courier.StatusUpdate
	writtenChannelEvents []courier.ChannelEvent
	writtenChannelLogs   []*courier.ChannelLog
	savedAttachments     []*SavedAttachment
	storageError         error

	lastMsgID       courier.MsgID
	lastContactName string
	urnAuthTokens   map[urns.URN]map[string]string
	sentMsgs        map[courier.MsgID]bool
	seenExternalIDs map[string]courier.MsgUUID
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
		seenExternalIDs:   make(map[string]courier.MsgUUID),
		redisPool:         redisPool,
	}
}

// DeleteMsgByExternalID delete a message we receive an event that it should be deleted
func (mb *MockBackend) DeleteMsgByExternalID(ctx context.Context, channel courier.Channel, externalID string) error {
	return nil
}

// NewIncomingMsg creates a new message from the given params
func (mb *MockBackend) NewIncomingMsg(channel courier.Channel, urn urns.URN, text string, extID string, clog *courier.ChannelLog) courier.MsgIn {
	m := &MockMsg{
		channel: channel, urn: urn, text: text, externalID: extID,
	}

	uuid := mb.seenExternalIDs[fmt.Sprintf("%s|%s", m.Channel().UUID(), m.ExternalID())]
	if uuid != "" {
		m.uuid = uuid
		m.alreadyWritten = true
	}

	return m
}

// NewOutgoingMsg creates a new outgoing message from the given params
func (mb *MockBackend) NewOutgoingMsg(channel courier.Channel, id courier.MsgID, urn urns.URN, text string, highPriority bool, quickReplies []string,
	topic string, responseToExternalID string, origin courier.MsgOrigin, contactLastSeenOn *time.Time) courier.MsgOut {

	return &MockMsg{
		channel:              channel,
		id:                   id,
		urn:                  urn,
		text:                 text,
		highPriority:         highPriority,
		quickReplies:         quickReplies,
		topic:                topic,
		responseToExternalID: responseToExternalID,
		origin:               origin,
		contactLastSeenOn:    contactLastSeenOn,
	}
}

// PushOutgoingMsg is a test method to add a message to our queue of messages to send
func (mb *MockBackend) PushOutgoingMsg(msg courier.MsgOut) {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()

	mb.outgoingMsgs = append(mb.outgoingMsgs, msg)
}

// PopNextOutgoingMsg returns the next message that should be sent, or nil if there are none to send
func (mb *MockBackend) PopNextOutgoingMsg(ctx context.Context) (courier.MsgOut, error) {
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
func (mb *MockBackend) MarkOutgoingMsgComplete(ctx context.Context, msg courier.MsgOut, s courier.StatusUpdate) {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()

	mb.sentMsgs[msg.ID()] = true
}

// WriteChannelLog writes the passed in channel log to the DB
func (mb *MockBackend) WriteChannelLog(ctx context.Context, clog *courier.ChannelLog) error {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()

	mb.writtenChannelLogs = append(mb.writtenChannelLogs, clog)
	return nil
}

// SetErrorOnQueue is a mock method which makes the QueueMsg call throw the passed in error on next call
func (mb *MockBackend) SetErrorOnQueue(shouldError bool) {
	mb.errorOnQueue = shouldError
}

// WriteMsg queues the passed in message internally
func (mb *MockBackend) WriteMsg(ctx context.Context, m courier.MsgIn, clog *courier.ChannelLog) error {
	mm := m.(*MockMsg)

	// this msg has already been written (we received it twice), we are a no op
	if mm.alreadyWritten {
		return nil
	}

	mb.lastMsgID++
	mm.id = mb.lastMsgID

	if mb.errorOnQueue {
		return errors.New("unable to queue message")
	}

	mb.writtenMsgs = append(mb.writtenMsgs, m)
	mb.lastContactName = mm.contactName

	if mm.urnAuthTokens != nil {
		mb.recordURNAuthTokens(mm.urn, mm.urnAuthTokens)
	}

	if m.ExternalID() != "" {
		mb.seenExternalIDs[fmt.Sprintf("%s|%s", m.Channel().UUID(), m.ExternalID())] = m.UUID()
	}

	return nil
}

// NewStatusUpdate creates a new Status object for the given message id
func (mb *MockBackend) NewStatusUpdate(channel courier.Channel, id courier.MsgID, status courier.MsgStatus, clog *courier.ChannelLog) courier.StatusUpdate {
	return &MockStatusUpdate{
		channel:   channel,
		msgID:     id,
		status:    status,
		createdOn: time.Now().In(time.UTC),
	}
}

// NewStatusUpdateByExternalID creates a new Status object for the given external id
func (mb *MockBackend) NewStatusUpdateByExternalID(channel courier.Channel, externalID string, status courier.MsgStatus, clog *courier.ChannelLog) courier.StatusUpdate {
	return &MockStatusUpdate{
		channel:    channel,
		externalID: externalID,
		status:     status,
		createdOn:  time.Now().In(time.UTC),
	}
}

// WriteStatusUpdate writes the status update to our queue
func (mb *MockBackend) WriteStatusUpdate(ctx context.Context, status courier.StatusUpdate) error {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()

	mb.writtenMsgStatuses = append(mb.writtenMsgStatuses, status)
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
	evt := event.(*mockChannelEvent)

	mb.mutex.Lock()
	defer mb.mutex.Unlock()

	mb.writtenChannelEvents = append(mb.writtenChannelEvents, event)
	mb.lastContactName = evt.contactName

	if evt.urnAuthTokens != nil {
		mb.recordURNAuthTokens(evt.urn, evt.urnAuthTokens)
	}

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
func (mb *MockBackend) GetContact(ctx context.Context, channel courier.Channel, urn urns.URN, authTokens map[string]string, name string, clog *courier.ChannelLog) (courier.Contact, error) {
	contact, found := mb.contacts[urn]
	if !found {
		contact = &mockContact{channel, urn, authTokens, courier.ContactUUID(uuids.New())}
		mb.contacts[urn] = contact
	}
	return contact, nil
}

// AddURNtoContact adds a URN to the passed in contact
func (mb *MockBackend) AddURNtoContact(context context.Context, channel courier.Channel, contact courier.Contact, urn urns.URN, authTokens map[string]string) (urns.URN, error) {
	mb.contacts[urn] = contact
	return urn, nil
}

// RemoveURNFromcontact removes a URN from the passed in contact
func (mb *MockBackend) RemoveURNfromContact(context context.Context, channel courier.Channel, contact courier.Contact, urn urns.URN) (urns.URN, error) {
	_, found := mb.contacts[urn]
	if found {
		delete(mb.contacts, urn)
	}
	return urn, nil
}

// Start starts our mock backend
func (mb *MockBackend) Start() error { return nil }

// Stop stops our mock backend
func (mb *MockBackend) Stop() error { return nil }

// Cleanup cleans up any connections that are open
func (mb *MockBackend) Cleanup() error { return nil }

// SaveAttachment saves an attachment to backend storage
func (mb *MockBackend) SaveAttachment(ctx context.Context, ch courier.Channel, contentType string, data []byte, extension string) (string, error) {
	if mb.storageError != nil {
		return "", mb.storageError
	}

	mb.savedAttachments = append(mb.savedAttachments, &SavedAttachment{
		Channel: ch, ContentType: contentType, Data: data, Extension: extension,
	})

	time.Sleep(time.Millisecond * 2)

	return fmt.Sprintf("https://backend.com/attachments/%s.%s", uuids.New(), extension), nil
}

// ResolveMedia resolves the passed in media URL to a media object
func (mb *MockBackend) ResolveMedia(ctx context.Context, mediaUrl string) (courier.Media, error) {
	media := mb.media[mediaUrl]
	if media == nil {
		return nil, nil
	}

	return media, nil
}

func (mb *MockBackend) Health() string {
	return ""
}

// Health gives a string representing our health, empty for our mock
func (mb *MockBackend) HttpClient(bool) *http.Client {
	return http.DefaultClient
}

func (mb *MockBackend) HttpAccess() *httpx.AccessConfig {
	return nil
}

// Status returns a string describing the status of the service, queue size etc..
func (mb *MockBackend) Status() string {
	return "ALL GOOD"
}

// Heartbeat is a noop for our mock backend
func (mb *MockBackend) Heartbeat() error {
	return nil
}

// RedisPool returns the redisPool for this backend
func (mb *MockBackend) RedisPool() *redis.Pool {
	return mb.redisPool
}

////////////////////////////////////////////////////////////////////////////////
// Methods not part of the backed interface but used in tests
////////////////////////////////////////////////////////////////////////////////

func (mb *MockBackend) WrittenMsgs() []courier.MsgIn                  { return mb.writtenMsgs }
func (mb *MockBackend) WrittenMsgStatuses() []courier.StatusUpdate    { return mb.writtenMsgStatuses }
func (mb *MockBackend) WrittenChannelEvents() []courier.ChannelEvent  { return mb.writtenChannelEvents }
func (mb *MockBackend) WrittenChannelLogs() []*courier.ChannelLog     { return mb.writtenChannelLogs }
func (mb *MockBackend) SavedAttachments() []*SavedAttachment          { return mb.savedAttachments }
func (mb *MockBackend) URNAuthTokens() map[urns.URN]map[string]string { return mb.urnAuthTokens }

// LastContactName returns the contact name set on the last msg or channel event written
func (mb *MockBackend) LastContactName() string {
	return mb.lastContactName
}

// MockMedia adds the given media to the mocked backend
func (mb *MockBackend) MockMedia(media courier.Media) {
	mb.media[media.URL()] = media
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

// Reset clears our queued messages, seen external IDs, and channel logs
func (mb *MockBackend) Reset() {
	mb.lastMsgID = courier.NilMsgID
	mb.seenExternalIDs = make(map[string]courier.MsgUUID)

	mb.writtenMsgs = nil
	mb.writtenMsgStatuses = nil
	mb.writtenChannelEvents = nil
	mb.writtenChannelLogs = nil
	mb.urnAuthTokens = nil
}

// SetStorageError sets the error to return for operation that try to use storage
func (mb *MockBackend) SetStorageError(err error) {
	mb.storageError = err
}

func (mb *MockBackend) recordURNAuthTokens(urn urns.URN, authTokens map[string]string) {
	if mb.urnAuthTokens == nil {
		mb.urnAuthTokens = make(map[urns.URN]map[string]string)
	}
	if mb.urnAuthTokens[urn] == nil {
		mb.urnAuthTokens[urn] = map[string]string{}
	}
	utils.MapUpdate(mb.urnAuthTokens[urn], authTokens)
}
