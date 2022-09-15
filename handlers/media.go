package handlers

import (
	"context"
	"net/url"
	"path"
	"strings"

	"github.com/nyaruka/courier"
	"github.com/pkg/errors"
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
	Media       courier.Media
	Thumbnail   courier.Media
}

// ResolveAttachments resolves the given attachment strings (content-type:url) into attachment objects
func ResolveAttachments(ctx context.Context, b courier.Backend, attachments []string, support map[MediaType]MediaTypeSupport, allowURLOnly bool) ([]*Attachment, error) {
	resolved := make([]*Attachment, 0, len(attachments))

	for _, as := range attachments {
		att, err := resolveAttachment(ctx, b, as, support, allowURLOnly)
		if err != nil {
			return nil, err
		}
		if att != nil {
			resolved = append(resolved, att)
		}
	}

	return resolved, nil
}

func resolveAttachment(ctx context.Context, b courier.Backend, attachment string, support map[MediaType]MediaTypeSupport, allowURLOnly bool) (*Attachment, error) {
	// split into content-type and URL
	parts := strings.SplitN(attachment, ":", 2)
	if len(parts) <= 1 || strings.HasPrefix(parts[1], "//") {
		return nil, errors.Errorf("invalid attachment format: %s", attachment)
	}
	contentType, mediaUrl := parts[0], parts[1]

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
	candidates := append([]courier.Media{media}, filterMediaByType(media.Alternates(), mediaType)...)

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
	var thumbnail courier.Media
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

func filterMediaByType(in []courier.Media, mediaType MediaType) []courier.Media {
	return filterMedia(in, func(m courier.Media) bool {
		mt, _ := parseContentType(m.ContentType())
		return mt == mediaType
	})
}

func filterMediaByContentTypes(in []courier.Media, types []string) []courier.Media {
	return filterMedia(in, func(m courier.Media) bool {
		for _, t := range types {
			if m.ContentType() == t {
				return true
			}
		}
		return false
	})
}

func filterMediaBySize(in []courier.Media, maxBytes int) []courier.Media {
	return filterMedia(in, func(m courier.Media) bool { return m.Size() <= maxBytes })
}

func filterMedia(in []courier.Media, f func(courier.Media) bool) []courier.Media {
	filtered := make([]courier.Media, 0, len(in))
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
