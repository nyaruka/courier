package vk_test

import (
	"testing"

	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/courier/handlers/vk"
	"github.com/stretchr/testify/assert"
)

func TestKeyboardFromReplies(t *testing.T) {
	tcs := []struct {
		replies  []models.QuickReply
		expected *vk.Keyboard
	}{
		{

			[]models.QuickReply{{Type: "text", Text: "OK"}},
			&vk.Keyboard{
				true,
				[][]vk.ButtonPayload{
					{
						{vk.ButtonAction{Type: "text", Label: "OK", Payload: "\"OK\""}, "primary"},
					},
				},
				false,
			},
		},
		{
			[]models.QuickReply{{Type: "text", Text: "Yes"}, {Type: "text", Text: "No"}, {Type: "text", Text: "Maybe"}},
			&vk.Keyboard{
				true,
				[][]vk.ButtonPayload{
					{
						{vk.ButtonAction{Type: "text", Label: "Yes", Payload: "\"Yes\""}, "primary"},
						{vk.ButtonAction{Type: "text", Label: "No", Payload: "\"No\""}, "primary"},
						{vk.ButtonAction{Type: "text", Label: "Maybe", Payload: "\"Maybe\""}, "primary"},
					},
				},
				false,
			},
		},
		{
			[]models.QuickReply{{Type: "text", Text: "Vanilla"}, {Type: "text", Text: "Chocolate"}, {Type: "text", Text: "Mint"}, {Type: "text", Text: "Lemon Sorbet"}, {Type: "text", Text: "Papaya"}, {Type: "text", Text: "Strawberry"}},
			&vk.Keyboard{
				true,
				[][]vk.ButtonPayload{

					{{vk.ButtonAction{Type: "text", Label: "Vanilla", Payload: "\"Vanilla\""}, "primary"}},
					{{vk.ButtonAction{Type: "text", Label: "Chocolate", Payload: "\"Chocolate\""}, "primary"}},
					{{vk.ButtonAction{Type: "text", Label: "Mint", Payload: "\"Mint\""}, "primary"}},
					{{vk.ButtonAction{Type: "text", Label: "Lemon Sorbet", Payload: "\"Lemon Sorbet\""}, "primary"}},
					{{vk.ButtonAction{Type: "text", Label: "Papaya", Payload: "\"Papaya\""}, "primary"}},
					{{vk.ButtonAction{Type: "text", Label: "Strawberry", Payload: "\"Strawberry\""}, "primary"}},
				},
				false,
			},
		},
		{
			[]models.QuickReply{{Type: "text", Text: "A"}, {Type: "text", Text: "B"}, {Type: "text", Text: "C"}, {Type: "text", Text: "D"}, {Type: "text", Text: "Chicken"}, {Type: "text", Text: "Fish"}, {Type: "text", Text: "Peanut Butter Pickle"}},
			&vk.Keyboard{
				true,
				[][]vk.ButtonPayload{

					{{vk.ButtonAction{Type: "text", Label: "A", Payload: "\"A\""}, "primary"}},
					{{vk.ButtonAction{Type: "text", Label: "B", Payload: "\"B\""}, "primary"}},
					{{vk.ButtonAction{Type: "text", Label: "C", Payload: "\"C\""}, "primary"}},
					{{vk.ButtonAction{Type: "text", Label: "D", Payload: "\"D\""}, "primary"}},
					{{vk.ButtonAction{Type: "text", Label: "Chicken", Payload: "\"Chicken\""}, "primary"}},
					{{vk.ButtonAction{Type: "text", Label: "Fish", Payload: "\"Fish\""}, "primary"}},
					{{vk.ButtonAction{Type: "text", Label: "Peanut Butter Pickle", Payload: "\"Peanut Butter Pickle\""}, "primary"}},
				},
				false,
			},
		},
		{
			[]models.QuickReply{{Type: "text", Text: "A"}, {Type: "text", Text: "B"}, {Type: "text", Text: "C"}, {Type: "text", Text: "D"}, {Type: "text", Text: "E"}},
			&vk.Keyboard{
				true,
				[][]vk.ButtonPayload{

					{
						{vk.ButtonAction{Type: "text", Label: "A", Payload: "\"A\""}, "primary"},
						{vk.ButtonAction{Type: "text", Label: "B", Payload: "\"B\""}, "primary"},
						{vk.ButtonAction{Type: "text", Label: "C", Payload: "\"C\""}, "primary"},
						{vk.ButtonAction{Type: "text", Label: "D", Payload: "\"D\""}, "primary"},
						{vk.ButtonAction{Type: "text", Label: "E", Payload: "\"E\""}, "primary"},
					},
				},
				false,
			},
		},
	}

	for _, tc := range tcs {
		kb := vk.NewKeyboardFromReplies(tc.replies)
		assert.Equal(t, tc.expected, kb, "keyboard mismatch for replies %v", tc.replies)
	}
}
