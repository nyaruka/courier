package rapidpro

import (
	"context"
	"time"
)

// SessionID is our type for RapidPro session ids
type SessionID int64

const updateSessionTimeoutSQL = `
	UPDATE
		flows_flowsession
	SET
		timeout_on = NOW() + $3 * interval '1 second'
	WHERE
		id = $1 AND
		extract(epoch from modified_on) = extract(epoch from $2::timestamptz) AND
		status = 'W'
`

// updateSessionTimeout updates the timeout_on value on a db session if our session's wait on hasn't changed
func updateSessionTimeout(ctx context.Context, b *backend, sessionID SessionID, sessionModifiedOn time.Time, timeoutSeconds int) error {
	_, err := b.db.ExecContext(ctx, updateSessionTimeoutSQL, sessionID, sessionModifiedOn.In(time.UTC), timeoutSeconds)
	return err
}
