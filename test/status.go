package test

import (
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
)

type MockStatusUpdate struct {
	channel            courier.Channel
	msgUUID            models.MsgUUID
	oldURN             urns.URN
	newURN             urns.URN
	externalID         string
	externalIdentifier string
	status             models.MsgStatus
	createdOn          time.Time
}

func (m *MockStatusUpdate) EventUUID() uuids.UUID           { return uuids.UUID(m.msgUUID) }
func (m *MockStatusUpdate) ChannelUUID() models.ChannelUUID { return m.channel.UUID() }
func (m *MockStatusUpdate) MsgUUID() models.MsgUUID         { return m.msgUUID }

func (m *MockStatusUpdate) SetURNUpdate(old, new urns.URN) error {
	m.oldURN = old
	m.newURN = new
	return nil
}
func (m *MockStatusUpdate) URNUpdate() (urns.URN, urns.URN) {
	return m.oldURN, m.newURN
}

func (m *MockStatusUpdate) ExternalIdentifier() string {
	if m.externalIdentifier != "" {
		return m.externalIdentifier
	}
	return m.externalID
}
func (m *MockStatusUpdate) SetExternalIdentifier(id string) {
	m.externalID = id
	m.externalIdentifier = id
}

func (m *MockStatusUpdate) Status() models.MsgStatus          { return m.status }
func (m *MockStatusUpdate) SetStatus(status models.MsgStatus) { m.status = status }
