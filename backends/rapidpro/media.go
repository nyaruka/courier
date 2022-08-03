package rapidpro

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/gocommon/uuids"
)

type DBMedia struct {
	UUID_        uuids.UUID `db:"uuid"         json:"uuid"`
	ContentType_ string     `db:"content_type" json:"content_type"`
	URL_         string     `db:"url"          json:"url"`
	Size_        int        `db:"size"        json:"size"`
	Width_       int        `db:"width"        json:"width"`
	Height_      int        `db:"height"       json:"height"`
	Duration_    int        `db:"duration"     json:"duration"`

	Alternates_ []*DBMedia `json:"alternates"`
}

func (m *DBMedia) UUID() uuids.UUID    { return m.UUID_ }
func (m *DBMedia) ContentType() string { return m.ContentType_ }
func (m *DBMedia) URL() string         { return m.URL_ }
func (m *DBMedia) Size() int           { return m.Size_ }
func (m *DBMedia) Width() int          { return m.Width_ }
func (m *DBMedia) Height() int         { return m.Height_ }
func (m *DBMedia) Duration() int       { return m.Duration_ }
func (m *DBMedia) Alternates() []courier.Media {
	as := make([]courier.Media, len(m.Alternates_))
	for i, alt := range m.Alternates_ {
		as[i] = alt
	}
	return as
}

var _ courier.Media = &DBMedia{}

var sqlLookupMediaFromUUID = `
SELECT m.uuid, m.content_type, m.url, m.size, m.width, m.height, m.duration
FROM msgs_media m
INNER JOIN msgs_media m0 ON m0.id = m.id OR m0.id = m.original_id
WHERE m0.uuid = $1
ORDER BY m.id`

func lookupMediaFromUUID(ctx context.Context, db *sqlx.DB, uuid uuids.UUID) (*DBMedia, error) {
	var records []*DBMedia
	err := db.SelectContext(ctx, &records, sqlLookupMediaFromUUID, uuid)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}

	media, alternates := records[0], records[1:]
	media.Alternates_ = alternates
	return media, nil
}
