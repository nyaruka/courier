package handlers_test

import (
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/gocommon/svclogs"
	"github.com/stretchr/testify/assert"
)

func TestResolveAttachments(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)

	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)

	// media only resolves for URLs on our media domain that contain the media's UUID
	rt.Config.MediaDomain = "mock.com"

	// media without alternates are created with empty (non-nil) alternate slices to match how they're read back
	imageJPG := test.NewMockMedia("ec6972be-809c-4c8d-be59-ba9dbd74c977", "test.jpg", "image/jpeg", "http://mock.com/media/ec6972be-809c-4c8d-be59-ba9dbd74c977/test.jpg", 1024*1024, 640, 480, 0, []*models.Media{})

	audioM4A := test.NewMockMedia("d8f6d8bb-9dd0-4b34-98b8-f2e9e857f2b6", "test.m4a", "audio/mp4", "http://mock.com/media/d8f6d8bb-9dd0-4b34-98b8-f2e9e857f2b6/test.m4a", 1024*1024, 0, 0, 200, nil)
	audioMP3 := test.NewMockMedia("9a4c4415-a06c-4edd-ad5b-33ed0be6b306", "test.mp3", "audio/mp3", "http://mock.com/media/9a4c4415-a06c-4edd-ad5b-33ed0be6b306/test.mp3", 1024*1024, 0, 0, 200, []*models.Media{audioM4A})

	thumbJPG := test.NewMockMedia("2f8db4b2-e21c-4fe4-a049-4dbcecf83cf6", "test.jpg", "image/jpeg", "http://mock.com/media/2f8db4b2-e21c-4fe4-a049-4dbcecf83cf6/test.jpg", 1024*1024, 640, 480, 0, nil)
	videoMP4 := test.NewMockMedia("55be7386-6851-406f-9c02-2b17bd05eb45", "test.mp4", "video/mp4", "http://mock.com/media/55be7386-6851-406f-9c02-2b17bd05eb45/test.mp4", 1024*1024, 0, 0, 1000, []*models.Media{thumbJPG})

	videoMOV := test.NewMockMedia("1a1a5b81-6f4f-4bf9-9dfc-e0e13c8b0d47", "test.mov", "video/quicktime", "http://mock.com/media/1a1a5b81-6f4f-4bf9-9dfc-e0e13c8b0d47/test.mov", 100*1024*1024, 0, 0, 2000, []*models.Media{})

	testsuite.InsertMedia(t, rt, imageJPG)
	testsuite.InsertMedia(t, rt, audioMP3)
	testsuite.InsertMedia(t, rt, videoMP4)
	testsuite.InsertMedia(t, rt, videoMOV)

	tcs := []struct {
		attachments  []string
		mediaSupport map[handlers.MediaType]handlers.MediaTypeSupport
		allowURLOnly bool
		resolved     []*handlers.Attachment
		errors       []*svclogs.Error
		err          string
	}{
		{ // 0: user entered image URL
			attachments:  []string{"image:https://example.com/image%201.jpg"},
			mediaSupport: map[handlers.MediaType]handlers.MediaTypeSupport{handlers.MediaTypeImage: {Types: []string{"image/png"}}}, // ignored
			allowURLOnly: true,
			resolved: []*handlers.Attachment{
				{Type: handlers.MediaTypeImage, Name: "image 1.jpg", ContentType: "image", URL: "https://example.com/image%201.jpg"},
			},
			errors: []*svclogs.Error{},
		},
		{ // 1: user entered audio URL which isn't properly escaped
			attachments:  []string{"image:https://example.com/audio 1.m4a"},
			mediaSupport: map[handlers.MediaType]handlers.MediaTypeSupport{handlers.MediaTypeImage: {Types: []string{"audio/mp3"}}}, // ignored
			allowURLOnly: true,
			resolved: []*handlers.Attachment{
				{Type: handlers.MediaTypeImage, Name: "audio 1.m4a", ContentType: "image", URL: "https://example.com/audio%201.m4a"},
			},
			errors: []*svclogs.Error{},
		},
		{ // 2: user entered image URL, URL only attachments not allowed
			attachments:  []string{"image:https://example.com/image.jpg"},
			mediaSupport: map[handlers.MediaType]handlers.MediaTypeSupport{handlers.MediaTypeImage: {Types: []string{"image/png"}}}, // ignored
			allowURLOnly: false,
			resolved:     []*handlers.Attachment{},
			errors:       []*svclogs.Error{models.ErrorMediaUnresolveable("image")},
		},
		{ // 3: resolveable uploaded image URL
			attachments:  []string{"image/jpeg:http://mock.com/media/ec6972be-809c-4c8d-be59-ba9dbd74c977/test.jpg"},
			mediaSupport: map[handlers.MediaType]handlers.MediaTypeSupport{handlers.MediaTypeImage: {Types: []string{"image/png", "image/jpeg"}}},
			allowURLOnly: true,
			resolved: []*handlers.Attachment{
				{Type: handlers.MediaTypeImage, Name: "test.jpg", ContentType: "image/jpeg", URL: "http://mock.com/media/ec6972be-809c-4c8d-be59-ba9dbd74c977/test.jpg", Media: imageJPG, Thumbnail: nil},
			},
			errors: []*svclogs.Error{},
		},
		{ // 4: unresolveable uploaded image URL
			attachments:  []string{"image/jpeg:http://mock.com/media/ff5c9d3a-2a3e-4a43-8ea9-4a4b3b7d1c3e/gone.jpg"},
			mediaSupport: map[handlers.MediaType]handlers.MediaTypeSupport{handlers.MediaTypeImage: {Types: []string{"image/jpeg", "image/png"}}},
			allowURLOnly: true,
			resolved: []*handlers.Attachment{
				{Type: handlers.MediaTypeImage, Name: "gone.jpg", ContentType: "image/jpeg", URL: "http://mock.com/media/ff5c9d3a-2a3e-4a43-8ea9-4a4b3b7d1c3e/gone.jpg", Media: nil, Thumbnail: nil},
			},
			errors: []*svclogs.Error{},
		},
		{ // 5: unresolveable uploaded image URL, URL only attachments not allowed
			attachments:  []string{"image/jpeg:http://mock.com/media/ff5c9d3a-2a3e-4a43-8ea9-4a4b3b7d1c3e/gone.jpg"},
			mediaSupport: map[handlers.MediaType]handlers.MediaTypeSupport{handlers.MediaTypeImage: {Types: []string{"image/jpeg", "image/png"}}},
			allowURLOnly: false,
			resolved:     []*handlers.Attachment{},
			errors:       []*svclogs.Error{models.ErrorMediaUnresolveable("image/jpeg")},
		},
		{ // 6: resolveable uploaded image URL, type not in supported types
			attachments:  []string{"image/jpeg:http://mock.com/media/ec6972be-809c-4c8d-be59-ba9dbd74c977/test.jpg"},
			mediaSupport: map[handlers.MediaType]handlers.MediaTypeSupport{handlers.MediaTypeImage: {Types: []string{"image/png"}}},
			allowURLOnly: true,
			resolved:     []*handlers.Attachment{},
			errors:       []*svclogs.Error{models.ErrorMediaUnresolveable("image/jpeg")},
		},
		{ // 7: resolveable uploaded audio URL, type in supported types
			attachments:  []string{"audio/mp3:http://mock.com/media/9a4c4415-a06c-4edd-ad5b-33ed0be6b306/test.mp3"},
			mediaSupport: map[handlers.MediaType]handlers.MediaTypeSupport{handlers.MediaTypeAudio: {Types: []string{"audio/mp3", "audio/mp4"}}},
			allowURLOnly: true,
			resolved: []*handlers.Attachment{
				{Type: handlers.MediaTypeAudio, Name: "test.mp3", ContentType: "audio/mp3", URL: "http://mock.com/media/9a4c4415-a06c-4edd-ad5b-33ed0be6b306/test.mp3", Media: audioMP3, Thumbnail: nil},
			},
			errors: []*svclogs.Error{},
		},
		{ // 8: resolveable uploaded audio URL, type not in supported types, but has alternate
			attachments:  []string{"audio/mp3:http://mock.com/media/9a4c4415-a06c-4edd-ad5b-33ed0be6b306/test.mp3"},
			mediaSupport: map[handlers.MediaType]handlers.MediaTypeSupport{handlers.MediaTypeAudio: {Types: []string{"audio/mp4"}}},
			allowURLOnly: true,
			resolved: []*handlers.Attachment{
				{Type: handlers.MediaTypeAudio, Name: "test.m4a", ContentType: "audio/mp4", URL: "http://mock.com/media/d8f6d8bb-9dd0-4b34-98b8-f2e9e857f2b6/test.m4a", Media: audioM4A, Thumbnail: nil},
			},
			errors: []*svclogs.Error{},
		},
		{ // 9: resolveable uploaded video URL, has thumbnail
			attachments:  []string{"video/mp4:http://mock.com/media/55be7386-6851-406f-9c02-2b17bd05eb45/test.mp4"},
			mediaSupport: map[handlers.MediaType]handlers.MediaTypeSupport{handlers.MediaTypeVideo: {Types: []string{"video/mp4", "video/quicktime"}}},
			allowURLOnly: true,
			resolved: []*handlers.Attachment{
				{Type: handlers.MediaTypeVideo, Name: "test.mp4", ContentType: "video/mp4", URL: "http://mock.com/media/55be7386-6851-406f-9c02-2b17bd05eb45/test.mp4", Media: videoMP4, Thumbnail: thumbJPG},
			},
			errors: []*svclogs.Error{},
		},
		{ // 10: resolveable uploaded video URL, no thumbnail
			attachments:  []string{"video/quicktime:http://mock.com/media/1a1a5b81-6f4f-4bf9-9dfc-e0e13c8b0d47/test.mov"},
			mediaSupport: map[handlers.MediaType]handlers.MediaTypeSupport{handlers.MediaTypeVideo: {Types: []string{"video/mp4", "video/quicktime"}}},
			allowURLOnly: true,
			resolved: []*handlers.Attachment{
				{Type: handlers.MediaTypeVideo, Name: "test.mov", ContentType: "video/quicktime", URL: "http://mock.com/media/1a1a5b81-6f4f-4bf9-9dfc-e0e13c8b0d47/test.mov", Media: videoMOV, Thumbnail: nil},
			},
			errors: []*svclogs.Error{},
		},
		{ // 11: resolveable uploaded video URL, too big
			attachments:  []string{"video/quicktime:http://mock.com/media/1a1a5b81-6f4f-4bf9-9dfc-e0e13c8b0d47/test.mov"},
			mediaSupport: map[handlers.MediaType]handlers.MediaTypeSupport{handlers.MediaTypeVideo: {Types: []string{"video/quicktime"}, MaxBytes: 10 * 1024 * 1024}},
			allowURLOnly: true,
			resolved:     []*handlers.Attachment{},
			errors:       []*svclogs.Error{models.ErrorMediaUnresolveable("video/quicktime")},
		},
		{ // 12: invalid attachment format
			attachments:  []string{"image"},
			mediaSupport: map[handlers.MediaType]handlers.MediaTypeSupport{},
			err:          "invalid attachment format: image",
		},
		{ // 13: invalid attachment format (missing content type)
			attachments:  []string{"http://mock.com/media/ec6972be-809c-4c8d-be59-ba9dbd74c977/test.jpg"},
			mediaSupport: map[handlers.MediaType]handlers.MediaTypeSupport{},
			err:          "invalid attachment format: http://mock.com/media/ec6972be-809c-4c8d-be59-ba9dbd74c977/test.jpg",
		},
	}

	for i, tc := range tcs {
		clog := models.NewChannelLog(models.ChannelLogTypeMsgSend, nil, nil, nil)

		resolved, err := handlers.ResolveAttachments(ctx, rt, tc.attachments, tc.mediaSupport, tc.allowURLOnly, clog)
		if tc.err != "" {
			assert.EqualError(t, err, tc.err, "expected error for test %d", i)
		} else {
			assert.NoError(t, err, "unexpected error for test %d", i)
			assert.Equal(t, tc.resolved, resolved, "resolved mismatch for test %d", i)
			assert.Equal(t, tc.errors, clog.Errors, "errors mismatch for test %d", i)
		}
	}
}
