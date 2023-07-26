package courier

import (
	"github.com/nyaruka/gocommon/uuids"
)

// ContactUUID is our typing of a contact's UUID
type ContactUUID uuids.UUID

// NilContactUUID is our nil value for contact UUIDs
var NilContactUUID = ContactUUID("")

//-----------------------------------------------------------------------------
// Contact Interface
//-----------------------------------------------------------------------------

// Contact defines the attributes on a contact, for our purposes that is just a contact UUID
type Contact interface {
	UUID() ContactUUID
}
