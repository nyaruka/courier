package handlers_test

import (
	"context"
	"testing"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/stretchr/testify/assert"
)

func TestResolveAttachments(t *testing.T) {
	ctx := context.Background()
	mb := courier.NewMockBackend()

	imageJPG := courier.NewMockMedia("image/jpeg", "http://mock.com/1234/test.jpg", 123, 640, 480, 0, nil)

	audioM4A := courier.NewMockMedia("audio/mp4", "http://mock.com/2345/test.m4a", 123, 0, 0, 200, nil)
	audioMP3 := courier.NewMockMedia("audio/mp3", "http://mock.com/3456/test.mp3", 123, 0, 0, 200, []courier.Media{audioM4A})

	thumbJPG := courier.NewMockMedia("image/jpeg", "http://mock.com/4567/test.jpg", 123, 640, 480, 0, nil)
	videoMP4 := courier.NewMockMedia("video/mp4", "http://mock.com/5678/test.mp4", 123, 0, 0, 1000, []courier.Media{thumbJPG})

	videoMOV := courier.NewMockMedia("video/quicktime", "http://mock.com/6789/test.mov", 123, 0, 0, 2000, nil)

	mb.MockMedia(imageJPG)
	mb.MockMedia(audioMP3)
	mb.MockMedia(videoMP4)
	mb.MockMedia(videoMOV)

	tcs := []struct {
		attachments    []string
		supportedTypes []string
		allowExternal  bool
		resolved       []*handlers.Attachment
		err            string
	}{
		{ // 0: user entered image URL
			attachments:    []string{"image:https://example.com/image.jpg"},
			supportedTypes: []string{"image/png"}, // ignored
			allowExternal:  true,
			resolved: []*handlers.Attachment{
				{Type: handlers.MediaTypeImage, ContentType: "image", URL: "https://example.com/image.jpg"},
			},
		},
		{ // 1: user entered image URL, external URLs not allowed
			attachments:    []string{"image:https://example.com/image.jpg"},
			supportedTypes: []string{"image/png"}, // ignored
			allowExternal:  false,
			resolved:       []*handlers.Attachment{},
		},
		{ // 2: resolveable uploaded image URL
			attachments:    []string{"image/jpeg:http://mock.com/1234/test.jpg"},
			supportedTypes: []string{"image/jpeg", "image/png"},
			allowExternal:  true,
			resolved: []*handlers.Attachment{
				{Type: handlers.MediaTypeImage, ContentType: "image/jpeg", URL: "http://mock.com/1234/test.jpg", Media: imageJPG, Thumbnail: nil},
			},
		},
		{ // 3: unresolveable uploaded image URL
			attachments:    []string{"image/jpeg:http://mock.com/9876/gone.jpg"},
			supportedTypes: []string{"image/jpeg", "image/png"},
			allowExternal:  true,
			resolved: []*handlers.Attachment{
				{Type: handlers.MediaTypeImage, ContentType: "image/jpeg", URL: "http://mock.com/9876/gone.jpg", Media: nil, Thumbnail: nil},
			},
		},
		{ // 4: unresolveable uploaded image URL, external URLs not allowed
			attachments:    []string{"image/jpeg:http://mock.com/9876/gone.jpg"},
			supportedTypes: []string{"image/jpeg", "image/png"},
			allowExternal:  false,
			resolved:       []*handlers.Attachment{},
		},
		{ // 5: resolveable uploaded image URL, type not in supported types
			attachments:    []string{"image/jpeg:http://mock.com/1234/test.jpg"},
			supportedTypes: []string{"image/png", "audio/mp4"},
			allowExternal:  true,
			resolved:       []*handlers.Attachment{},
		},
		{ // 6: resolveable uploaded audio URL, type in supported types
			attachments:    []string{"audio/mp3:http://mock.com/3456/test.mp3"},
			supportedTypes: []string{"image/jpeg", "audio/mp3", "audio/mp4"},
			allowExternal:  true,
			resolved: []*handlers.Attachment{
				{Type: handlers.MediaTypeAudio, ContentType: "audio/mp3", URL: "http://mock.com/3456/test.mp3", Media: audioMP3, Thumbnail: nil},
			},
		},
		{ // 7: resolveable uploaded audio URL, type not in supported types, but has alternate
			attachments:    []string{"audio/mp3:http://mock.com/3456/test.mp3"},
			supportedTypes: []string{"image/jpeg", "audio/mp4"},
			allowExternal:  true,
			resolved: []*handlers.Attachment{
				{Type: handlers.MediaTypeAudio, ContentType: "audio/mp4", URL: "http://mock.com/2345/test.m4a", Media: audioM4A, Thumbnail: nil},
			},
		},
		{ // 8: resolveable uploaded video URL, has thumbnail
			attachments:    []string{"video/mp4:http://mock.com/5678/test.mp4"},
			supportedTypes: []string{"image/jpeg", "audio/mp4", "video/mp4"},
			allowExternal:  true,
			resolved: []*handlers.Attachment{
				{Type: handlers.MediaTypeVideo, ContentType: "video/mp4", URL: "http://mock.com/5678/test.mp4", Media: videoMP4, Thumbnail: thumbJPG},
			},
		},
		{ // 9: resolveable uploaded video URL, no thumbnail
			attachments:    []string{"video/quicktime:http://mock.com/6789/test.mov"},
			supportedTypes: []string{"image/jpeg", "audio/mp4", "video/mp4", "video/quicktime"},
			allowExternal:  true,
			resolved: []*handlers.Attachment{
				{Type: handlers.MediaTypeVideo, ContentType: "video/quicktime", URL: "http://mock.com/6789/test.mov", Media: videoMOV, Thumbnail: nil},
			},
		},
		{ // 10: invalid attachment format
			attachments:    []string{"image"},
			supportedTypes: []string{"image/jpeg"},
			err:            "invalid attachment format: image",
		},
		{ // 11: invalid attachment format (missing content type)
			attachments:    []string{"http://mock.com/1234/test.jpg"},
			supportedTypes: []string{"image/jpeg"},
			err:            "invalid attachment format: http://mock.com/1234/test.jpg",
		},
	}

	for i, tc := range tcs {
		resolved, err := handlers.ResolveAttachments(ctx, mb, tc.attachments, tc.supportedTypes, tc.allowExternal)
		if tc.err != "" {
			assert.EqualError(t, err, tc.err, "expected error for test %d", i)
		} else {
			assert.NoError(t, err, "unexpected error for test %d", i)
			assert.Equal(t, tc.resolved, resolved, "mismatch for test %d", i)
		}
	}
}
