package rapidpro

import null "gopkg.in/guregu/null.v3"

// OrgID is our type for database Org ids
type OrgID struct {
	null.Int
}

// NilOrgID is our nil value for OrgID
var NilOrgID = OrgID{null.NewInt(0, false)}
