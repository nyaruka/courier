package rapidpro

import (
	"context"
	"fmt"
	"time"

	"github.com/nyaruka/gocommon/dates"
)

// SessionID is our type for RapidPro session ids
type SessionID int64

const sqlInsertTimeoutFire = `
INSERT INTO contacts_contactfire(org_id, contact_id, fire_type, scope, fire_on, session_uuid, sprint_uuid)
                          VALUES($1, $2, 'T', '', $3, $4, $5)
ON CONFLICT DO NOTHING`

// insertTimeoutFire inserts a timeout fire for the session associated with the given msg
func (b *backend) insertTimeoutFire(ctx context.Context, m *Msg) error {
	timeoutOn := dates.Now().Add(time.Duration(m.Session_.Timeout) * time.Second)

	_, err := b.db.ExecContext(ctx, sqlInsertTimeoutFire, m.OrgID_, m.ContactID_, timeoutOn, m.Session_.UUID, m.Session_.SprintUUID)
	if err != nil {
		return fmt.Errorf("error inserting session timeout contact fire for session %s: %w", m.Session_.UUID, err)
	}
	return nil
}
