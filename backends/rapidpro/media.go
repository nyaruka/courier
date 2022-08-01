package rapidpro

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/gocommon/uuids"
)

type Media struct {
	UUID_        uuids.UUID `db:"uuid"`
	ContentType_ string     `db:"content_type"`
	URL_         string     `db:"url"`
	Width_       int        `db:"width"`
	Height_      int        `db:"height"`
	Duration_    int        `db:"duration"`

	alternates []courier.Media
}

func (m *Media) UUID() uuids.UUID            { return m.UUID_ }
func (m *Media) ContentType() string         { return m.ContentType_ }
func (m *Media) URL() string                 { return m.URL_ }
func (m *Media) Width() int                  { return m.Width_ }
func (m *Media) Height() int                 { return m.Height_ }
func (m *Media) Duration() int               { return m.Duration_ }
func (m *Media) Alternates() []courier.Media { return m.alternates }

var _ courier.Media = &Media{}

var sqlLookupMediaFromUUID = `
SELECT m.uuid, m.content_type, m.url, m.width, m.height, m.duration
FROM msgs_media m
INNER JOIN msgs_media m0 ON m0.id = m.id OR m0.id = m.original_id
WHERE m0.uuid = $1
ORDER BY m.created_on`

func lookupMediaFromUUID(ctx context.Context, db *sqlx.DB, uuid uuids.UUID) (*Media, error) {
	var records []*Media
	err := db.SelectContext(ctx, &records, sqlLookupMediaFromUUID, uuid)
	if err == sql.ErrNoRows {
		return nil, nil
	}

	media, alternates := records[0], records[1:]
	media.alternates = make([]courier.Media, len(alternates))
	for i, alt := range alternates {
		media.alternates[i] = alt
	}
	return media, nil
}
