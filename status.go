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
	EventID() int64

	ChannelUUID() ChannelUUID
	ID() MsgID

	SetUpdatedURN(old, new urns.URN) error
	UpdatedURN() (old, new urns.URN)
	HasUpdatedURN() bool

	ExternalID() string
	SetExternalID(string)

	Status() MsgStatus
	SetStatus(MsgStatus)
}
