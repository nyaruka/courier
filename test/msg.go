package test

import (
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/gocommon/urns"
)

// NewMockMsg creates a new outgoing message for testing
func NewMockMsg(uuid models.MsgUUID, channel *models.Channel, urn urns.URN, text string, attachments []string) *models.MsgOut {
	m := &models.MsgOut{
		UUID_:        uuid,
		URN_:         urn,
		Text_:        text,
		Attachments_: attachments,
		Channel_:     channel,
	}
	if channel != nil {
		m.ChannelUUID_ = channel.UUID()
	}
	return m
}
