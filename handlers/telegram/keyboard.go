package telegram

import (
	"github.com/nyaruka/courier/v26/core/models"
)

// WebAppInfo describes a Mini App to be launched, see https://core.telegram.org/bots/api/#webappinfo
type WebAppInfo struct {
	URL string `json:"url"`
}

// KeyboardButton is button on a keyboard, see https://core.telegram.org/bots/api/#keyboardbutton
type KeyboardButton struct {
	Text            string      `json:"text"`
	RequestContact  bool        `json:"request_contact,omitempty"`
	RequestLocation bool        `json:"request_location,omitempty"`
	WebApp          *WebAppInfo `json:"web_app,omitempty"`
}

// ReplyKeyboardMarkup models a keyboard, see https://core.telegram.org/bots/api/#replykeyboardmarkup
type ReplyKeyboardMarkup struct {
	Keyboard        [][]KeyboardButton `json:"keyboard"`
	IsPersistent    bool               `json:"is_persistent,omitempty"`
	ResizeKeyboard  bool               `json:"resize_keyboard"`
	OneTimeKeyboard bool               `json:"one_time_keyboard"`
}

// NewKeyboardFromReplies creates a keyboard from the given quick replies
func NewKeyboardFromReplies(replies []models.QuickReply) *ReplyKeyboardMarkup {
	rows := models.QuickRepliesToRows(replies, 5, 30, 2)
	keyboard := make([][]KeyboardButton, len(rows))
	hasForm, hasLocation := false, false

	for i := range rows {
		keyboard[i] = make([]KeyboardButton, len(rows[i]))
		for j := range rows[i] {
			keyboard[i][j].Text = rows[i][j].GetText()

			// a button can specify at most one of these optional fields, but each reply is its own button so text,
			// location and form replies can all appear on the same keyboard
			switch rows[i][j].Type {
			case models.QuickReplyTypeLocation:
				keyboard[i][j].RequestLocation = true
				hasLocation = true
			case models.QuickReplyTypeForm:
				keyboard[i][j].WebApp = &WebAppInfo{URL: rows[i][j].Extra}
				hasForm = true
			}
		}
	}

	// tapping a text button is itself the reply so those keyboards can be single use, but form and location buttons
	// launch a separate interaction that the user might not complete, and one-time state syncs across their devices -
	// so keep those keyboards available (and pinned open, for forms) until the next message clears them
	return &ReplyKeyboardMarkup{
		Keyboard:        keyboard,
		IsPersistent:    hasForm,
		ResizeKeyboard:  true,
		OneTimeKeyboard: !hasForm && !hasLocation,
	}
}
