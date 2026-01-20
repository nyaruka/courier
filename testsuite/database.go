package testsuite

import (
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/courier/runtime"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/null/v3"
	"github.com/stretchr/testify/require"
)

type DBMsg struct {
	OrgID              models.OrgID         `db:"org_id"`
	ID                 int64                `db:"id"`
	UUID               models.MsgUUID       `db:"uuid"`
	Direction          models.MsgDirection  `db:"direction"`
	Status             models.MsgStatus     `db:"status"`
	MsgType            string               `db:"msg_type"`
	Visibility         models.MsgVisibility `db:"visibility"`
	HighPriority       bool                 `db:"high_priority"`
	IsAndroid          bool                 `db:"is_android"`
	Text               string               `db:"text"`
	Attachments        pq.StringArray       `db:"attachments"`
	QuickReplies       pq.StringArray       `db:"quick_replies"`
	Locale             null.String          `db:"locale"`
	Templating         *models.Templating   `db:"templating"`
	ExternalIdentifier null.String          `db:"external_identifier"`
	ChannelID          models.ChannelID     `db:"channel_id"`
	ContactID          models.ContactID     `db:"contact_id"`
	ContactURNID       models.ContactURNID  `db:"contact_urn_id"`
	MsgCount           int                  `db:"msg_count"`
	CreatedByID        null.Int             `db:"created_by_id"`
	CreatedOn          time.Time            `db:"created_on"`
	ModifiedOn         time.Time            `db:"modified_on"`
	SentOn             *time.Time           `db:"sent_on"`
	ErrorCount         int                  `db:"error_count"`
	FailedReason       null.String          `db:"failed_reason"`
	NextAttempt        *time.Time           `db:"next_attempt"`
	OptInID            null.Int             `db:"optin_id"`
	LogUUIDs           pq.StringArray       `db:"log_uuids"`
}

func ReadDBMsg(t *testing.T, rt *runtime.Runtime, uuid models.MsgUUID) *DBMsg {
	m := &DBMsg{}
	err := rt.DB.Get(m, `SELECT * FROM msgs_msg WHERE uuid = $1`, uuid)
	require.NoError(t, err)
	return m
}

type ChannelEvent struct {
	ID           int64                   `db:"id"`
	UUID         models.ChannelEventUUID `db:"uuid"`
	OrgID        models.OrgID            `db:"org_id"`
	ChannelID    models.ChannelID        `db:"channel_id"`
	ContactID    models.ContactID        `db:"contact_id"`
	ContactURNID models.ContactURNID     `db:"contact_urn_id"`
	URN          urns.URN                `db:"urn"`
	EventType    models.ChannelEventType `db:"event_type"`
	OptInID      null.Int                `db:"optin_id"`
	Extra        null.Map[string]        `db:"extra"`
	Status       string                  `db:"status"`
	OccurredOn   time.Time               `db:"occurred_on"`
	CreatedOn    time.Time               `db:"created_on"`
	LogUUIDs     pq.StringArray          `db:"log_uuids"`
}

func ReadDBEvent(t *testing.T, rt *runtime.Runtime, uuid models.ChannelEventUUID) *ChannelEvent {
	e := &ChannelEvent{}
	err := rt.DB.Get(e, `SELECT * FROM channels_channelevent WHERE uuid = $1`, uuid)
	require.NoError(t, err)
	return e
}
