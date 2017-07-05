package courier

import (
	"sync"
	"time"
)

// MsgStatus is the status of a message
type MsgStatus string

// Possible values for MsgStatus
const (
	MsgPending   MsgStatus = "P"
	MsgQueued    MsgStatus = "Q"
	MsgSent      MsgStatus = "S"
	MsgWired     MsgStatus = "W"
	MsgErrored   MsgStatus = "E"
	MsgDelivered MsgStatus = "D"
	MsgFailed    MsgStatus = "F"
	NilMsgStatus MsgStatus = ""
)

// NewStatusUpdateForID creates a new status update for a message identified by its primary key
func NewStatusUpdateForID(channel Channel, id MsgID, status MsgStatus) *MsgStatusUpdate {
	s := statusPool.Get().(*MsgStatusUpdate)
	s.Channel = channel
	s.ID = id
	s.ExternalID = ""
	s.Status = status
	s.CreatedOn = time.Now()
	return s
}

// NewStatusUpdateForExternalID creates a new status update for a message identified by its external ID
func NewStatusUpdateForExternalID(channel Channel, externalID string, status MsgStatus) *MsgStatusUpdate {
	s := statusPool.Get().(*MsgStatusUpdate)
	s.Channel = channel
	s.ID = NilMsgID
	s.ExternalID = externalID
	s.Status = status
	s.CreatedOn = time.Now()
	return s
}

// AddLog adds a channel log object to the passed in status
func (m *MsgStatusUpdate) AddLog(log *ChannelLog) {
	m.Logs = append(m.Logs, log)
}

var statusPool = sync.Pool{New: func() interface{} { return &MsgStatusUpdate{} }}

//-----------------------------------------------------------------------------
// MsgStatusUpdate implementation
//-----------------------------------------------------------------------------

// MsgStatusUpdate represents a status update on a message
type MsgStatusUpdate struct {
	Channel    Channel
	ID         MsgID
	ExternalID string
	Status     MsgStatus
	CreatedOn  time.Time
	Logs       []*ChannelLog
}

// Release releases this status and assigns it back to our pool for reuse
func (m *MsgStatusUpdate) Release() {
	m.clear()
	statusPool.Put(m)
}

func (m *MsgStatusUpdate) clear() {
	m.Channel = nil
	m.ID = NilMsgID
	m.ExternalID = ""
	m.Status = ""
	m.CreatedOn = time.Time{}
	m.Logs = nil
}
