package telegram

import "github.com/nyaruka/courier/utils"

// KeyboardButton is button on a keyboard, see https://core.telegram.org/bots/api/#keyboardbutton
type KeyboardButton struct {
	Text            string `json:"text"`
	RequestContact  bool   `json:"request_contact,omitempty"`
	RequestLocation bool   `json:"request_location,omitempty"`
}

// ReplyKeyboardMarkup models a keyboard, see https://core.telegram.org/bots/api/#replykeyboardmarkup
type ReplyKeyboardMarkup struct {
	Keyboard        [][]KeyboardButton `json:"keyboard"`
	ResizeKeyboard  bool               `json:"resize_keyboard"`
	OneTimeKeyboard bool               `json:"one_time_keyboard"`
}

// NewKeyboardFromReplies creates a keyboard from the given quick replies
func NewKeyboardFromReplies(replies []string) *ReplyKeyboardMarkup {
	rows := utils.StringsToRows(replies, 5, 30, 2)
	keyboard := make([][]KeyboardButton, len(rows))

	for i := range rows {
		keyboard[i] = make([]KeyboardButton, len(rows[i]))
		for j := range rows[i] {
			keyboard[i][j].Text = rows[i][j]
		}
	}

	return &ReplyKeyboardMarkup{Keyboard: keyboard, ResizeKeyboard: true, OneTimeKeyboard: true}
}
