package rapidpro

import (
	"context"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/nyaruka/courier"
	"strings"
	"time"
)

const selectMsgAttachmentForExternalID = `
SELECT m."id" msg_id, m."uuid" msg_uuid, m."channel_id", c."uuid" channel_uuid, m."external_id", m."attachments"
FROM "msgs_msg" m INNER JOIN "channels_channel" c ON (m."channel_id" = c."id")
WHERE ( c."id" = $1 AND m."external_id" = $2 AND m."direction" = 'I')
ORDER BY m."modified_on" DESC
LIMIT 1
`

const updateMsgAttachmentsID = `
UPDATE msgs_msg SET attachments = :attachments, modified_on = :modified_on
WHERE 
	id = :msg_id AND
	channel_id = :channel_id AND 
	direction = 'I'
RETURNING 
	id msg_id
`

// newMsgAttachment creates a new DBMsgStatus for the passed in parameters
func newMsgAttachment(channel courier.Channel, externalID string, attachmentUrl string) *DBMsgAttachment {
	dbChannel := channel.(*DBChannel)

	return &DBMsgAttachment{
		ChannelUUID_:   channel.UUID(),
		ChannelID_:     dbChannel.ID(),
		ExternalID_:    externalID,
		NewAttachment_: attachmentUrl,
		ModifiedOn_:    time.Now().In(time.UTC),
	}
}

// validateMsgAttachmentInDB validates attachment by ExternalID and fills with data from the DB
func validateMsgAttachmentInDB(b *backend, a *DBMsgAttachment) (err error) {
	err = b.db.Get(a, selectMsgAttachmentForExternalID, a.ChannelID(), a.ExternalID())
	return err
}

// writeMsgAttachment append attachment to the existing message with the same ExternalID
func writeMsgAttachment(ctx context.Context, b *backend, channel courier.Channel, attachment *courier.MsgAttachment) error {
	dbAttachment := (*attachment).(*DBMsgAttachment)

	if strings.HasPrefix(dbAttachment.NewAttachment_, "http") {
		url, err := downloadMediaToS3(ctx, b, channel, dbAttachment.OrgID_, dbAttachment.UUID_, dbAttachment.NewAttachment_)
		if err != nil {
			return err
		}
		dbAttachment.NewAttachment_ = url
	}
	dbAttachment.Attachments_ = append(dbAttachment.Attachments_, dbAttachment.NewAttachment_)

	err := writeMsgAttachmentToDB(ctx, b, dbAttachment)

	if err == courier.ErrMsgNotFound {
		return err
	}

	// failed writing, write to our spool instead
	if err != nil {
		err = courier.WriteToSpool(b.config.SpoolDir, "statuses", dbAttachment)
	}

	return err
}

func writeMsgAttachmentToDB(ctx context.Context, b *backend, attachment *DBMsgAttachment) error {
	var rows *sqlx.Rows
	var err error

	if attachment.ID() != courier.NilMsgID {
		rows, err = b.db.NamedQueryContext(ctx, updateMsgAttachmentsID, attachment)
	}
	if err != nil {
		return err
	}
	defer rows.Close()

	// scan and read the id of the msg that was updated
	if rows.Next() {
		rows.Scan(&attachment.ID_)
	} else {
		return courier.ErrMsgNotFound
	}

	return nil
}

//-----------------------------------------------------------------------------
// MsgAttachmentUpdate implementation
//-----------------------------------------------------------------------------

// DBMsgAttachment represents an attachment update on a message
type DBMsgAttachment struct {
	ChannelUUID_   courier.ChannelUUID `json:"channel_uuid"             db:"channel_uuid"`
	ChannelID_     courier.ChannelID   `json:"channel_id"               db:"channel_id"`
	OrgID_         OrgID               `json:"org_id"                   db:"org_id"`
	ID_            courier.MsgID       `json:"msg_id,omitempty"         db:"msg_id"`
	UUID_          courier.MsgUUID     `json:"msg_uuid"                 db:"msg_uuid"`
	ExternalID_    string              `json:"external_id,omitempty"    db:"external_id"`
	ModifiedOn_    time.Time           `json:"modified_on"              db:"modified_on"`
	Attachments_   pq.StringArray      `json:"attachments"              db:"attachments"`
	NewAttachment_ string
}

func (a *DBMsgAttachment) Attachments() []string            { return a.Attachments_ }
func (a *DBMsgAttachment) ChannelID() courier.ChannelID     { return a.ChannelID_ }
func (a *DBMsgAttachment) ChannelUUID() courier.ChannelUUID { return a.ChannelUUID_ }
func (a *DBMsgAttachment) ID() courier.MsgID                { return a.ID_ }
func (a *DBMsgAttachment) ExternalID() string               { return a.ExternalID_ }
