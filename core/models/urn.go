package models

import (
	"database/sql/driver"

	"github.com/nyaruka/null/v3"
)

// ContactURNID represents a contact urn's id
type ContactURNID null.Int

// NilContactURNID is our constant for a nil contact URN id
const NilContactURNID = ContactURNID(0)

func (i *ContactURNID) Scan(value any) error         { return null.ScanInt(value, i) }
func (i ContactURNID) Value() (driver.Value, error)  { return null.IntValue(i) }
func (i *ContactURNID) UnmarshalJSON(b []byte) error { return null.UnmarshalInt(b, i) }
func (i ContactURNID) MarshalJSON() ([]byte, error)  { return null.MarshalInt(i) }
