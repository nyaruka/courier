package rapidpro

import (
	"database/sql/driver"

	"github.com/nyaruka/null/v2"
)

// OrgID is our type for database Org ids
type OrgID null.Int

// NilOrgID is our nil value for OrgID
var NilOrgID = OrgID(0)

func (i *OrgID) Scan(value any) error         { return null.ScanInt(value, i) }
func (i OrgID) Value() (driver.Value, error)  { return null.IntValue(i) }
func (i *OrgID) UnmarshalJSON(b []byte) error { return null.UnmarshalInt(b, i) }
func (i OrgID) MarshalJSON() ([]byte, error)  { return null.MarshalInt(i) }
