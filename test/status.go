package test

import (
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/gocommon/urns"
)

type mockMsgStatus struct {
	channel    courier.Channel
	id         courier.MsgID
	oldURN     urns.URN
	newURN     urns.URN
	externalID string
	status     courier.MsgStatusValue
	createdOn  time.Time

	logs []*courier.ChannelLog
}

func (m *mockMsgStatus) ChannelUUID() courier.ChannelUUID { return m.channel.UUID() }
func (m *mockMsgStatus) ID() courier.MsgID                { return m.id }
func (m *mockMsgStatus) EventID() int64                   { return int64(m.id) }

func (m *mockMsgStatus) SetUpdatedURN(old, new urns.URN) error {
	m.oldURN = old
	m.newURN = new
	return nil
}
func (m *mockMsgStatus) UpdatedURN() (urns.URN, urns.URN) {
	return m.oldURN, m.newURN
}
func (m *mockMsgStatus) HasUpdatedURN() bool {
	if m.oldURN != urns.NilURN && m.newURN != urns.NilURN {
		return true
	}
	return false
}

func (m *mockMsgStatus) ExternalID() string      { return m.externalID }
func (m *mockMsgStatus) SetExternalID(id string) { m.externalID = id }

func (m *mockMsgStatus) Status() courier.MsgStatusValue          { return m.status }
func (m *mockMsgStatus) SetStatus(status courier.MsgStatusValue) { m.status = status }

func (m *mockMsgStatus) Logs() []*courier.ChannelLog    { return m.logs }
func (m *mockMsgStatus) AddLog(log *courier.ChannelLog) { m.logs = append(m.logs, log) }
