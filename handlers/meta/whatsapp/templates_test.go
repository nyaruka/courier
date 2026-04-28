package whatsapp_test

import (
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers/meta/whatsapp"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/stretchr/testify/assert"
)

func TestGetTemplatePayload(t *testing.T) {
	tcs := []struct {
		templating string
		expected   *whatsapp.Template
	}{
		{
			templating: `{
				"template": {"uuid": "4ed5000f-5c94-4143-9697-b7cbd230a381", "name": "Update"},
				"namespace": "12345",
				"language": "en"
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
				"components": [
					{
						"type": "header",
						"name": "header",
						"variables": {"1": 0}
					},
					{
						"type": "body",
						"name": "body",
						"variables": {"1": 1, "2": 2}
					}
				],
				"variables": [
					{"type": "text", "value": "Welcome"},
					{"type": "text", "value": "Hello"},
					{"type": "text", "value": "Bob"}
				]
			}`,
			expected: &whatsapp.Template{
				Name:     "Update",
				Language: &whatsapp.Language{Policy: "deterministic", Code: "en"},
				Components: []*whatsapp.Component{
					{Type: "header", Params: []*whatsapp.Param{{Type: "text", Text: "Welcome"}}},
					{Type: "body", Params: []*whatsapp.Param{{Type: "text", Text: "Hello"}, {Type: "text", Text: "Bob"}}},
				},
			},
		},
		{
			templating: `{
				"template": {"uuid": "4ed5000f-5c94-4143-9697-b7cbd230a381", "name": "Update"},
				"language": "en",
				"components": [
					{
						"type": "header",
						"name": "header",
						"variables": {"1": 0}
					},
					{
						"type": "body",
						"name": "body",
						"variables": {"1": 1, "2": 2}
					}
				],
				"variables": [
					{"type": "image", "value": "image/jpeg:http://example.com/cat2.jpg"},
					{"type": "text", "value": "Hello"},
					{"type": "text", "value": "Bob"}
				]
			}`,
			expected: &whatsapp.Template{
				Name:     "Update",
				Language: &whatsapp.Language{Policy: "deterministic", Code: "en"},
				Components: []*whatsapp.Component{
					{Type: "header", Params: []*whatsapp.Param{{Type: "image", Image: &struct {
						Link string "json:\"link,omitempty\""
					}{Link: "http://example.com/cat2.jpg"}}}},
					{Type: "body", Params: []*whatsapp.Param{{Type: "text", Text: "Hello"}, {Type: "text", Text: "Bob"}}},
				},
			},
		},
		{
			templating: `{
				"template": {"uuid": "4ed5000f-5c94-4143-9697-b7cbd230a381", "name": "Update"},
				"language": "en",
				"components": [
					{
						"type": "header",
						"name": "header",
						"variables": {"1": 0}
					},
					{
						"type": "body",
						"name": "body",
						"variables": {"1": 1, "2": 2}
					}
				],
				"variables": [
					{"type": "video", "value": "video/mp4:http://example.com/video.mp4"},
					{"type": "text", "value": "Hello"},
					{"type": "text", "value": "Bob"}
				]
			}`,
			expected: &whatsapp.Template{
				Name:     "Update",
				Language: &whatsapp.Language{Policy: "deterministic", Code: "en"},
				Components: []*whatsapp.Component{
					{Type: "header", Params: []*whatsapp.Param{{Type: "video", Video: &struct {
						Link string "json:\"link,omitempty\""
					}{Link: "http://example.com/video.mp4"}}}},
					{Type: "body", Params: []*whatsapp.Param{{Type: "text", Text: "Hello"}, {Type: "text", Text: "Bob"}}},
				},
			},
		},
		{
			templating: `{
				"template": {"uuid": "4ed5000f-5c94-4143-9697-b7cbd230a381", "name": "Update"},
				"language": "en",
				"components": [
					{
						"type": "header",
						"name": "header",
						"variables": {"1": 0}
					},
					{
						"type": "body",
						"name": "body",
						"variables": {"1": 1, "2": 2}
					}
				],
				"variables": [
					{"type": "document", "value": "application/pdf:http://example.com/doc.pdf"},
					{"type": "text", "value": "Hello"},
					{"type": "text", "value": "Bob"}
				]
			}`,
			expected: &whatsapp.Template{
				Name:     "Update",
				Language: &whatsapp.Language{Policy: "deterministic", Code: "en"},
				Components: []*whatsapp.Component{
					{Type: "header", Params: []*whatsapp.Param{{Type: "document", Document: &struct {
						Link     string "json:\"link,omitempty\""
						Filename string `json:"filename,omitempty"`
					}{Link: "http://example.com/doc.pdf", Filename: "doc.pdf"}}}},
					{Type: "body", Params: []*whatsapp.Param{{Type: "text", Text: "Hello"}, {Type: "text", Text: "Bob"}}},
				},
			},
		},
		{
			templating: `{
				"template": {"uuid": "4ed5000f-5c94-4143-9697-b7cbd230a381", "name": "Update"},
				"language": "en",
				"components": [
					{
						"type": "header",
						"name": "header",
						"variables": {"1": 0}
					},
					{
						"type": "body",
						"name": "body",
						"variables": {"1": 1, "2": 2}
					}
				],
				"variables": [
					{"type": "document", "value": "document:http://example.com/doc.pdf"},
					{"type": "text", "value": "Hello"},
					{"type": "text", "value": "Bob"}
				]
			}`,
			expected: &whatsapp.Template{
				Name:     "Update",
				Language: &whatsapp.Language{Policy: "deterministic", Code: "en"},
				Components: []*whatsapp.Component{
					{Type: "header", Params: []*whatsapp.Param{{Type: "document", Document: &struct {
						Link     string "json:\"link,omitempty\""
						Filename string `json:"filename,omitempty"`
					}{Link: "http://example.com/doc.pdf", Filename: "doc.pdf"}}}},
					{Type: "body", Params: []*whatsapp.Param{{Type: "text", Text: "Hello"}, {Type: "text", Text: "Bob"}}},
				},
			},
		},
		{
			templating: `{
				"template": {"uuid": "4ed5000f-5c94-4143-9697-b7cbd230a381", "name": "Update"},
				"language": "en",
				"components": [
					{
						"type": "body",
						"name": "body",
						"variables": {"1": 0, "2": 1, "3": 2, "10": 3, "11": 4}
					}
				],
				"variables": [
					{"type": "text", "value": "var1"},
					{"type": "text", "value": "var2"},
					{"type": "text", "value": "var3"},
					{"type": "text", "value": "var10"},
					{"type": "text", "value": "var11"}
				]
			}`,
			expected: &whatsapp.Template{
				Name:     "Update",
				Language: &whatsapp.Language{Policy: "deterministic", Code: "en"},
				Components: []*whatsapp.Component{
					{Type: "body", Params: []*whatsapp.Param{
						{Type: "text", Text: "var1"},
						{Type: "text", Text: "var2"},
						{Type: "text", Text: "var3"},
						{Type: "text", Text: "var10"},
						{Type: "text", Text: "var11"},
					}},
				},
			},
		},
	}

	for i, tc := range tcs {
		templating := &models.Templating{}
		jsonx.MustUnmarshal([]byte(tc.templating), templating)

		msg := test.NewMockMsg("87995844-2017-4ba0-bc73-f3da75b32f9b", nil, "tel:+1234567890", "hi", nil).WithTemplating(templating)
		actual := whatsapp.GetTemplatePayload(msg.Templating())

		assert.Equal(t, tc.expected, actual, "%d: template payload mismatch", i)
	}
}
