package whatsapp_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/nyaruka/courier/handlers/meta/whatsapp"
	"github.com/nyaruka/courier/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTemplating(t *testing.T) {
	msg := test.NewMockMsg(1, "87995844-2017-4ba0-bc73-f3da75b32f9b", nil, "tel:+1234567890", "hi", nil)

	// no metadata, no templating
	tpl, err := whatsapp.GetTemplating(msg)
	assert.NoError(t, err)
	assert.Nil(t, tpl)

	msg.WithMetadata(json.RawMessage(`{}`))

	// no templating in metadata, no templating
	tpl, err = whatsapp.GetTemplating(msg)
	assert.NoError(t, err)
	assert.Nil(t, tpl)

	msg.WithMetadata(json.RawMessage(`{"templating": {"foo": "bar"}}`))

	// invalid templating in metadata, error
	tpl, err = whatsapp.GetTemplating(msg)
	assert.Error(t, err, "x")
	assert.Nil(t, tpl)

	msg.WithMetadata(json.RawMessage(`{"templating": {"template": {"uuid": "4ed5000f-5c94-4143-9697-b7cbd230a381", "name": "Update"}}}`))

	// valid templating, no error
	tpl, err = whatsapp.GetTemplating(msg)
	assert.NoError(t, err)
	assert.Equal(t, "4ed5000f-5c94-4143-9697-b7cbd230a381", tpl.Template.UUID)
	assert.Equal(t, "Update", tpl.Template.Name)
}

func TestGetTemplatePayload(t *testing.T) {
	tcs := []struct {
		templating string
		expected   *whatsapp.Template
	}{
		{
			templating: `{
				"template": {"uuid": "4ed5000f-5c94-4143-9697-b7cbd230a381", "name": "Update"},
				"namespace": "12345",
				"language": "en",
				"params": {}
			}`,
			expected: &whatsapp.Template{
				Name:       "Update",
				Language:   &whatsapp.Language{Policy: "deterministic", Code: "en"},
				Components: []*whatsapp.Component{},
			},
		},
		{
			templating: `{
				"template": {"uuid": "4ed5000f-5c94-4143-9697-b7cbd230a381", "name": "Update"},
				"language": "en",
				"params": {
					"header": [{"type": "text", "value": "Welome"}],
					"body": [{"type": "text", "value": "Hello"}, {"type": "text", "value": "Bob"}]
				}
			}`,
			expected: &whatsapp.Template{
				Name:     "Update",
				Language: &whatsapp.Language{Policy: "deterministic", Code: "en"},
				Components: []*whatsapp.Component{
					{Type: "body", Params: []*whatsapp.Param{{Type: "text", Text: "Hello"}, {Type: "text", Text: "Bob"}}},
					{Type: "header", Params: []*whatsapp.Param{{Type: "text", Text: "Welome"}}},
				},
			},
		},
		{
			templating: `{
				"template": {"uuid": "4ed5000f-5c94-4143-9697-b7cbd230a381", "name": "Update"},
				"language": "en",
				"params": {
					"button.1": [{"type": "text", "value": "No"}],
					"button.0": [{"type": "text", "value": "Yes"}, {"type": "text", "value": "Bob"}]
				}
			}`,
			expected: &whatsapp.Template{
				Name:     "Update",
				Language: &whatsapp.Language{Policy: "deterministic", Code: "en"},
				Components: []*whatsapp.Component{
					{Type: "button", SubType: "quick_reply", Index: "0", Params: []*whatsapp.Param{{Type: "payload", Payload: "Yes"}, {Type: "payload", Payload: "Bob"}}},
					{Type: "button", SubType: "quick_reply", Index: "1", Params: []*whatsapp.Param{{Type: "payload", Payload: "No"}}},
				},
			},
		},
	}

	for _, tc := range tcs {
		metadata := json.RawMessage(fmt.Sprintf(`{"templating": %s}`, tc.templating))
		msg := test.NewMockMsg(1, "87995844-2017-4ba0-bc73-f3da75b32f9b", nil, "tel:+1234567890", "hi", nil).WithMetadata(metadata)
		templating, err := whatsapp.GetTemplating(msg)
		require.NoError(t, err)

		actual := whatsapp.GetTemplatePayload(templating)

		assert.Equal(t, tc.expected, actual)
	}
}
