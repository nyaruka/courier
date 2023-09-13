package rapidpro

import (
	"context"
	"database/sql"
	"path/filepath"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/gocommon/uuids"
)

type Media struct {
	UUID_        uuids.UUID `db:"uuid"         json:"uuid"`
	Path_        string     `db:"path"         json:"path"`
	ContentType_ string     `db:"content_type" json:"content_type"`
	URL_         string     `db:"url"          json:"url"`
	Size_        int        `db:"size"         json:"size"`
	Width_       int        `db:"width"        json:"width"`
	Height_      int        `db:"height"       json:"height"`
	Duration_    int        `db:"duration"     json:"duration"`
	Alternates_  []*Media   `                  json:"alternates"`
}

func (m *Media) UUID() uuids.UUID    { return m.UUID_ }
func (m *Media) Name() string        { return filepath.Base(m.Path_) }
func (m *Media) ContentType() string { return m.ContentType_ }
func (m *Media) URL() string         { return m.URL_ }
func (m *Media) Size() int           { return m.Size_ }
func (m *Media) Width() int          { return m.Width_ }
func (m *Media) Height() int         { return m.Height_ }
func (m *Media) Duration() int       { return m.Duration_ }
func (m *Media) Alternates() []courier.Media {
	as := make([]courier.Media, len(m.Alternates_))
	for i, alt := range m.Alternates_ {
		as[i] = alt
	}
	return as
}

var _ courier.Media = &Media{}

var sqlLookupMediaFromUUID = `
SELECT m.uuid, m.path, m.content_type, m.url, m.size, m.width, m.height, m.duration
FROM msgs_media m
INNER JOIN msgs_media m0 ON m0.id = m.id OR m0.id = m.original_id
WHERE m0.uuid = $1
ORDER BY m.id`

func lookupMediaFromUUID(ctx context.Context, db *sqlx.DB, uuid uuids.UUID) (*Media, error) {
	var records []*Media
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
