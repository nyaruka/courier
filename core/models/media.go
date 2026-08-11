package models

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/vinovest/sqlx"
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

func (m *Media) UUID() uuids.UUID     { return m.UUID_ }
func (m *Media) Name() string         { return filepath.Base(m.Path_) }
func (m *Media) ContentType() string  { return m.ContentType_ }
func (m *Media) URL() string          { return m.URL_ }
func (m *Media) Size() int            { return m.Size_ }
func (m *Media) Width() int           { return m.Width_ }
func (m *Media) Height() int          { return m.Height_ }
func (m *Media) Duration() int        { return m.Duration_ }
func (m *Media) Alternates() []*Media { return m.Alternates_ }

var sqlSelectMediaFromUUID = `
SELECT m.uuid, m.path, m.content_type, m.url, m.size, m.width, m.height, m.duration
FROM msgs_media m
INNER JOIN msgs_media m0 ON m0.id = m.id OR m0.id = m.original_id
WHERE m0.uuid = $1
ORDER BY m.id`

func LoadMediaByUUID(ctx context.Context, db *sqlx.DB, uuid uuids.UUID) (*Media, error) {
	var records []*Media
	err := db.SelectContext(ctx, &records, sqlSelectMediaFromUUID, uuid)
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

var uuidRegex = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// ResolveMedia resolves the passed in attachment URL to a media object
func ResolveMedia(ctx context.Context, rt *runtime.Runtime, mediaUrl string) (*Media, error) {
	u, err := url.Parse(mediaUrl)
	if err != nil {
		return nil, fmt.Errorf("error parsing media URL: %w", err)
	}

	mediaUUID := uuidRegex.FindString(u.Path)

	// if hostname isn't our media domain, or path doesn't contain a UUID, don't try to resolve
	if stripS3Region(rt, u.Hostname()) != rt.Config.MediaDomain || mediaUUID == "" {
		return nil, nil
	}

	unlock := mediaMutexes.Lock(mediaUUID)
	defer unlock()

	rc := rt.VK.Get()
	defer rc.Close()

	var media *Media
	mediaJSON, err := mediaCache.Get(ctx, rc, mediaUUID)
	if err != nil {
		return nil, fmt.Errorf("error looking up cached media: %w", err)
	}
	if mediaJSON != "" {
		jsonx.MustUnmarshal([]byte(mediaJSON), &media)
	} else {
		// lookup media in our database
		media, err = LoadMediaByUUID(ctx, rt.DB, uuids.UUID(mediaUUID))
		if err != nil {
			return nil, fmt.Errorf("error looking up media: %w", err)
		}

		// cache it for future requests
		mediaCache.Set(ctx, rc, mediaUUID, string(jsonx.MustMarshal(media)))
	}

	// if we found a media record but it doesn't match the URL, don't use it
	if media == nil || (media.URL() != mediaUrl && media.URL() != stripS3Region(rt, mediaUrl)) {
		return nil, nil
	}

	return media, nil
}

// strips the region qualifier that S3 adds to virtual-host style URLs so they can be compared against our
// unqualified media domain and URLs, e.g. foo.s3.us-east-1.amazonaws.com becomes foo.s3.amazonaws.com. No-op
// when there's no region (e.g. local dev setups using path-style URLs).
func stripS3Region(rt *runtime.Runtime, s string) string {
	if rt.S3.Region == "" {
		return s
	}
	return strings.Replace(s, fmt.Sprintf("%s.", rt.S3.Region), "", -1)
}

// SaveAttachment saves an attachment to storage
func SaveAttachment(ctx context.Context, rt *runtime.Runtime, ch *Channel, contentType string, data []byte, extension string) (string, error) {
	// create our filename
	filename := string(uuids.NewV4())
	if extension != "" {
		filename = fmt.Sprintf("%s.%s", filename, extension)
	}

	orgID := ch.OrgID()

	path := filepath.Join("attachments", strconv.FormatInt(int64(orgID), 10), filename[:4], filename[4:8], filename)

	storageURL, err := rt.S3.PutObject(ctx, rt.Config.S3AttachmentsBucket, path, contentType, data, s3types.ObjectCannedACLPublicRead)
	if err != nil {
		return "", fmt.Errorf("error saving attachment to storage (bytes=%d): %w", len(data), err)
	}

	return storageURL, nil
}
