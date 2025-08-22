package courier

import (
	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/gocommon/urns"
)

//-----------------------------------------------------------------------------
// StatusUpdate Interface
//-----------------------------------------------------------------------------

// StatusUpdate represents a status update on a message
type StatusUpdate interface {
	Event

	ChannelUUID() ChannelUUID
	MsgID() models.MsgID

	SetURNUpdate(old, new urns.URN) error
	URNUpdate() (old, new urns.URN)

	ExternalID() string
	SetExternalID(string)

	Status() models.MsgStatus
	SetStatus(models.MsgStatus)
}
