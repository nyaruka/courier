package telegram_test

import (
	"testing"

	"github.com/nyaruka/courier/handlers/telegram"
	"github.com/stretchr/testify/assert"
)

func TestKeyboardFromReplies(t *testing.T) {
	tcs := []struct {
		replies  []string
		expected *telegram.ReplyKeyboardMarkup
	}{
		{

			[]string{"OK"},
			&telegram.ReplyKeyboardMarkup{
				[][]telegram.KeyboardButton{
					{{Text: "OK"}},
				},
				true, true,
			},
		},
		{
			[]string{"Yes", "No", "Maybe"},
			&telegram.ReplyKeyboardMarkup{
				[][]telegram.KeyboardButton{
					{{Text: "Yes"}, {Text: "No"}, {Text: "Maybe"}},
				},
				true, true,
			},
		},
		{
			[]string{"Vanilla", "Chocolate", "Mint", "Lemon Sorbet", "Papaya", "Strawberry"},
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
			[]string{"A", "B", "C", "D", "Chicken", "Fish", "Peanut Butter Pickle"},
			&telegram.ReplyKeyboardMarkup{
				[][]telegram.KeyboardButton{
					{{Text: "A"}, {Text: "B"}, {Text: "C"}, {Text: "D"}},
					{{Text: "Chicken"}, {Text: "Fish"}},
					{{Text: "Peanut Butter Pickle"}},
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
