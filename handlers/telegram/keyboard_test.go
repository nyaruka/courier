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
				true, false,
			},
		},
		{

			[]models.QuickReply{{Type: "location", Text: "Share Location"}},
			&telegram.ReplyKeyboardMarkup{
				[][]telegram.KeyboardButton{
					{{Text: "Share Location", RequestLocation: true}},
				},
				true, false,
			},
		},
		{

			[]models.QuickReply{{Type: "form", Extra: "https://example.com/form"}, {Type: "text", Text: "Skip"}},
			&telegram.ReplyKeyboardMarkup{
				[][]telegram.KeyboardButton{
					{{Text: "Open Form", WebApp: &telegram.WebAppInfo{URL: "https://example.com/form"}}, {Text: "Skip"}},
				},
				true, false,
			},
		},
	}

	for _, tc := range tcs {
		kb := telegram.NewKeyboardFromReplies(tc.replies)
		assert.Equal(t, tc.expected, kb, "keyboard mismatch for replies %v", tc.replies)
	}
}

func TestInlineKeyboardFromReplies(t *testing.T) {
	tcs := []struct {
		replies  []models.QuickReply
		expected *telegram.InlineKeyboardMarkup
	}{
		{
			[]models.QuickReply{{Type: "url", Extra: "https://example.com"}, {Type: "url", Text: "Read More", Extra: "https://example.com/more"}},
			&telegram.InlineKeyboardMarkup{
				InlineKeyboard: [][]telegram.InlineKeyboardButton{
					{{Text: "Open Link", URL: "https://example.com"}, {Text: "Read More", URL: "https://example.com/more"}},
				},
			},
		},
		{
			[]models.QuickReply{{Type: "url", Text: "Visit Us", Extra: "https://example.com/visit"}},
			&telegram.InlineKeyboardMarkup{
				InlineKeyboard: [][]telegram.InlineKeyboardButton{
					{{Text: "Visit Us", URL: "https://example.com/visit"}},
				},
			},
		},
		{
			[]models.QuickReply{{Type: "url", Text: "Visit our website", Extra: "https://example.com"}, {Type: "url", Text: "Read the latest news", Extra: "https://example.com/news"}},
			&telegram.InlineKeyboardMarkup{
				InlineKeyboard: [][]telegram.InlineKeyboardButton{
					{{Text: "Visit our website", URL: "https://example.com"}},
					{{Text: "Read the latest news", URL: "https://example.com/news"}},
				},
			},
		},
	}

	for _, tc := range tcs {
		kb := telegram.NewInlineKeyboardFromReplies(tc.replies)
		assert.Equal(t, tc.expected, kb, "keyboard mismatch for replies %v", tc.replies)
	}
}
