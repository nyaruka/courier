package rapidpro

import (
	"database/sql/driver"

	"github.com/nyaruka/null"
)

// OrgID is our type for database Org ids
type OrgID null.Int

// NilOrgID is our nil value for OrgID
var NilOrgID = OrgID(0)

// MarshalJSON marshals into JSON. 0 values will become null
func (i OrgID) MarshalJSON() ([]byte, error) {
	return null.Int(i).MarshalJSON()
}

// UnmarshalJSON unmarshals from JSON. null values become 0
func (i *OrgID) UnmarshalJSON(b []byte) error {
	return null.UnmarshalInt(b, (*null.Int)(i))
}

// Value returns the db value, null is returned for 0
func (i OrgID) Value() (driver.Value, error) {
	return null.Int(i).Value()
}

// Scan scans from the db value. null values become 0
func (i *OrgID) Scan(value interface{}) error {
	return null.ScanInt(value, (*null.Int)(i))
}
