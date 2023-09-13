package test

import (
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/gocommon/urns"
)

type MockStatusUpdate struct {
	channel    courier.Channel
	msgID      courier.MsgID
	oldURN     urns.URN
	newURN     urns.URN
	externalID string
	status     courier.MsgStatus
	createdOn  time.Time
}

func (m *MockStatusUpdate) EventID() int64                   { return int64(m.msgID) }
func (m *MockStatusUpdate) ChannelUUID() courier.ChannelUUID { return m.channel.UUID() }
func (m *MockStatusUpdate) MsgID() courier.MsgID             { return m.msgID }

func (m *MockStatusUpdate) SetURNUpdate(old, new urns.URN) error {
	m.oldURN = old
	m.newURN = new
	return nil
}
func (m *MockStatusUpdate) URNUpdate() (urns.URN, urns.URN) {
	return m.oldURN, m.newURN
}

func (m *MockStatusUpdate) ExternalID() string      { return m.externalID }
func (m *MockStatusUpdate) SetExternalID(id string) { m.externalID = id }

func (m *MockStatusUpdate) Status() courier.MsgStatus          { return m.status }
func (m *MockStatusUpdate) SetStatus(status courier.MsgStatus) { m.status = status }
