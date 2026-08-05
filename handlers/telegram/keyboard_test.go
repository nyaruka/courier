package telegram_test

import (
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers/telegram"
	"github.com/stretchr/testify/assert"
)

func TestKeyboardFromReplies(t *testing.T) {
	tcs := []struct {
		replies  []models.QuickReply
		expected *telegram.ReplyKeyboardMarkup
	}{
		{

			[]models.QuickReply{{Type: "text", Text: "OK"}},
			&telegram.ReplyKeyboardMarkup{
				Keyboard: [][]telegram.KeyboardButton{
					{{Text: "OK"}},
				},
				ResizeKeyboard:  true,
				OneTimeKeyboard: true,
			},
		},
		{
			[]models.QuickReply{{Type: "text", Text: "Yes"}, {Type: "text", Text: "No"}, {Type: "text", Text: "Maybe"}},
			&telegram.ReplyKeyboardMarkup{
				Keyboard: [][]telegram.KeyboardButton{
					{{Text: "Yes"}, {Text: "No"}, {Text: "Maybe"}},
				},
				ResizeKeyboard:  true,
				OneTimeKeyboard: true,
			},
		},
		{
			[]models.QuickReply{{Type: "text", Text: "Vanilla"}, {Type: "text", Text: "Chocolate"}, {Type: "text", Text: "Mint"}, {Type: "text", Text: "Lemon Sorbet"}, {Type: "text", Text: "Papaya"}, {Type: "text", Text: "Strawberry"}},
			&telegram.ReplyKeyboardMarkup{
				Keyboard: [][]telegram.KeyboardButton{
					{{Text: "Vanilla"}, {Text: "Chocolate"}},
					{{Text: "Mint"}, {Text: "Lemon Sorbet"}},
					{{Text: "Papaya"}, {Text: "Strawberry"}},
				},
				ResizeKeyboard:  true,
				OneTimeKeyboard: true,
			},
		},
		{
			[]models.QuickReply{{Type: "text", Text: "A"}, {Type: "text", Text: "B"}, {Type: "text", Text: "C"}, {Type: "text", Text: "D"}, {Type: "text", Text: "Chicken"}, {Type: "text", Text: "Fish"}, {Type: "text", Text: "Peanut Butter Pickle"}},
			&telegram.ReplyKeyboardMarkup{
				Keyboard: [][]telegram.KeyboardButton{
					{{Text: "A"}, {Text: "B"}, {Text: "C"}, {Text: "D"}},
					{{Text: "Chicken"}, {Text: "Fish"}},
					{{Text: "Peanut Butter Pickle"}},
				},
				ResizeKeyboard:  true,
				OneTimeKeyboard: true,
			},
		},
		{

			[]models.QuickReply{{Type: "location"}},
			&telegram.ReplyKeyboardMarkup{
				Keyboard: [][]telegram.KeyboardButton{
					{{Text: "Send Location", RequestLocation: true}},
				},
				ResizeKeyboard:  true,
				OneTimeKeyboard: false,
			},
		},
		{

			[]models.QuickReply{{Type: "location", Text: "Share Location"}},
			&telegram.ReplyKeyboardMarkup{
				Keyboard: [][]telegram.KeyboardButton{
					{{Text: "Share Location", RequestLocation: true}},
				},
				ResizeKeyboard:  true,
				OneTimeKeyboard: false,
			},
		},
		{

			[]models.QuickReply{{Type: "form", Extra: "https://example.com/form"}, {Type: "text", Text: "Skip"}},
			&telegram.ReplyKeyboardMarkup{
				Keyboard: [][]telegram.KeyboardButton{
					{{Text: "Open Form", WebApp: &telegram.WebAppInfo{URL: "https://example.com/form"}}, {Text: "Skip"}},
				},
				ResizeKeyboard:  true,
				OneTimeKeyboard: false,
			},
		},
	}

	for _, tc := range tcs {
		kb := telegram.NewKeyboardFromReplies(tc.replies)
		assert.Equal(t, tc.expected, kb, "keyboard mismatch for replies %v", tc.replies)
	}
}
