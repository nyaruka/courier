package models_test

import (
	"encoding/json"
	"testing"

	"github.com/nyaruka/courier/core/models"
	"github.com/stretchr/testify/assert"
)

func TestMsgOut(t *testing.T) {
	msgJSON := `{
		"attachments": ["https://foo.bar/image.jpg"],
		"quick_replies": [{"type": "text", "text": "Yes"}, {"type": "text", "text": "No"}, {"text": "Maybe"}],
		"text": "Test message 21",
		"contact": {"id": 100, "uuid": "a984069d-0008-4d8c-a772-b14a8a6acccc"},
		"flow": {"uuid": "9de3663f-c5c5-4c92-9f45-ecbc09abcc85", "name": "Favorites"},
		"id": 204,
		"channel_uuid": "f3ad3eb6-d00d-4dc3-92e9-9f34f32940ba",
		"uuid": "54c893b9-b026-44fc-a490-50aed0361c3f",
		"urn": "telegram:3527065",
		"urn_auth": "5ApPVsFDcFt:RZdK9ne7LgfvBYdtCYg7tv99hC9P2",
		"org_id": 1,
		"origin": "flow",
		"created_on": "2017-07-21T19:22:23.242757Z",
		"high_priority": true,
		"response_to_external_id": "external-id",
		"is_resend": true
	}`

	msg := &models.MsgOut{}
	err := json.Unmarshal([]byte(msgJSON), msg)
	assert.NoError(t, err)
	assert.Equal(t, models.ContactID(100), msg.Contact_.ID)
	assert.Equal(t, models.ContactUUID("a984069d-0008-4d8c-a772-b14a8a6acccc"), msg.Contact_.UUID)
	assert.Equal(t, models.ChannelUUID("f3ad3eb6-d00d-4dc3-92e9-9f34f32940ba"), msg.ChannelUUID_)
	assert.Equal(t, []string{"https://foo.bar/image.jpg"}, msg.Attachments())
	assert.Equal(t, "5ApPVsFDcFt:RZdK9ne7LgfvBYdtCYg7tv99hC9P2", msg.URNAuth_)
	assert.Equal(t, []models.QuickReply{{Type: "text", Text: "Yes"}, {Type: "text", Text: "No"}, {Type: "text", Text: "Maybe"}}, msg.QuickReplies())
	assert.Equal(t, "external-id", msg.ResponseToExternalID())
	assert.True(t, msg.HighPriority())
	assert.True(t, msg.IsResend())
	assert.Equal(t, &models.FlowReference{UUID: "9de3663f-c5c5-4c92-9f45-ecbc09abcc85", Name: "Favorites"}, msg.Flow())

	msgJSONNoQR := `{
		"text": "Test message 21",
		"contact": {"id": 100, "uuid": "a984069d-0008-4d8c-a772-b14a8a6acccc"},
		"id": 204,
		"channel_uuid": "f3ad3eb6-d00d-4dc3-92e9-9f34f32940ba",
		"uuid": "54c893b9-b026-44fc-a490-50aed0361c3f",
		"urn": "telegram:3527065",
		"org_id": 1,
		"origin": "chat",
		"created_on": "2017-07-21T19:22:23.242757Z",
		"high_priority": true,
		"response_to_external_id": null,
		"metadata": null
	}`

	msg = &models.MsgOut{}
	err = json.Unmarshal([]byte(msgJSONNoQR), msg)
	assert.NoError(t, err)
	assert.Nil(t, msg.Attachments())
	assert.Nil(t, msg.QuickReplies())
	assert.Equal(t, "", msg.ResponseToExternalID())
	assert.False(t, msg.IsResend())
	assert.Nil(t, msg.Flow())
}

func TestQuickReplyValidation(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid text quick reply",
			json:        `{"type": "text", "text": "Yes"}`,
			expectError: false,
		},
		{
			name:        "text quick reply with empty text field",
			json:        `{"type": "text", "text": ""}`,
			expectError: true,
			errorMsg:    "text field is required when type is 'text'",
		},
		{
			name:        "text quick reply without text field",
			json:        `{"type": "text"}`,
			expectError: true,
			errorMsg:    "text field is required when type is 'text'",
		},
		{
			name:        "non-text quick reply without text field is valid",
			json:        `{"type": "location"}`,
			expectError: false,
		},
		{
			name:        "quick reply defaults to text type",
			json:        `{"text": "Maybe"}`,
			expectError: false,
		},
		{
			name:        "quick reply defaults to text type without text field fails",
			json:        `{}`,
			expectError: true,
			errorMsg:    "text field is required when type is 'text'",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			qr := &models.QuickReply{}
			err := json.Unmarshal([]byte(tc.json), qr)

			if tc.expectError {
				assert.Error(t, err)
				if tc.errorMsg != "" {
					assert.Contains(t, err.Error(), tc.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
