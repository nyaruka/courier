package telegram_test

import (
	"testing"

	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/courier/handlers/telegram"
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
				[][]telegram.KeyboardButton{
					{{Text: "OK"}},
				},
				true, true,
			},
		},
		{
			[]models.QuickReply{{Type: "text", Text: "Yes"}, {Type: "text", Text: "No"}, {Type: "text", Text: "Maybe"}},
			&telegram.ReplyKeyboardMarkup{
				[][]telegram.KeyboardButton{
					{{Text: "Yes"}, {Text: "No"}, {Text: "Maybe"}},
				},
				true, true,
			},
		},
		{
			[]models.QuickReply{{Type: "text", Text: "Vanilla"}, {Type: "text", Text: "Chocolate"}, {Type: "text", Text: "Mint"}, {Type: "text", Text: "Lemon Sorbet"}, {Type: "text", Text: "Papaya"}, {Type: "text", Text: "Strawberry"}},
			&telegram.ReplyKeyboardMarkup{
				[][]telegram.KeyboardButton{
					{{Text: "Vanilla"}, {Text: "Chocolate"}},
					{{Text: "Mint"}, {Text: "Lemon Sorbet"}},
					{{Text: "Papaya"}, {Text: "Strawberry"}},
				},
				true, true,
			},
		},
		{
			[]models.QuickReply{{Type: "text", Text: "A"}, {Type: "text", Text: "B"}, {Type: "text", Text: "C"}, {Type: "text", Text: "D"}, {Type: "text", Text: "Chicken"}, {Type: "text", Text: "Fish"}, {Type: "text", Text: "Peanut Butter Pickle"}},
			&telegram.ReplyKeyboardMarkup{
				[][]telegram.KeyboardButton{
					{{Text: "A"}, {Text: "B"}, {Text: "C"}, {Text: "D"}},
					{{Text: "Chicken"}, {Text: "Fish"}},
					{{Text: "Peanut Butter Pickle"}},
				},
				true, true,
			},
		},
		{

			[]models.QuickReply{{Type: "location"}},
			&telegram.ReplyKeyboardMarkup{
				[][]telegram.KeyboardButton{
					{{Text: "Send Location", RequestLocation: true}},
				},
				true, true,
			},
		},
		{

			[]models.QuickReply{{Type: "location", Text: "Share Location"}},
			&telegram.ReplyKeyboardMarkup{
				[][]telegram.KeyboardButton{
					{{Text: "Share Location", RequestLocation: true}},
				},
				true, true,
			},
		},
	}

	for _, tc := range tcs {
		kb := telegram.NewKeyboardFromReplies(tc.replies)
		assert.Equal(t, tc.expected, kb, "keyboard mismatch for replies %v", tc.replies)
	}
}
