package vk_test

import (
	"testing"

	"github.com/nyaruka/courier/handlers/vk"
	"github.com/stretchr/testify/assert"
)

func TestKeyboardFromReplies(t *testing.T) {
	tcs := []struct {
		replies  []string
		expected *vk.Keyboard
	}{
		{

			[]string{"OK"},
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
			[]string{"Yes", "No", "Maybe"},
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
			[]string{"Vanilla", "Chocolate", "Mint", "Lemon Sorbet", "Papaya", "Strawberry"},
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
			[]string{"A", "B", "C", "D", "Chicken", "Fish", "Peanut Butter Pickle"},
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
			[]string{"A", "B", "C", "D", "E"},
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
