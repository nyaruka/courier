package models

import (
	"database/sql/driver"
	"strconv"

	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/null/v3"
)

// ContactID is our representation of our database contact id
type ContactID null.Int

// NilContactID represents our nil value for ContactID
var NilContactID = ContactID(0)

func (i *ContactID) Scan(value any) error         { return null.ScanInt(value, i) }
func (i ContactID) Value() (driver.Value, error)  { return null.IntValue(i) }
func (i *ContactID) UnmarshalJSON(b []byte) error { return null.UnmarshalInt(b, i) }
func (i ContactID) MarshalJSON() ([]byte, error)  { return null.MarshalInt(i) }

// String returns a string representation of the id
func (i ContactID) String() string {
	if i != NilContactID {
		return strconv.FormatInt(int64(i), 10)
	}
	return "null"
}

// ContactUUID is our typing of a contact's UUID
type ContactUUID uuids.UUID

// NilContactUUID is our nil value for contact UUIDs
var NilContactUUID = ContactUUID("")
