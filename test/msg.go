package test

import (
	"encoding/json"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/urns"
)

type MockMsg struct {
	id                   courier.MsgID
	uuid                 courier.MsgUUID
	channel              courier.Channel
	urn                  urns.URN
	urnAuth              string
	urnAuthTokens        map[string]string
	text                 string
	attachments          []string
	locale               i18n.Locale
	externalID           string
	contactName          string
	highPriority         bool
	quickReplies         []string
	origin               courier.MsgOrigin
	contactLastSeenOn    *time.Time
	topic                string
	responseToExternalID string
	metadata             json.RawMessage
	alreadyWritten       bool
	isResend             bool

	flow  *courier.FlowReference
	optIn *courier.OptInReference

	receivedOn *time.Time
	sentOn     *time.Time
}

func NewMockMsg(id courier.MsgID, uuid courier.MsgUUID, channel courier.Channel, urn urns.URN, text string, attachments []string) *MockMsg {
	return &MockMsg{
		id:          id,
		uuid:        uuid,
		channel:     channel,
		urn:         urn,
		text:        text,
		attachments: attachments,
	}
}

func (m *MockMsg) EventID() int64           { return int64(m.id) }
func (m *MockMsg) ID() courier.MsgID        { return m.id }
func (m *MockMsg) UUID() courier.MsgUUID    { return m.uuid }
func (m *MockMsg) ExternalID() string       { return m.externalID }
func (m *MockMsg) Text() string             { return m.text }
func (m *MockMsg) Attachments() []string    { return m.attachments }
func (m *MockMsg) URN() urns.URN            { return m.urn }
func (m *MockMsg) Channel() courier.Channel { return m.channel }

// outgoing specific
func (m *MockMsg) QuickReplies() []string         { return m.quickReplies }
func (m *MockMsg) Locale() i18n.Locale            { return m.locale }
func (m *MockMsg) URNAuth() string                { return m.urnAuth }
func (m *MockMsg) Origin() courier.MsgOrigin      { return m.origin }
func (m *MockMsg) ContactLastSeenOn() *time.Time  { return m.contactLastSeenOn }
func (m *MockMsg) Topic() string                  { return m.topic }
func (m *MockMsg) Metadata() json.RawMessage      { return m.metadata }
func (m *MockMsg) ResponseToExternalID() string   { return m.responseToExternalID }
func (m *MockMsg) SentOn() *time.Time             { return m.sentOn }
func (m *MockMsg) IsResend() bool                 { return m.isResend }
func (m *MockMsg) Flow() *courier.FlowReference   { return m.flow }
func (m *MockMsg) OptIn() *courier.OptInReference { return m.optIn }
func (m *MockMsg) SessionStatus() string          { return "" }
func (m *MockMsg) HighPriority() bool             { return m.highPriority }

// incoming specific
func (m *MockMsg) ReceivedOn() *time.Time { return m.receivedOn }
func (m *MockMsg) WithAttachment(url string) courier.MsgIn {
	m.attachments = append(m.attachments, url)
	return m
}
func (m *MockMsg) WithContactName(name string) courier.MsgIn { m.contactName = name; return m }
func (m *MockMsg) WithURNAuthTokens(tokens map[string]string) courier.MsgIn {
	m.urnAuthTokens = tokens
	return m
}
func (m *MockMsg) WithReceivedOn(date time.Time) courier.MsgIn { m.receivedOn = &date; return m }

// used to create outgoing messages for testing
func (m *MockMsg) WithID(id courier.MsgID) courier.MsgOut       { m.id = id; return m }
func (m *MockMsg) WithUUID(uuid courier.MsgUUID) courier.MsgOut { m.uuid = uuid; return m }
func (m *MockMsg) WithMetadata(metadata json.RawMessage) courier.MsgOut {
	m.metadata = metadata
	return m
}
func (m *MockMsg) WithFlow(flow *courier.FlowReference) courier.MsgOut    { m.flow = flow; return m }
func (m *MockMsg) WithOptIn(optIn *courier.OptInReference) courier.MsgOut { m.optIn = optIn; return m }
func (m *MockMsg) WithLocale(lc i18n.Locale) courier.MsgOut               { m.locale = lc; return m }
func (m *MockMsg) WithURNAuth(token string) courier.MsgOut                { m.urnAuth = token; return m }
