package whatsapp_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/courier/handlers/meta/whatsapp"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetMsgPayloads(t *testing.T) {
	ctx := context.Background()
	maxMsgLength := 4096

	// Create a mock channel
	channel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "WAC", "12345", "", []string{urns.WhatsApp.Prefix}, nil)

	tcs := []struct {
		label                string
		text                 string
		attachments          []string
		quickReplies         []models.QuickReply
		locale               i18n.Locale
		expectedPayloadsCount int
		expectedType         string // type of first payload
		checkFunc            func(*testing.T, []whatsapp.SendRequest, *courier.ChannelLog)
	}{
		// Test case (a): ≤3 QRs with Extra + attachment
		{
			label:                "3 QRs with Extra and attachment - should use list with attachment as separate message",
			text:                 "Pick an option",
			attachments:          []string{"image/jpeg:https://example.com/image.jpg"},
			quickReplies:         []models.QuickReply{{Type: "text", Text: "Option 1", Extra: "Description 1"}, {Type: "text", Text: "Option 2", Extra: "Description 2"}},
			expectedPayloadsCount: 2,
			expectedType:         "image",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *courier.ChannelLog) {
				assert.Equal(t, 2, len(payloads))
				// First should be image attachment
				assert.Equal(t, "image", payloads[0].Type)
				assert.NotNil(t, payloads[0].Image)
				assert.Equal(t, "https://example.com/image.jpg", payloads[0].Image.Link)
				// Second should be interactive list
				assert.Equal(t, "interactive", payloads[1].Type)
				assert.NotNil(t, payloads[1].Interactive)
				assert.Equal(t, "list", payloads[1].Interactive.Type)
				assert.Equal(t, "Pick an option", payloads[1].Interactive.Body.Text)
				assert.Equal(t, 2, len(payloads[1].Interactive.Action.Sections[0].Rows))
				assert.Equal(t, "Option 1", payloads[1].Interactive.Action.Sections[0].Rows[0].Title)
				assert.Equal(t, "Description 1", payloads[1].Interactive.Action.Sections[0].Rows[0].Description)
			},
		},
		{
			label:                "2 QRs with Extra and attachment - should use list with attachment as separate message",
			text:                 "Choose wisely",
			attachments:          []string{"video/mp4:https://example.com/video.mp4"},
			quickReplies:         []models.QuickReply{{Type: "text", Text: "Yes", Extra: "Agree"}, {Type: "text", Text: "No", Extra: "Disagree"}},
			expectedPayloadsCount: 2,
			expectedType:         "video",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *courier.ChannelLog) {
				assert.Equal(t, 2, len(payloads))
				// First should be video attachment
				assert.Equal(t, "video", payloads[0].Type)
				assert.NotNil(t, payloads[0].Video)
				// Second should be interactive list
				assert.Equal(t, "interactive", payloads[1].Type)
				assert.Equal(t, "list", payloads[1].Interactive.Type)
			},
		},
		// Test case (b): ≤3 QRs + image/video/document attachment header
		{
			label:                "2 QRs with image attachment - should use image as header",
			text:                 "Select an option",
			attachments:          []string{"image/jpeg:https://example.com/image.jpg"},
			quickReplies:         []models.QuickReply{{Type: "text", Text: "Option 1"}, {Type: "text", Text: "Option 2"}},
			expectedPayloadsCount: 1,
			expectedType:         "interactive",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *courier.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				// Should be interactive button with image header
				assert.Equal(t, "interactive", payloads[0].Type)
				assert.NotNil(t, payloads[0].Interactive)
				assert.Equal(t, "button", payloads[0].Interactive.Type)
				// Check header
				assert.NotNil(t, payloads[0].Interactive.Header)
				assert.Equal(t, "image", payloads[0].Interactive.Header.Type)
				assert.NotNil(t, payloads[0].Interactive.Header.Image)
				assert.Equal(t, "https://example.com/image.jpg", payloads[0].Interactive.Header.Image.Link)
				// Check buttons
				assert.Equal(t, 2, len(payloads[0].Interactive.Action.Buttons))
			},
		},
		{
			label:                "3 QRs with video attachment - should use video as header",
			text:                 "Watch and choose",
			attachments:          []string{"video/mp4:https://example.com/video.mp4"},
			quickReplies:         []models.QuickReply{{Type: "text", Text: "Like"}, {Type: "text", Text: "Dislike"}, {Type: "text", Text: "Share"}},
			expectedPayloadsCount: 1,
			expectedType:         "interactive",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *courier.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				// Should be interactive button with video header
				assert.Equal(t, "interactive", payloads[0].Type)
				assert.Equal(t, "button", payloads[0].Interactive.Type)
				// Check header
				assert.NotNil(t, payloads[0].Interactive.Header)
				assert.Equal(t, "video", payloads[0].Interactive.Header.Type)
				assert.NotNil(t, payloads[0].Interactive.Header.Video)
				assert.Equal(t, "https://example.com/video.mp4", payloads[0].Interactive.Header.Video.Link)
				// Check buttons
				assert.Equal(t, 3, len(payloads[0].Interactive.Action.Buttons))
			},
		},
		{
			label:                "1 QR with document attachment - should use document as header",
			text:                 "Review this",
			attachments:          []string{"document/pdf:https://example.com/document.pdf"},
			quickReplies:         []models.QuickReply{{Type: "text", Text: "Approve"}},
			expectedPayloadsCount: 1,
			expectedType:         "interactive",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *courier.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				// Should be interactive button with document header
				assert.Equal(t, "interactive", payloads[0].Type)
				assert.Equal(t, "button", payloads[0].Interactive.Type)
				// Check header
				assert.NotNil(t, payloads[0].Interactive.Header)
				assert.Equal(t, "document", payloads[0].Interactive.Header.Type)
				assert.NotNil(t, payloads[0].Interactive.Header.Document)
				assert.Equal(t, "https://example.com/document.pdf", payloads[0].Interactive.Header.Document.Link)
				assert.Equal(t, "document.pdf", payloads[0].Interactive.Header.Document.Filename)
			},
		},
		{
			label:                "3 QRs with audio attachment - should NOT use as header, audio not supported",
			text:                 "Listen and respond",
			attachments:          []string{"audio/mp3:https://example.com/audio.mp3"},
			quickReplies:         []models.QuickReply{{Type: "text", Text: "Good"}, {Type: "text", Text: "Bad"}},
			expectedPayloadsCount: 2,
			expectedType:         "audio",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *courier.ChannelLog) {
				assert.Equal(t, 2, len(payloads))
				// First should be audio (not used as header)
				assert.Equal(t, "audio", payloads[0].Type)
				assert.NotNil(t, payloads[0].Audio)
				// Second should be interactive button WITHOUT header
				assert.Equal(t, "interactive", payloads[1].Type)
				assert.Equal(t, "button", payloads[1].Interactive.Type)
				assert.Nil(t, payloads[1].Interactive.Header)
			},
		},
		// Test case (c): >10 QRs truncation
		{
			label:        "12 QRs - should truncate to 10",
			text:         "Select from many options",
			quickReplies: []models.QuickReply{
				{Type: "text", Text: "Option 1"}, {Type: "text", Text: "Option 2"}, {Type: "text", Text: "Option 3"},
				{Type: "text", Text: "Option 4"}, {Type: "text", Text: "Option 5"}, {Type: "text", Text: "Option 6"},
				{Type: "text", Text: "Option 7"}, {Type: "text", Text: "Option 8"}, {Type: "text", Text: "Option 9"},
				{Type: "text", Text: "Option 10"}, {Type: "text", Text: "Option 11"}, {Type: "text", Text: "Option 12"},
			},
			expectedPayloadsCount: 1,
			expectedType:         "interactive",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *courier.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				// Should be interactive list with exactly 10 rows
				assert.Equal(t, "interactive", payloads[0].Type)
				assert.Equal(t, "list", payloads[0].Interactive.Type)
				assert.Equal(t, 10, len(payloads[0].Interactive.Action.Sections[0].Rows))
				// Verify it's the first 10 options
				assert.Equal(t, "Option 1", payloads[0].Interactive.Action.Sections[0].Rows[0].Title)
				assert.Equal(t, "Option 10", payloads[0].Interactive.Action.Sections[0].Rows[9].Title)
				// Check that error was logged
				assert.Equal(t, 1, len(clog.Errors))
				assert.Contains(t, clog.Errors[0].Message, "too many quick replies")
			},
		},
		{
			label:        "15 QRs with Extra - should truncate to 10",
			text:         "Many choices",
			quickReplies: []models.QuickReply{
				{Type: "text", Text: "Option 1", Extra: "Desc 1"}, {Type: "text", Text: "Option 2", Extra: "Desc 2"},
				{Type: "text", Text: "Option 3", Extra: "Desc 3"}, {Type: "text", Text: "Option 4", Extra: "Desc 4"},
				{Type: "text", Text: "Option 5", Extra: "Desc 5"}, {Type: "text", Text: "Option 6", Extra: "Desc 6"},
				{Type: "text", Text: "Option 7", Extra: "Desc 7"}, {Type: "text", Text: "Option 8", Extra: "Desc 8"},
				{Type: "text", Text: "Option 9", Extra: "Desc 9"}, {Type: "text", Text: "Option 10", Extra: "Desc 10"},
				{Type: "text", Text: "Option 11", Extra: "Desc 11"}, {Type: "text", Text: "Option 12", Extra: "Desc 12"},
				{Type: "text", Text: "Option 13", Extra: "Desc 13"}, {Type: "text", Text: "Option 14", Extra: "Desc 14"},
				{Type: "text", Text: "Option 15", Extra: "Desc 15"},
			},
			expectedPayloadsCount: 1,
			expectedType:         "interactive",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *courier.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				assert.Equal(t, "interactive", payloads[0].Type)
				assert.Equal(t, "list", payloads[0].Interactive.Type)
				assert.Equal(t, 10, len(payloads[0].Interactive.Action.Sections[0].Rows))
				// Verify descriptions are preserved for first 10
				assert.Equal(t, "Desc 1", payloads[0].Interactive.Action.Sections[0].Rows[0].Description)
				assert.Equal(t, "Desc 10", payloads[0].Interactive.Action.Sections[0].Rows[9].Description)
				// Check error logged
				assert.Equal(t, 1, len(clog.Errors))
			},
		},
		// Additional edge cases
		{
			label:                "4 QRs without Extra - should use list (>3 buttons)",
			text:                 "Pick one",
			quickReplies:         []models.QuickReply{{Type: "text", Text: "A"}, {Type: "text", Text: "B"}, {Type: "text", Text: "C"}, {Type: "text", Text: "D"}},
			expectedPayloadsCount: 1,
			expectedType:         "interactive",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *courier.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				assert.Equal(t, "interactive", payloads[0].Type)
				assert.Equal(t, "list", payloads[0].Interactive.Type)
				assert.Equal(t, 4, len(payloads[0].Interactive.Action.Sections[0].Rows))
			},
		},
		{
			label:                "3 QRs without Extra and no attachment - should use buttons",
			text:                 "Quick choice",
			quickReplies:         []models.QuickReply{{Type: "text", Text: "Yes"}, {Type: "text", Text: "No"}, {Type: "text", Text: "Maybe"}},
			expectedPayloadsCount: 1,
			expectedType:         "interactive",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *courier.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				assert.Equal(t, "interactive", payloads[0].Type)
				assert.Equal(t, "button", payloads[0].Interactive.Type)
				assert.Equal(t, 3, len(payloads[0].Interactive.Action.Buttons))
				assert.Nil(t, payloads[0].Interactive.Header)
			},
		},
		{
			label:                "No quick replies with attachment and text - should have caption",
			text:                 "Check this out",
			attachments:          []string{"image/jpeg:https://example.com/image.jpg"},
			expectedPayloadsCount: 1,
			expectedType:         "image",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *courier.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				assert.Equal(t, "image", payloads[0].Type)
				assert.NotNil(t, payloads[0].Image)
				assert.Equal(t, "Check this out", payloads[0].Image.Caption)
			},
		},
		{
			label:                "Multiple attachments with QRs - first attachment as header, second sent separately",
			text:                 "Multiple files",
			attachments:          []string{"image/jpeg:https://example.com/image1.jpg", "image/jpeg:https://example.com/image2.jpg"},
			quickReplies:         []models.QuickReply{{Type: "text", Text: "Download"}},
			expectedPayloadsCount: 2,
			expectedType:         "image",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *courier.ChannelLog) {
				assert.Equal(t, 2, len(payloads))
				// Second attachment sent first as standalone
				assert.Equal(t, "image", payloads[0].Type)
				assert.Equal(t, "https://example.com/image2.jpg", payloads[0].Image.Link)
				// Then interactive with first attachment as header
				assert.Equal(t, "interactive", payloads[1].Type)
				assert.Equal(t, "button", payloads[1].Interactive.Type)
				assert.NotNil(t, payloads[1].Interactive.Header)
				assert.Equal(t, "image", payloads[1].Interactive.Header.Type)
				assert.Equal(t, "https://example.com/image1.jpg", payloads[1].Interactive.Header.Image.Link)
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.label, func(t *testing.T) {
			// Create mock message
			mockMsg := test.NewMockMsg("87995844-2017-4ba0-bc73-f3da75b32f9b", channel, "whatsapp:250788123123", tc.text, tc.attachments)
			mockMsg.SetQuickReplies(tc.quickReplies)
			var msg courier.MsgOut = mockMsg
			if tc.locale != "" {
				msg = mockMsg.WithLocale(tc.locale)
			}

			// Create channel log
			clog := courier.NewChannelLogForSend(msg, nil)

			// Call GetMsgPayloads
			payloads, err := whatsapp.GetMsgPayloads(ctx, msg, maxMsgLength, clog)

			// Assert no error
			require.NoError(t, err)

			// Assert expected number of payloads
			assert.Equal(t, tc.expectedPayloadsCount, len(payloads), "unexpected number of payloads")

			// Assert first payload type
			if len(payloads) > 0 {
				assert.Equal(t, tc.expectedType, payloads[0].Type, "unexpected first payload type")
			}

			// Run custom checks
			if tc.checkFunc != nil {
				tc.checkFunc(t, payloads, clog)
			}

			// Debug output
			if t.Failed() {
				for i, p := range payloads {
					b, _ := json.MarshalIndent(p, "", "  ")
					t.Logf("Payload %d: %s", i, string(b))
				}
			}
		})
	}
}
