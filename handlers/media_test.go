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

	imageJPG := courier.NewMockMedia("image/jpeg", "http://mock.com/1234/test.jpg", 1024*1024, 640, 480, 0, nil)

	audioM4A := courier.NewMockMedia("audio/mp4", "http://mock.com/2345/test.m4a", 1024*1024, 0, 0, 200, nil)
	audioMP3 := courier.NewMockMedia("audio/mp3", "http://mock.com/3456/test.mp3", 1024*1024, 0, 0, 200, []courier.Media{audioM4A})

	thumbJPG := courier.NewMockMedia("image/jpeg", "http://mock.com/4567/test.jpg", 1024*1024, 640, 480, 0, nil)
	videoMP4 := courier.NewMockMedia("video/mp4", "http://mock.com/5678/test.mp4", 1024*1024, 0, 0, 1000, []courier.Media{thumbJPG})

	videoMOV := courier.NewMockMedia("video/quicktime", "http://mock.com/6789/test.mov", 100*1024*1024, 0, 0, 2000, nil)

	mb.MockMedia(imageJPG)
	mb.MockMedia(audioMP3)
	mb.MockMedia(videoMP4)
	mb.MockMedia(videoMOV)

	tcs := []struct {
		attachments  []string
		mediaSupport map[handlers.MediaType]handlers.MediaTypeSupport
		allowURLOnly bool
		resolved     []*handlers.Attachment
		err          string
	}{
		{ // 0: user entered image URL
			attachments:  []string{"image:https://example.com/image.jpg"},
			mediaSupport: map[handlers.MediaType]handlers.MediaTypeSupport{handlers.MediaTypeImage: {Types: []string{"image/png"}}}, // ignored
			allowURLOnly: true,
			resolved: []*handlers.Attachment{
				{Type: handlers.MediaTypeImage, ContentType: "image", URL: "https://example.com/image.jpg"},
			},
		},
		{ // 1: user entered image URL, URL only attachments not allowed
			attachments:  []string{"image:https://example.com/image.jpg"},
			mediaSupport: map[handlers.MediaType]handlers.MediaTypeSupport{handlers.MediaTypeImage: {Types: []string{"image/png"}}}, // ignored
			allowURLOnly: false,
			resolved:     []*handlers.Attachment{},
		},
		{ // 2: resolveable uploaded image URL
			attachments:  []string{"image/jpeg:http://mock.com/1234/test.jpg"},
			mediaSupport: map[handlers.MediaType]handlers.MediaTypeSupport{handlers.MediaTypeImage: {Types: []string{"image/jpeg", "image/png"}}},
			allowURLOnly: true,
			resolved: []*handlers.Attachment{
				{Type: handlers.MediaTypeImage, ContentType: "image/jpeg", URL: "http://mock.com/1234/test.jpg", Media: imageJPG, Thumbnail: nil},
			},
		},
		{ // 3: unresolveable uploaded image URL
			attachments:  []string{"image/jpeg:http://mock.com/9876/gone.jpg"},
			mediaSupport: map[handlers.MediaType]handlers.MediaTypeSupport{handlers.MediaTypeImage: {Types: []string{"image/jpeg", "image/png"}}},
			allowURLOnly: true,
			resolved: []*handlers.Attachment{
				{Type: handlers.MediaTypeImage, ContentType: "image/jpeg", URL: "http://mock.com/9876/gone.jpg", Media: nil, Thumbnail: nil},
			},
		},
		{ // 4: unresolveable uploaded image URL, URL only attachments not allowed
			attachments:  []string{"image/jpeg:http://mock.com/9876/gone.jpg"},
			mediaSupport: map[handlers.MediaType]handlers.MediaTypeSupport{handlers.MediaTypeImage: {Types: []string{"image/jpeg", "image/png"}}},
			allowURLOnly: false,
			resolved:     []*handlers.Attachment{},
		},
		{ // 5: resolveable uploaded image URL, type not in supported types
			attachments:  []string{"image/jpeg:http://mock.com/1234/test.jpg"},
			mediaSupport: map[handlers.MediaType]handlers.MediaTypeSupport{handlers.MediaTypeImage: {Types: []string{"image/png"}}},
			allowURLOnly: true,
			resolved:     []*handlers.Attachment{},
		},
		{ // 6: resolveable uploaded audio URL, type in supported types
			attachments:  []string{"audio/mp3:http://mock.com/3456/test.mp3"},
			mediaSupport: map[handlers.MediaType]handlers.MediaTypeSupport{handlers.MediaTypeAudio: {Types: []string{"audio/mp3", "audio/mp4"}}},
			allowURLOnly: true,
			resolved: []*handlers.Attachment{
				{Type: handlers.MediaTypeAudio, ContentType: "audio/mp3", URL: "http://mock.com/3456/test.mp3", Media: audioMP3, Thumbnail: nil},
			},
		},
		{ // 7: resolveable uploaded audio URL, type not in supported types, but has alternate
			attachments:  []string{"audio/mp3:http://mock.com/3456/test.mp3"},
			mediaSupport: map[handlers.MediaType]handlers.MediaTypeSupport{handlers.MediaTypeAudio: {Types: []string{"audio/mp4"}}},
			allowURLOnly: true,
			resolved: []*handlers.Attachment{
				{Type: handlers.MediaTypeAudio, ContentType: "audio/mp4", URL: "http://mock.com/2345/test.m4a", Media: audioM4A, Thumbnail: nil},
			},
		},
		{ // 8: resolveable uploaded video URL, has thumbnail
			attachments:  []string{"video/mp4:http://mock.com/5678/test.mp4"},
			mediaSupport: map[handlers.MediaType]handlers.MediaTypeSupport{handlers.MediaTypeVideo: {Types: []string{"video/mp4", "video/quicktime"}}},
			allowURLOnly: true,
			resolved: []*handlers.Attachment{
				{Type: handlers.MediaTypeVideo, ContentType: "video/mp4", URL: "http://mock.com/5678/test.mp4", Media: videoMP4, Thumbnail: thumbJPG},
			},
		},
		{ // 9: resolveable uploaded video URL, no thumbnail
			attachments:  []string{"video/quicktime:http://mock.com/6789/test.mov"},
			mediaSupport: map[handlers.MediaType]handlers.MediaTypeSupport{handlers.MediaTypeVideo: {Types: []string{"video/mp4", "video/quicktime"}}},
			allowURLOnly: true,
			resolved: []*handlers.Attachment{
				{Type: handlers.MediaTypeVideo, ContentType: "video/quicktime", URL: "http://mock.com/6789/test.mov", Media: videoMOV, Thumbnail: nil},
			},
		},
		{ // 10: resolveable uploaded video URL, too big
			attachments:  []string{"video/quicktime:http://mock.com/6789/test.mov"},
			mediaSupport: map[handlers.MediaType]handlers.MediaTypeSupport{handlers.MediaTypeVideo: {Types: []string{"video/quicktime"}, MaxBytes: 10 * 1024 * 1024}},
			allowURLOnly: true,
			resolved:     []*handlers.Attachment{},
		},
		{ // 11: invalid attachment format
			attachments:  []string{"image"},
			mediaSupport: map[handlers.MediaType]handlers.MediaTypeSupport{},
			err:          "invalid attachment format: image",
		},
		{ // 12: invalid attachment format (missing content type)
			attachments:  []string{"http://mock.com/1234/test.jpg"},
			mediaSupport: map[handlers.MediaType]handlers.MediaTypeSupport{},
			err:          "invalid attachment format: http://mock.com/1234/test.jpg",
		},
	}

	for i, tc := range tcs {
		resolved, err := handlers.ResolveAttachments(ctx, mb, tc.attachments, tc.mediaSupport, tc.allowURLOnly)
		if tc.err != "" {
			assert.EqualError(t, err, tc.err, "expected error for test %d", i)
		} else {
			assert.NoError(t, err, "unexpected error for test %d", i)
			assert.Equal(t, tc.resolved, resolved, "mismatch for test %d", i)
		}
	}
}
