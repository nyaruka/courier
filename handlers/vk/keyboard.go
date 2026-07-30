package vk

import (
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/gocommon/jsonx"
)

type Keyboard struct {
	One_Time bool              `json:"one_time"`
	Buttons  [][]ButtonPayload `json:"buttons"`
	Inline   bool              `json:"inline"`
}

type ButtonPayload struct {
	Action ButtonAction `json:"action"`
	Color  string       `json:"color,omitempty"`
}

type ButtonAction struct {
	Type    string `json:"type"`
	Label   string `json:"label,omitempty"`
	Link    string `json:"link,omitempty"`
	Payload string `json:"payload"`
}

// NewKeyboardFromReplies creates a keyboard from the given quick replies
func NewKeyboardFromReplies(replies []models.QuickReply) *Keyboard {
	rows := models.QuickRepliesToRows(replies, 10, 30, 2)
	buttons := make([][]ButtonPayload, len(rows))

	for i := range rows {
		buttons[i] = make([]ButtonPayload, len(rows[i]))
		for j := range rows[i] {
			qr := rows[i][j]
			buttons[i][j].Action.Payload = string(jsonx.MustMarshal(qr.GetText()))

			switch qr.Type {
			case models.QuickReplyTypeLocation:
				// location buttons carry no label or color and always take up the whole row width
				buttons[i][j].Action.Type = "location"
			case models.QuickReplyTypeURL:
				buttons[i][j].Action.Type = "open_link"
				buttons[i][j].Action.Label = qr.GetText()
				buttons[i][j].Action.Link = qr.Extra
				buttons[i][j].Color = "primary"
			default:
				buttons[i][j].Action.Type = "text"
				buttons[i][j].Action.Label = qr.GetText()
				buttons[i][j].Color = "primary"
			}
		}
	}

	return &Keyboard{One_Time: true, Buttons: buttons, Inline: false}
}
