package courier

import "github.com/nyaruka/gocommon/urns"

// MsgStatus is the status of a message
type MsgStatus string

// Possible values for MsgStatus
const (
	MsgStatusPending   MsgStatus = "P"
	MsgStatusQueued    MsgStatus = "Q"
	MsgStatusSent      MsgStatus = "S"
	MsgStatusWired     MsgStatus = "W"
	MsgStatusErrored   MsgStatus = "E"
	MsgStatusDelivered MsgStatus = "D"
	MsgStatusFailed    MsgStatus = "F"
	NilMsgStatus       MsgStatus = ""
)

//-----------------------------------------------------------------------------
// StatusUpdate Interface
//-----------------------------------------------------------------------------

// StatusUpdate represents a status update on a message
type StatusUpdate interface {
	Event

	ChannelUUID() ChannelUUID
	MsgID() MsgID

	SetURNUpdate(old, new urns.URN) error
	URNUpdate() (old, new urns.URN)

	ExternalID() string
	SetExternalID(string)

	Status() MsgStatus
	SetStatus(MsgStatus)
}
