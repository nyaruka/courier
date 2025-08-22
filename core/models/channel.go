package models

import (
	"database/sql/driver"

	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/null/v3"
)

// ChannelType is the 1-3 letter code used for channel types in the database
type ChannelType string

// AnyChannelType is our empty channel type used when doing lookups without channel type assertions
const AnyChannelType = ChannelType("")

// ChannelRole is a role that a channel can perform
type ChannelRole string

// different roles that channels can perform
const (
	ChannelRoleSend    ChannelRole = "S"
	ChannelRoleReceive ChannelRole = "R"
	ChannelRoleCall    ChannelRole = "C"
	ChannelRoleAnswer  ChannelRole = "A"
)

// ChannelUUID is our typing of a channel's UUID
type ChannelUUID uuids.UUID

// NilChannelUUID is our nil value for channel UUIDs
var NilChannelUUID = ChannelUUID("")

// ChannelID is our SQL type for a channel's id
type ChannelID null.Int

// NilChannelID represents a nil channel id
const NilChannelID = ChannelID(0)

// NewChannelID creates a new ChannelID for the passed in int64
func NewChannelID(id int64) ChannelID {
	return ChannelID(id)
}

func (i *ChannelID) Scan(value any) error         { return null.ScanInt(value, i) }
func (i ChannelID) Value() (driver.Value, error)  { return null.IntValue(i) }
func (i *ChannelID) UnmarshalJSON(b []byte) error { return null.UnmarshalInt(b, i) }
func (i ChannelID) MarshalJSON() ([]byte, error)  { return null.MarshalInt(i) }

// ChannelAddress is our SQL type for a channel address
type ChannelAddress null.String

// NilChannelAddress represents a nil channel address
const NilChannelAddress = ChannelAddress("")

func (a ChannelAddress) String() string {
	return string(a)
}
