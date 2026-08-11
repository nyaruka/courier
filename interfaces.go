package courier

import (
	"github.com/nyaruka/gocommon/uuids"
)

// Event is our interface for the types of things a ChannelHandleFunc can return.
type Event interface {
	EventUUID() uuids.UUID
}
