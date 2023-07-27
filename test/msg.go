package test

import (
	"encoding/json"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/gocommon/urns"
)

type mockMsg struct {
	id                   courier.MsgID
	uuid                 courier.MsgUUID
	channel              courier.Channel
	urn                  urns.URN
	urnAuth              string
	text                 string
	attachments          []string
	locale               courier.Locale
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

	flow *courier.FlowReference

	receivedOn *time.Time
	sentOn     *time.Time
	wiredOn    *time.Time
}

func NewMockMsg(id courier.MsgID, uuid courier.MsgUUID, channel courier.Channel, urn urns.URN, text string) courier.Msg {
	return &mockMsg{
		id:      id,
		uuid:    uuid,
		channel: channel,
		urn:     urn,
		text:    text,
	}
}

func (m *mockMsg) SessionStatus() string        { return "" }
func (m *mockMsg) Flow() *courier.FlowReference { return m.flow }

func (m *mockMsg) FlowName() string {
	if m.flow == nil {
		return ""
	}
	return m.flow.Name
}

func (m *mockMsg) FlowUUID() string {
	if m.flow == nil {
		return ""
	}
	return m.flow.UUID
}

func (m *mockMsg) Channel() courier.Channel      { return m.channel }
func (m *mockMsg) ID() courier.MsgID             { return m.id }
func (m *mockMsg) EventID() int64                { return int64(m.id) }
func (m *mockMsg) UUID() courier.MsgUUID         { return m.uuid }
func (m *mockMsg) Text() string                  { return m.text }
func (m *mockMsg) Attachments() []string         { return m.attachments }
func (m *mockMsg) Locale() courier.Locale        { return m.locale }
func (m *mockMsg) ExternalID() string            { return m.externalID }
func (m *mockMsg) URN() urns.URN                 { return m.urn }
func (m *mockMsg) URNAuth() string               { return m.urnAuth }
func (m *mockMsg) ContactName() string           { return m.contactName }
func (m *mockMsg) HighPriority() bool            { return m.highPriority }
func (m *mockMsg) QuickReplies() []string        { return m.quickReplies }
func (m *mockMsg) Origin() courier.MsgOrigin     { return m.origin }
func (m *mockMsg) ContactLastSeenOn() *time.Time { return m.contactLastSeenOn }
func (m *mockMsg) Topic() string                 { return m.topic }
func (m *mockMsg) ResponseToExternalID() string  { return m.responseToExternalID }
func (m *mockMsg) Metadata() json.RawMessage     { return m.metadata }
func (m *mockMsg) IsResend() bool                { return m.isResend }
func (m *mockMsg) ReceivedOn() *time.Time        { return m.receivedOn }
func (m *mockMsg) SentOn() *time.Time            { return m.sentOn }
func (m *mockMsg) WiredOn() *time.Time           { return m.wiredOn }

func (m *mockMsg) WithContactName(name string) courier.Msg   { m.contactName = name; return m }
func (m *mockMsg) WithURNAuth(auth string) courier.Msg       { m.urnAuth = auth; return m }
func (m *mockMsg) WithReceivedOn(date time.Time) courier.Msg { m.receivedOn = &date; return m }
func (m *mockMsg) WithID(id courier.MsgID) courier.Msg       { m.id = id; return m }
func (m *mockMsg) WithUUID(uuid courier.MsgUUID) courier.Msg { m.uuid = uuid; return m }
func (m *mockMsg) WithAttachment(url string) courier.Msg {
	m.attachments = append(m.attachments, url)
	return m
}
func (m *mockMsg) WithLocale(lc courier.Locale) courier.Msg          { m.locale = lc; return m }
func (m *mockMsg) WithMetadata(metadata json.RawMessage) courier.Msg { m.metadata = metadata; return m }

func (m *mockMsg) WithFlow(flow *courier.FlowReference) courier.Msg { m.flow = flow; return m }
