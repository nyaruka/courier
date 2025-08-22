package handlers

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/core/models"
)

type MediaType string

const (
	MediaTypeImage       MediaType = "image"
	MediaTypeAudio       MediaType = "audio"
	MediaTypeVideo       MediaType = "video"
	MediaTypeApplication MediaType = "application"
)

type MediaTypeSupport struct {
	Types    []string
	MaxBytes int
}

// Attachment is a resolved attachment
type Attachment struct {
	Type        MediaType
	Name        string
	ContentType string
	URL         string
	Media       *models.Media
	Thumbnail   *models.Media
}

// ResolveAttachments resolves the given attachment strings (content-type:url) into attachment objects
func ResolveAttachments(ctx context.Context, b courier.Backend, attachments []string, support map[MediaType]MediaTypeSupport, allowURLOnly bool, clog *courier.ChannelLog) ([]*Attachment, error) {
	resolved := make([]*Attachment, 0, len(attachments))

	for _, as := range attachments {
		// split into content-type and URL
		parts := strings.SplitN(as, ":", 2)
		if len(parts) <= 1 || strings.HasPrefix(parts[1], "//") {
			return nil, fmt.Errorf("invalid attachment format: %s", as)
		}
		contentType, mediaUrl := parts[0], parts[1]

		att, err := resolveAttachment(ctx, b, contentType, mediaUrl, support, allowURLOnly)
		if err != nil {
			return nil, err
		}
		if att != nil {
			resolved = append(resolved, att)
		} else {
			clog.Error(courier.ErrorMediaUnresolveable(contentType))
		}
	}

	return resolved, nil
}

func resolveAttachment(ctx context.Context, b courier.Backend, contentType, mediaUrl string, support map[MediaType]MediaTypeSupport, allowURLOnly bool) (*Attachment, error) {
	media, err := b.ResolveMedia(ctx, mediaUrl)
	if err != nil {
		return nil, err
	}

	if media == nil {
		// if the channel type allows it, we can still use the media URL without being able to resolve it
		if allowURLOnly {
			// potentially fix the URL which might not be properly encoded
			parsedURL, err := url.Parse(mediaUrl)
			if err == nil {
				mediaUrl = parsedURL.String()
			}

			mediaType, _ := parseContentType(contentType)
			name := filenameFromURL(mediaUrl)
			return &Attachment{Type: mediaType, Name: name, ContentType: contentType, URL: mediaUrl}, nil
		} else {
			return nil, nil
		}
	}

	mediaType, _ := parseContentType(media.ContentType())
	mediaSupport := support[mediaType]

	// our candidates are the uploaded media and any alternates of the same media type
	candidates := append([]*models.Media{media}, filterMediaByType(media.Alternates(), mediaType)...)

	// narrow down the candidates to the ones we support
	if len(mediaSupport.Types) > 0 {
		candidates = filterMediaByContentTypes(candidates, mediaSupport.Types)
	}

	// narrow down the candidates to the ones that don't exceed our max bytes
	if mediaSupport.MaxBytes > 0 {
		candidates = filterMediaBySize(candidates, mediaSupport.MaxBytes)
	}

	// if we have no candidates, we can't use this media
	if len(candidates) == 0 {
		return nil, nil
	}
	media = candidates[0]

	// if we have an image alternate, that can be a thumbnail
	var thumbnail *models.Media
	thumbnails := filterMediaByType(media.Alternates(), MediaTypeImage)
	if len(thumbnails) > 0 {
		thumbnail = thumbnails[0]
	}

	return &Attachment{
		Type:        mediaType,
		Name:        media.Name(),
		ContentType: media.ContentType(),
		URL:         media.URL(),
		Media:       media,
		Thumbnail:   thumbnail,
	}, nil
}

func filterMediaByType(in []*models.Media, mediaType MediaType) []*models.Media {
	return filterMedia(in, func(m *models.Media) bool {
		mt, _ := parseContentType(m.ContentType())
		return mt == mediaType
	})
}

func filterMediaByContentTypes(in []*models.Media, types []string) []*models.Media {
	return filterMedia(in, func(m *models.Media) bool {
		for _, t := range types {
			if m.ContentType() == t {
				return true
			}
		}
		return false
	})
}

func filterMediaBySize(in []*models.Media, maxBytes int) []*models.Media {
	return filterMedia(in, func(m *models.Media) bool { return m.Size() <= maxBytes })
}

func filterMedia(in []*models.Media, f func(*models.Media) bool) []*models.Media {
	filtered := make([]*models.Media, 0, len(in))
	for _, m := range in {
		if f(m) {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

func filenameFromURL(u string) string {
	name := path.Base(u)
	unescaped, err := url.PathUnescape(name)
	if err == nil {
		return unescaped
	}
	return name
}

func parseContentType(t string) (MediaType, string) {
	parts := strings.SplitN(t, "/", 2)
	if len(parts) == 2 {
		return MediaType(parts[0]), parts[1]
	}
	return MediaType(parts[0]), ""
}
