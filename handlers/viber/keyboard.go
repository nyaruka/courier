package viber

import (
	"fmt"
	"strings"

	"github.com/nyaruka/courier/utils"
)

// KeyboardButton is button on a keyboard, see https://developers.viber.com/docs/tools/keyboards/#buttons-parameters
type KeyboardButton struct {
	ActionType string `json:"ActionType"`
	ActionBody string `json:"ActionBody"`
	Text       string `json:"Text"`
	TextSize   string `json:"TextSize"`
	Columns    string `json:"Columns,omitempty"`
	BgColor    string `json:"BgColor,omitempty"`
}

// Keyboard models a keyboard, see https://developers.viber.com/docs/tools/keyboards/#general-keyboard-parameters
type Keyboard struct {
	Type          string           `json:"Type"`
	DefaultHeight bool             `json:"DefaultHeight"`
	Buttons       []KeyboardButton `json:"Buttons"`
}

const (
	// maxRows refer to the maximum number of rows visible without scrolling
	maxRows = 4
	// maxRowsRunes refer to maximum number of runes in the same row to help define buttons layout
	maxRowRunes = 30
	// paddingRunes to help to calculate the size of button column width
	paddingRunes = 2
)

// NewKeyboardFromReplies create a keyboard from the given quick replies
func NewKeyboardFromReplies(replies []string, buttonConfig map[string]interface{}) *Keyboard {
	rows := utils.StringsToRows(replies, maxRows, maxRowRunes, paddingRunes)
	buttons := []KeyboardButton{}

	for i := range rows {
		for j := range rows[i] {
			var cols int
			switch len(rows[i]) {
			case 1:
				cols = 6
			case 2:
				cols = 3
			case 3:
				cols = 2
			case 4:
				cols = 3
			case 5:
				if j < 3 {
					cols = 2
				} else {
					cols = 3
				}
			default:
				cols = 6
			}

			button := KeyboardButton{
				ActionType: "reply",
				TextSize:   "regular",
				ActionBody: rows[i][j],
				Text:       rows[i][j],
				Columns:    fmt.Sprint(cols),
			}

			button.ApplyConfig(buttonConfig)
			buttons = append(buttons, button)
		}
	}

	return &Keyboard{"keyboard", false, buttons}
}

//ApplyConfig apply the configs from the channel to KeyboardButton
func (b *KeyboardButton) ApplyConfig(buttonConfig map[string]interface{}) {
	bgColor := strings.TrimSpace(fmt.Sprint(buttonConfig["bg_color"]))
	textStyle := strings.TrimSpace(fmt.Sprint(buttonConfig["text"]))
	textSize := strings.TrimSpace(fmt.Sprint(buttonConfig["text_size"]))

	if len(bgColor) == 7 {
		b.BgColor = bgColor
	}
	if strings.Contains(textStyle, "*") {
		b.Text = strings.Replace(textStyle, "*", b.Text, 1)
	}
	if map[string]bool{"small": true, "large": true}[textSize] {
		b.TextSize = textSize
	}
}
