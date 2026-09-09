package models

import (
	"errors"
	"fmt"
)

// OrgID is our type for database Org ids
type OrgID int

// NoLimit is returned for a limit which isn't configured for an org
const NoLimit = -1

// LimitReachedError is returned when an operation can't be performed because it would take the org over one of its
// configured limits.
type LimitReachedError struct {
	Limit string
	Max   int
}

func (e *LimitReachedError) Error() string {
	return fmt.Sprintf("workspace has reached its limit of %d %s", e.Max, e.Limit)
}

// isLimitReached returns whether the given error is an operation being refused because of an org limit
func isLimitReached(err error) bool {
	var lre *LimitReachedError
	return errors.As(err, &lre)
}
