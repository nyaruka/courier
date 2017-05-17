package rapidpro

import "database/sql"

// OrgID is our type for database Org ids
type OrgID struct {
	sql.NullInt64
}

// NilOrgID is our nil value for OrgID
var NilOrgID = OrgID{sql.NullInt64{Int64: 0, Valid: false}}
