package vk

import (
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/jsonx"
)

type Keyboard struct {
	One_Time bool              `json:"one_time"`
	Buttons  [][]ButtonPayload `json:"buttons"`
	Inline   bool              `json:"inline"`
}

type ButtonPayload struct {
	Action ButtonAction `json:"action"`
	Color  string       `json:"color"`
}

type ButtonAction struct {
	Type    string `json:"type"`
	Label   string `json:"label"`
	Payload string `json:"payload"`
}

// NewKeyboardFromReplies creates a keyboard from the given quick replies
func NewKeyboardFromReplies(replies []string) *Keyboard {
	rows := utils.StringsToRows(replies, 10, 30, 2)
	buttons := make([][]ButtonPayload, len(rows))

	for i := range rows {
		buttons[i] = make([]ButtonPayload, len(rows[i]))
		for j := range rows[i] {
			buttons[i][j].Action.Label = rows[i][j]
			buttons[i][j].Action.Type = "text"
			buttons[i][j].Action.Payload = string(jsonx.MustMarshal(rows[i][j]))
			buttons[i][j].Color = "primary"
		}
	}

	return &Keyboard{One_Time: true, Buttons: buttons, Inline: false}
}
