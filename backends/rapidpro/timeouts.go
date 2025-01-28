package rapidpro

import (
	"context"
	"fmt"
	"time"

	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/jsonx"
)

// SessionID is our type for RapidPro session ids
type SessionID int64

const sqlInsertTimeoutFire = `
INSERT INTO contacts_contactfire(org_id, contact_id, fire_type, scope, extra, fire_on)
                          VALUES($1, $2, 'T', '', $3, $4)
ON CONFLICT DO NOTHING`

// insertTimeoutFire inserts a timeout fire for the session associated with the given msg
func (b *backend) insertTimeoutFire(ctx context.Context, m *Msg) error {
	extra := map[string]any{"session_id": m.SessionID_, "session_modified_on": m.SessionModifiedOn_}
	timeoutOn := dates.Now().Add(time.Duration(m.SessionTimeout_) * time.Second)

	_, err := b.db.ExecContext(ctx, sqlInsertTimeoutFire, m.OrgID_, m.ContactID_, jsonx.MustMarshal(extra), timeoutOn)
	if err != nil {
		return fmt.Errorf("error inserting session timeout contact fire for session #%d: %w", m.SessionID_, err)
	}
	return nil
}
