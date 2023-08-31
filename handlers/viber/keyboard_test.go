package viber_test

import (
	"testing"

	"github.com/nyaruka/courier/handlers/viber"
	"github.com/stretchr/testify/assert"
)

func TestKeyboardFromReplies(t *testing.T) {
	tsc := []struct {
		replies      []string
		expected     *viber.Keyboard
		buttonConfig map[string]any
	}{
		{
			[]string{"OK"},
			&viber.Keyboard{
				"keyboard",
				false,
				[]viber.KeyboardButton{
					{ActionType: "reply", TextSize: "regular", ActionBody: "OK", Text: "OK", Columns: "6"},
				},
			},
			map[string]any{},
		},
		{
			[]string{"Yes", "No", "Maybe"},
			&viber.Keyboard{
				"keyboard",
				false,
				[]viber.KeyboardButton{
					{ActionType: "reply", TextSize: "regular", ActionBody: "Yes", Text: "Yes", Columns: "2"},
					{ActionType: "reply", TextSize: "regular", ActionBody: "No", Text: "No", Columns: "2"},
					{ActionType: "reply", TextSize: "regular", ActionBody: "Maybe", Text: "Maybe", Columns: "2"},
				},
			},
			map[string]any{},
		},
		{
			[]string{"A", "B", "C", "D"},
			&viber.Keyboard{
				"keyboard",
				false,
				[]viber.KeyboardButton{
					{ActionType: "reply", TextSize: "regular", ActionBody: "A", Text: "A", Columns: "2"},
					{ActionType: "reply", TextSize: "regular", ActionBody: "B", Text: "B", Columns: "2"},
					{ActionType: "reply", TextSize: "regular", ActionBody: "C", Text: "C", Columns: "2"},
					{ActionType: "reply", TextSize: "regular", ActionBody: "D", Text: "D", Columns: "6"},
				},
			},
			map[string]any{},
		},
		{
			[]string{"\"A\"", "<B>"},
			&viber.Keyboard{
				"keyboard",
				false,
				[]viber.KeyboardButton{
					{ActionType: "reply", TextSize: "regular", ActionBody: "\"A\"", Text: "&#34;A&#34;", Columns: "3"},
					{ActionType: "reply", TextSize: "regular", ActionBody: "<B>", Text: "&lt;B&gt;", Columns: "3"},
				},
			},
			map[string]any{},
		},
		{
			[]string{"Vanilla", "Chocolate", "Mint", "Lemon Sorbet", "Papaya", "Strawberry"},
			&viber.Keyboard{
				"keyboard",
				false,
				[]viber.KeyboardButton{
					{ActionType: "reply", TextSize: "regular", ActionBody: "Vanilla", Text: "Vanilla", Columns: "2"},
					{ActionType: "reply", TextSize: "regular", ActionBody: "Chocolate", Text: "Chocolate", Columns: "2"},
					{ActionType: "reply", TextSize: "regular", ActionBody: "Mint", Text: "Mint", Columns: "2"},
					{ActionType: "reply", TextSize: "regular", ActionBody: "Lemon Sorbet", Text: "Lemon Sorbet", Columns: "3"},
					{ActionType: "reply", TextSize: "regular", ActionBody: "Papaya", Text: "Papaya", Columns: "3"},
					{ActionType: "reply", TextSize: "regular", ActionBody: "Strawberry", Text: "Strawberry", Columns: "6"},
				},
			},
			map[string]any{},
		},
		{
			[]string{"A", "B", "C", "D", "Chicken", "Fish", "Peanut Butter Pickle"},
			&viber.Keyboard{
				"keyboard",
				false,
				[]viber.KeyboardButton{
					{ActionType: "reply", TextSize: "regular", ActionBody: "A", Text: "A", Columns: "2"},
					{ActionType: "reply", TextSize: "regular", ActionBody: "B", Text: "B", Columns: "2"},
					{ActionType: "reply", TextSize: "regular", ActionBody: "C", Text: "C", Columns: "2"},
					{ActionType: "reply", TextSize: "regular", ActionBody: "D", Text: "D", Columns: "2"},
					{ActionType: "reply", TextSize: "regular", ActionBody: "Chicken", Text: "Chicken", Columns: "2"},
					{ActionType: "reply", TextSize: "regular", ActionBody: "Fish", Text: "Fish", Columns: "2"},
					{ActionType: "reply", TextSize: "regular", ActionBody: "Peanut Butter Pickle", Text: "Peanut Butter Pickle", Columns: "6"},
				},
			},
			map[string]any{},
		},
		{
			[]string{"Foo", "Bar", "Baz"},
			&viber.Keyboard{
				"keyboard",
				false,
				[]viber.KeyboardButton{
					{ActionType: "reply", TextSize: "large", ActionBody: "Foo", Text: "<font color=\"#FFFFFF\">Foo</font><br>", Columns: "2", BgColor: "#f7bb3f"},
					{ActionType: "reply", TextSize: "large", ActionBody: "Bar", Text: "<font color=\"#FFFFFF\">Bar</font><br>", Columns: "2", BgColor: "#f7bb3f"},
					{ActionType: "reply", TextSize: "large", ActionBody: "Baz", Text: "<font color=\"#FFFFFF\">Baz</font><br>", Columns: "2", BgColor: "#f7bb3f"},
				},
			},
			map[string]any{
				"bg_color":  "#f7bb3f",
				"text":      "<font color=\"#FFFFFF\">*</font><br>",
				"text_size": "large",
			},
		},
		{
			[]string{"Yes", "No", "Maybe"},
			&viber.Keyboard{
				"keyboard",
				false,
				[]viber.KeyboardButton{
					{ActionType: "reply", TextSize: "small", ActionBody: "Yes", Text: "<font color=\"#0066FF\"><b>Yes</b></font><br>", Columns: "2"},
					{ActionType: "reply", TextSize: "small", ActionBody: "No", Text: "<font color=\"#0066FF\"><b>No</b></font><br>", Columns: "2"},
					{ActionType: "reply", TextSize: "small", ActionBody: "Maybe", Text: "<font color=\"#0066FF\"><b>Maybe</b></font><br>", Columns: "2"},
				},
			},
			map[string]any{
				"text":      "<font color=\"#0066FF\"><b>*</b></font><br>",
				"text_size": "small",
			},
		},
	}

	for _, tc := range tsc {
		kb := viber.NewKeyboardFromReplies(tc.replies, tc.buttonConfig)
		assert.Equal(t, tc.expected, kb, "keyboard mismatch for replies %v", tc.replies)
	}
}
