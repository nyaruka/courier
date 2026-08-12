package whatsapp_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers/meta/whatsapp"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecipientFields(t *testing.T) {
	tcs := []struct {
		urn               urns.URN
		expectedTo        string
		expectedRecipient string
	}{
		{urn: "whatsapp:250788123123", expectedTo: "250788123123", expectedRecipient: ""},                       // phone number -> to
		{urn: "whatsapp:US.13491208655302741918", expectedTo: "", expectedRecipient: "US.13491208655302741918"}, // BSUID -> recipient
		{urn: "bsuid:US.13491208655302741918", expectedTo: "", expectedRecipient: "US.13491208655302741918"},    // legacy bsuid scheme -> recipient
	}

	for _, tc := range tcs {
		to, recipient := whatsapp.RecipientFields(tc.urn)
		assert.Equal(t, tc.expectedTo, to, "to mismatch for %s", tc.urn)
		assert.Equal(t, tc.expectedRecipient, recipient, "recipient mismatch for %s", tc.urn)
	}
}

func TestGetMsgPayloads(t *testing.T) {
	ctx := context.Background()
	maxMsgLength := 4096

	// Create a mock channel
	channel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "WAC", "12345", "", []string{urns.WhatsApp.Prefix}, nil)

	tcs := []struct {
		label                 string
		text                  string
		attachments           []string
		quickReplies          []models.QuickReply
		locale                i18n.Locale
		urn                   urns.URN
		expectedPayloadsCount int
		expectedType          string // type of first payload
		checkFunc             func(*testing.T, []whatsapp.SendRequest, *models.ChannelLog)
	}{
		// Test case (a): ≤3 QRs with Extra + attachment
		{
			label:                 "3 QRs with Extra and attachment - should use list with attachment as separate message",
			text:                  "Pick an option",
			attachments:           []string{"image/jpeg:https://example.com/image.jpg"},
			quickReplies:          []models.QuickReply{{Type: "text", Text: "Option 1", Extra: "Description 1"}, {Type: "text", Text: "Option 2", Extra: "Description 2"}},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 2,
			expectedType:          "image",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 2, len(payloads))
				// First should be image attachment
				assert.Equal(t, "image", payloads[0].Type)
				assert.Equal(t, "250788123123", payloads[0].To)
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
			label:                 "2 QRs with Extra and attachment - should use list with attachment as separate message",
			text:                  "Choose wisely",
			attachments:           []string{"video/mp4:https://example.com/video.mp4"},
			quickReplies:          []models.QuickReply{{Type: "text", Text: "Yes", Extra: "Agree"}, {Type: "text", Text: "No", Extra: "Disagree"}},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 2,
			expectedType:          "video",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 2, len(payloads))
				// First should be video attachment
				assert.Equal(t, "video", payloads[0].Type)
				assert.Equal(t, "250788123123", payloads[0].To)
				assert.NotNil(t, payloads[0].Video)
				// Second should be interactive list
				assert.Equal(t, "interactive", payloads[1].Type)
				assert.Equal(t, "list", payloads[1].Interactive.Type)
			},
		},
		// Test case (b): ≤3 QRs + image/video/document attachment header
		{
			label:                 "2 QRs with image attachment - should use image as header",
			text:                  "Select an option",
			attachments:           []string{"image/jpeg:https://example.com/image.jpg"},
			quickReplies:          []models.QuickReply{{Type: "text", Text: "Option 1"}, {Type: "text", Text: "Option 2"}},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 1,
			expectedType:          "interactive",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				// Should be interactive button with image header
				assert.Equal(t, "interactive", payloads[0].Type)
				assert.Equal(t, "250788123123", payloads[0].To)
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
			label:                 "3 QRs with video attachment - should use video as header",
			text:                  "Watch and choose",
			attachments:           []string{"video/mp4:https://example.com/video.mp4"},
			quickReplies:          []models.QuickReply{{Type: "text", Text: "Like"}, {Type: "text", Text: "Dislike"}, {Type: "text", Text: "Share"}},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 1,
			expectedType:          "interactive",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				// Should be interactive button with video header
				assert.Equal(t, "interactive", payloads[0].Type)
				assert.Equal(t, "250788123123", payloads[0].To)
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
			label:                 "1 QR with document attachment - should use document as header",
			text:                  "Review this",
			attachments:           []string{"application/pdf:https://example.com/document.pdf"},
			quickReplies:          []models.QuickReply{{Type: "text", Text: "Approve"}},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 1,
			expectedType:          "interactive",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				// Should be interactive button with document header
				assert.Equal(t, "interactive", payloads[0].Type)
				assert.Equal(t, "250788123123", payloads[0].To)
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
			label:                 "3 QRs with audio attachment - should NOT use as header, audio not supported",
			text:                  "Listen and respond",
			attachments:           []string{"audio/mp3:https://example.com/audio.mp3"},
			quickReplies:          []models.QuickReply{{Type: "text", Text: "Good"}, {Type: "text", Text: "Bad"}},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 2,
			expectedType:          "audio",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 2, len(payloads))
				// First should be audio (not used as header)
				assert.Equal(t, "audio", payloads[0].Type)
				assert.Equal(t, "250788123123", payloads[0].To)
				assert.NotNil(t, payloads[0].Audio)
				// Second should be interactive button WITHOUT header
				assert.Equal(t, "interactive", payloads[1].Type)
				assert.Equal(t, "button", payloads[1].Interactive.Type)
				assert.Nil(t, payloads[1].Interactive.Header)
			},
		},
		// Test case (c): >10 QRs truncation
		{
			label: "12 QRs - should truncate to 10",
			text:  "Select from many options",
			quickReplies: []models.QuickReply{
				{Type: "text", Text: "Option 1"}, {Type: "text", Text: "Option 2"}, {Type: "text", Text: "Option 3"},
				{Type: "text", Text: "Option 4"}, {Type: "text", Text: "Option 5"}, {Type: "text", Text: "Option 6"},
				{Type: "text", Text: "Option 7"}, {Type: "text", Text: "Option 8"}, {Type: "text", Text: "Option 9"},
				{Type: "text", Text: "Option 10"}, {Type: "text", Text: "Option 11"}, {Type: "text", Text: "Option 12"},
			},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 1,
			expectedType:          "interactive",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				// Should be interactive list with exactly 10 rows
				assert.Equal(t, "interactive", payloads[0].Type)
				assert.Equal(t, "250788123123", payloads[0].To)
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
			label: "15 QRs with Extra - should truncate to 10",
			text:  "Many choices",
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
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 1,
			expectedType:          "interactive",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				assert.Equal(t, "interactive", payloads[0].Type)
				assert.Equal(t, "250788123123", payloads[0].To)
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
			label:                 "4 QRs without Extra - should use list (>3 buttons)",
			text:                  "Pick one",
			quickReplies:          []models.QuickReply{{Type: "text", Text: "A"}, {Type: "text", Text: "B"}, {Type: "text", Text: "C"}, {Type: "text", Text: "D"}},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 1,
			expectedType:          "interactive",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				assert.Equal(t, "interactive", payloads[0].Type)
				assert.Equal(t, "250788123123", payloads[0].To)
				assert.Equal(t, "list", payloads[0].Interactive.Type)
				assert.Equal(t, 4, len(payloads[0].Interactive.Action.Sections[0].Rows))
			},
		},
		{
			label:                 "3 QRs without Extra and no attachment - should use buttons",
			text:                  "Quick choice",
			quickReplies:          []models.QuickReply{{Type: "text", Text: "Yes"}, {Type: "text", Text: "No"}, {Type: "text", Text: "Maybe"}},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 1,
			expectedType:          "interactive",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				assert.Equal(t, "interactive", payloads[0].Type)
				assert.Equal(t, "250788123123", payloads[0].To)
				assert.Equal(t, "button", payloads[0].Interactive.Type)
				assert.Equal(t, 3, len(payloads[0].Interactive.Action.Buttons))
				assert.Nil(t, payloads[0].Interactive.Header)
			},
		},
		{
			label:                 "No quick replies with attachment and text - should have caption",
			text:                  "Check this out",
			attachments:           []string{"image/jpeg:https://example.com/image.jpg"},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 1,
			expectedType:          "image",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				assert.Equal(t, "image", payloads[0].Type)
				assert.Equal(t, "250788123123", payloads[0].To)
				assert.NotNil(t, payloads[0].Image)
				assert.Equal(t, "Check this out", payloads[0].Image.Caption)
			},
		},
		{
			label:                 "Multiple attachments with QRs - first attachment as header, second sent separately",
			text:                  "Multiple files",
			attachments:           []string{"image/jpeg:https://example.com/image1.jpg", "image/jpeg:https://example.com/image2.jpg"},
			quickReplies:          []models.QuickReply{{Type: "text", Text: "Download"}},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 2,
			expectedType:          "image",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 2, len(payloads))
				// Second attachment sent first as standalone
				assert.Equal(t, "image", payloads[0].Type)
				assert.Equal(t, "https://example.com/image2.jpg", payloads[0].Image.Link)
				// Then interactive with first attachment as header
				assert.Equal(t, "interactive", payloads[1].Type)
				assert.Equal(t, "250788123123", payloads[1].To)
				assert.Equal(t, "button", payloads[1].Interactive.Type)
				assert.NotNil(t, payloads[1].Interactive.Header)
				assert.Equal(t, "image", payloads[1].Interactive.Header.Type)
				assert.Equal(t, "https://example.com/image1.jpg", payloads[1].Interactive.Header.Image.Link)
			},
		},
		{
			label:                 "Send message by BSUID",
			text:                  "Hello, BSUID",
			urn:                   "bsuid:US.1234",
			expectedPayloadsCount: 1,
			expectedType:          "text",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				assert.Equal(t, "text", payloads[0].Type)
				assert.Equal(t, "US.1234", payloads[0].Recipient)
				assert.Equal(t, "Hello, BSUID", payloads[0].Text.Body)
			},
		},
		{
			label:                 "Send media and quick replies by BSUID",
			text:                  "Pick an option",
			attachments:           []string{"image/jpeg:https://example.com/image.jpg"},
			quickReplies:          []models.QuickReply{{Type: "text", Text: "Option 1"}, {Type: "text", Text: "Option 2"}},
			urn:                   "bsuid:US.1234",
			expectedPayloadsCount: 1,
			expectedType:          "interactive",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				// every payload builder routes through newBasePayload, so the recipient field must be
				// populated (and to left empty) for media/interactive flows too, not just plain text
				assert.Equal(t, 1, len(payloads))
				assert.Equal(t, "interactive", payloads[0].Type)
				assert.Equal(t, "US.1234", payloads[0].Recipient)
				assert.Empty(t, payloads[0].To)
			},
		},
		{
			label:                 "Form QR - should use flow interactive, other QRs ignored",
			text:                  "Book an appointment",
			quickReplies:          []models.QuickReply{{Type: "form", Text: "Book now", Extra: "123456"}, {Type: "text", Text: "Yes"}},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 1,
			expectedType:          "interactive",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				assert.Equal(t, "interactive", payloads[0].Type)
				assert.Equal(t, "flow", payloads[0].Interactive.Type)
				assert.Equal(t, "Book an appointment", payloads[0].Interactive.Body.Text)
				assert.Equal(t, "flow", payloads[0].Interactive.Action.Name)
				assert.Equal(t, &whatsapp.ActionParameters{FlowMessageVersion: "3", FlowID: "123456", FlowCTA: "Book now"}, payloads[0].Interactive.Action.Parameters)
			},
		},
		{
			label:                 "Form QR without form ID - should be ignored and logged",
			text:                  "Book an appointment",
			quickReplies:          []models.QuickReply{{Type: "form", Text: "Book now"}},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 1,
			expectedType:          "text",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				assert.Equal(t, "text", payloads[0].Type)
				assert.Len(t, clog.Errors, 1)
				assert.Equal(t, "quick reply of type form is missing its extra value and can't be sent", clog.Errors[0].Message)
			},
		},
		{
			label:                 "URL QR - should use cta_url interactive, other QRs ignored",
			text:                  "Check out our site",
			quickReplies:          []models.QuickReply{{Type: "url", Text: "Visit", Extra: "https://example.com"}, {Type: "text", Text: "Yes"}},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 1,
			expectedType:          "interactive",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				assert.Equal(t, "interactive", payloads[0].Type)
				assert.Equal(t, "cta_url", payloads[0].Interactive.Type)
				assert.Equal(t, "Check out our site", payloads[0].Interactive.Body.Text)
				assert.Equal(t, "cta_url", payloads[0].Interactive.Action.Name)
				assert.Equal(t, &whatsapp.ActionParameters{DisplayText: "Visit", URL: "https://example.com"}, payloads[0].Interactive.Action.Parameters)
				// dropped text QR should be logged
				assert.Len(t, clog.Errors, 1)
				assert.Equal(t, "quick reply of type text can't be combined with a url quick reply and won't be sent", clog.Errors[0].Message)
			},
		},
		{
			label:                 "Two URL QRs - only first sent, second logged as dropped",
			text:                  "Check out our sites",
			quickReplies:          []models.QuickReply{{Type: "url", Text: "Visit", Extra: "https://example.com"}, {Type: "url", Text: "Docs", Extra: "https://docs.example.com"}},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 1,
			expectedType:          "interactive",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				assert.Equal(t, &whatsapp.ActionParameters{DisplayText: "Visit", URL: "https://example.com"}, payloads[0].Interactive.Action.Parameters)
				assert.Len(t, clog.Errors, 1)
				assert.Equal(t, "only one quick reply of type url can be sent per message", clog.Errors[0].Message)
			},
		},
		{
			label:                 "Two form QRs - only first sent, second logged as dropped",
			text:                  "Book an appointment",
			quickReplies:          []models.QuickReply{{Type: "form", Text: "Book now", Extra: "111"}, {Type: "form", Text: "Book later", Extra: "222"}},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 1,
			expectedType:          "interactive",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				assert.Equal(t, "flow", payloads[0].Interactive.Type)
				assert.Equal(t, "111", payloads[0].Interactive.Action.Parameters.FlowID)
				assert.Len(t, clog.Errors, 1)
				assert.Equal(t, "only one quick reply of type form can be sent per message", clog.Errors[0].Message)
			},
		},
		{
			label:                 "Mixed QRs with attachment but no text - all QRs logged as dropped",
			text:                  "",
			attachments:           []string{"image/jpeg:https://example.com/image.jpg"},
			quickReplies:          []models.QuickReply{{Type: "url", Text: "Visit", Extra: "https://example.com"}, {Type: "text", Text: "Yes"}},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 1,
			expectedType:          "image",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				assert.Len(t, clog.Errors, 2)
				assert.Equal(t, "quick reply of type url can't be sent on a message with no text", clog.Errors[0].Message)
				assert.Equal(t, "quick reply of type text can't be sent on a message with no text", clog.Errors[1].Message)
			},
		},
		{
			label:                 "URL QR with image attachment - should use image as header of cta_url",
			text:                  "Check out our site",
			attachments:           []string{"image/jpeg:https://example.com/image.jpg"},
			quickReplies:          []models.QuickReply{{Type: "url", Text: "Visit", Extra: "https://example.com"}},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 1,
			expectedType:          "interactive",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				assert.Equal(t, "interactive", payloads[0].Type)
				assert.Equal(t, "cta_url", payloads[0].Interactive.Type)
				assert.Equal(t, "Check out our site", payloads[0].Interactive.Body.Text)
				// Check header
				assert.NotNil(t, payloads[0].Interactive.Header)
				assert.Equal(t, "image", payloads[0].Interactive.Header.Type)
				assert.NotNil(t, payloads[0].Interactive.Header.Image)
				assert.Equal(t, "https://example.com/image.jpg", payloads[0].Interactive.Header.Image.Link)
				assert.Equal(t, &whatsapp.ActionParameters{DisplayText: "Visit", URL: "https://example.com"}, payloads[0].Interactive.Action.Parameters)
			},
		},
		{
			label:                 "URL QR with application attachment - should use document as header of cta_url",
			text:                  "Review this",
			attachments:           []string{"application/pdf:https://example.com/document.pdf"},
			quickReplies:          []models.QuickReply{{Type: "url", Text: "Visit", Extra: "https://example.com"}},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 1,
			expectedType:          "interactive",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				assert.Equal(t, "cta_url", payloads[0].Interactive.Type)
				// Check header
				assert.NotNil(t, payloads[0].Interactive.Header)
				assert.Equal(t, "document", payloads[0].Interactive.Header.Type)
				assert.NotNil(t, payloads[0].Interactive.Header.Document)
				assert.Equal(t, "https://example.com/document.pdf", payloads[0].Interactive.Header.Document.Link)
				assert.Equal(t, "document.pdf", payloads[0].Interactive.Header.Document.Filename)
			},
		},
		{
			label:                 "URL QR with audio attachment - should NOT use as header, audio not supported",
			text:                  "Check out our site",
			attachments:           []string{"audio/mp3:https://example.com/audio.mp3"},
			quickReplies:          []models.QuickReply{{Type: "url", Text: "Visit", Extra: "https://example.com"}},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 2,
			expectedType:          "audio",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 2, len(payloads))
				// First should be audio (not used as header)
				assert.Equal(t, "audio", payloads[0].Type)
				// Second should be cta_url interactive WITHOUT header
				assert.Equal(t, "interactive", payloads[1].Type)
				assert.Equal(t, "cta_url", payloads[1].Interactive.Type)
				assert.Nil(t, payloads[1].Interactive.Header)
			},
		},
		{
			label:                 "URL QR with image attachment but no text - attachment sent standalone, not dropped",
			text:                  "",
			attachments:           []string{"image/jpeg:https://example.com/image.jpg"},
			quickReplies:          []models.QuickReply{{Type: "url", Text: "Visit", Extra: "https://example.com"}},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 1,
			expectedType:          "image",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				assert.Equal(t, "image", payloads[0].Type)
				assert.Equal(t, "https://example.com/image.jpg", payloads[0].Image.Link)
				assert.Len(t, clog.Errors, 1)
				assert.Equal(t, "quick reply of type url can't be sent on a message with no text", clog.Errors[0].Message)
			},
		},
		{
			label:                 "Text QRs with image attachment but no text - attachment sent standalone, not dropped",
			text:                  "",
			attachments:           []string{"image/jpeg:https://example.com/image.jpg"},
			quickReplies:          []models.QuickReply{{Type: "text", Text: "Yes"}, {Type: "text", Text: "No"}},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 1,
			expectedType:          "image",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				assert.Equal(t, "image", payloads[0].Type)
				assert.Equal(t, "https://example.com/image.jpg", payloads[0].Image.Link)
			},
		},
		{
			label:                 "URL QR without URL - should be ignored and logged",
			text:                  "Check out our site",
			quickReplies:          []models.QuickReply{{Type: "url", Text: "Visit"}},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 1,
			expectedType:          "text",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				assert.Equal(t, "text", payloads[0].Type)
				assert.Len(t, clog.Errors, 1)
				assert.Equal(t, "quick reply of type url is missing its extra value and can't be sent", clog.Errors[0].Message)
			},
		},
		{
			label:                 "Unsupported attachment type - should be skipped and logged, text still sent",
			text:                  "Here's a contact",
			attachments:           []string{"text/vcard:https://example.com/contact.vcf"},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 1,
			expectedType:          "text",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				assert.Equal(t, "text", payloads[0].Type)
				assert.Equal(t, "Here's a contact", payloads[0].Text.Body)
				assert.Len(t, clog.Errors, 1)
				assert.Equal(t, "media_unsupported", clog.Errors[0].Code)
			},
		},
		{
			label:                 "Button title over 20 characters - should be truncated and logged",
			text:                  "Pick one",
			quickReplies:          []models.QuickReply{{Type: "text", Text: "This is a very long button title"}, {Type: "text", Text: "Short"}},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 1,
			expectedType:          "interactive",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, "button", payloads[0].Interactive.Type)
				assert.Equal(t, "This is a very lo...", payloads[0].Interactive.Action.Buttons[0].Reply.Title)
				assert.Equal(t, "Short", payloads[0].Interactive.Action.Buttons[1].Reply.Title)
				assert.Len(t, clog.Errors, 1)
				assert.Equal(t, "quick reply text 'This is a very long button title' exceeds the 20 character limit and will be truncated", clog.Errors[0].Message)
			},
		},
		{
			label:                 "List row title and description over limits - should be truncated and logged",
			text:                  "Pick one",
			quickReplies:          []models.QuickReply{{Type: "text", Text: "Option with a very long title", Extra: strings.Repeat("x", 80)}, {Type: "text", Text: "Short", Extra: "ok"}},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 1,
			expectedType:          "interactive",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, "list", payloads[0].Interactive.Type)
				rows := payloads[0].Interactive.Action.Sections[0].Rows
				assert.Equal(t, "Option with a very lo...", rows[0].Title)
				assert.Equal(t, strings.Repeat("x", 69)+"...", rows[0].Description)
				assert.Equal(t, "Short", rows[1].Title)
				assert.Len(t, clog.Errors, 2)
			},
		},
		{
			label:                 "Flow CTA over 30 characters - should be truncated and logged",
			text:                  "Book an appointment",
			quickReplies:          []models.QuickReply{{Type: "form", Text: "Book your appointment now please", Extra: "123456"}},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 1,
			expectedType:          "interactive",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, "flow", payloads[0].Interactive.Type)
				assert.Equal(t, "Book your appointment now p...", payloads[0].Interactive.Action.Parameters.FlowCTA)
				assert.Len(t, clog.Errors, 1)
				assert.Equal(t, "quick reply text 'Book your appointment now please' exceeds the 30 character limit and will be truncated", clog.Errors[0].Message)
			},
		},
		{
			label:                 "Button titles at exactly the limit or with multibyte characters - correct truncation",
			text:                  "Pick one",
			quickReplies:          []models.QuickReply{{Type: "text", Text: "Exactly twenty chars"}, {Type: "text", Text: strings.Repeat("é", 22)}},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 1,
			expectedType:          "interactive",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				// exactly at the limit is left unchanged and not logged
				assert.Equal(t, "Exactly twenty chars", payloads[0].Interactive.Action.Buttons[0].Reply.Title)
				// multibyte text is truncated by runes, not bytes
				assert.Equal(t, strings.Repeat("é", 17)+"...", payloads[0].Interactive.Action.Buttons[1].Reply.Title)
				assert.Len(t, clog.Errors, 1)
			},
		},
		{
			label:                 "URL button display text over 20 characters - should be truncated and logged",
			text:                  "Check out our site",
			quickReplies:          []models.QuickReply{{Type: "url", Text: "Visit our brand new website", Extra: "https://example.com"}},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 1,
			expectedType:          "interactive",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, "cta_url", payloads[0].Interactive.Type)
				assert.Equal(t, "Visit our brand n...", payloads[0].Interactive.Action.Parameters.DisplayText)
				assert.Len(t, clog.Errors, 1)
			},
		},
		{
			label:                 "Text between 1024 and 4096 without attachments or QRs - should be a single text message",
			text:                  strings.Repeat("x", 2000),
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 1,
			expectedType:          "text",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 1, len(payloads))
				assert.Equal(t, 2000, len(payloads[0].Text.Body))
			},
		},
		{
			label:                 "Text between 1024 and 4096 with QRs - should split so interactive body stays within 1024",
			text:                  strings.Repeat("x", 2000),
			quickReplies:          []models.QuickReply{{Type: "text", Text: "Yes"}, {Type: "text", Text: "No"}},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 2,
			expectedType:          "text",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 2, len(payloads))
				assert.Equal(t, "text", payloads[0].Type)
				assert.Equal(t, "interactive", payloads[1].Type)
				assert.LessOrEqual(t, len(payloads[1].Interactive.Body.Text), 1024)
			},
		},
		{
			label:                 "Text between 1024 and 4096 with attachment - should split so nothing exceeds caption limit",
			text:                  strings.Repeat("x", 2000),
			attachments:           []string{"image/jpeg:https://example.com/image.jpg"},
			urn:                   "whatsapp:250788123123",
			expectedPayloadsCount: 3,
			expectedType:          "image",
			checkFunc: func(t *testing.T, payloads []whatsapp.SendRequest, clog *models.ChannelLog) {
				assert.Equal(t, 3, len(payloads))
				assert.Equal(t, "image", payloads[0].Type)
				assert.Equal(t, "", payloads[0].Image.Caption)
				assert.Equal(t, "text", payloads[1].Type)
				assert.Equal(t, "text", payloads[2].Type)
				assert.LessOrEqual(t, len(payloads[1].Text.Body), 1024)
				assert.LessOrEqual(t, len(payloads[2].Text.Body), 1024)
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.label, func(t *testing.T) {
			// Create mock message
			msg := test.NewMockMsg("87995844-2017-4ba0-bc73-f3da75b32f9b", channel, tc.urn, tc.text, tc.attachments)
			msg.QuickReplies_ = tc.quickReplies
			msg.Locale_ = tc.locale

			// Create channel log
			clog := models.NewChannelLogForSend(msg, nil)

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
