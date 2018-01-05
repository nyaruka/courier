package courier

// MsgStatusValue is the status of a message
type MsgStatusValue string

// Possible values for MsgStatus
const (
	MsgPending   MsgStatusValue = "P"
	MsgQueued    MsgStatusValue = "Q"
	MsgSent      MsgStatusValue = "S"
	MsgWired     MsgStatusValue = "W"
	MsgErrored   MsgStatusValue = "E"
	MsgDelivered MsgStatusValue = "D"
	MsgFailed    MsgStatusValue = "F"
	NilMsgStatus MsgStatusValue = ""
)

//-----------------------------------------------------------------------------
// MsgStatusUpdate Interface
//-----------------------------------------------------------------------------

// MsgStatus represents a status update on a message
type MsgStatus interface {
	EventID() int64

	ChannelUUID() ChannelUUID
	ID() MsgID

	ExternalID() string
	SetExternalID(string)

	Status() MsgStatusValue
	SetStatus(MsgStatusValue)

	Logs() []*ChannelLog
	AddLog(log *ChannelLog)
}
