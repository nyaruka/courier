package viber

import (
	"fmt"
	"html"
	"strings"
	"unicode/utf8"
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
	// maxColumns refer to the maximum number of columns units in a row
	maxColumns = 6
	// maxRowsRunes refer to maximum number of runes in the same row to help define buttons layout
	maxRowRunes = 38
	// paddingRunes to help to calculate the size of button column width
	paddingRunes = 2
)

var textSizes = map[string]bool{"small": true, "regular": true, "large": true}

// NewKeyboardFromReplies create a keyboard from the given quick replies
func NewKeyboardFromReplies(replies []string, buttonConfig map[string]any) *Keyboard {
	rows := StringsToRows(replies, maxColumns, maxRowRunes, paddingRunes)
	buttons := []KeyboardButton{}

	for i := range rows {
		for j := range rows[i] {
			cols := 6 / len(rows[i])

			button := KeyboardButton{
				ActionType: "reply",
				TextSize:   "regular",
				ActionBody: rows[i][j],
				Text:       html.EscapeString(rows[i][j]),
				Columns:    fmt.Sprint(cols),
			}

			button.ApplyConfig(buttonConfig)
			buttons = append(buttons, button)
		}
	}

	return &Keyboard{"keyboard", false, buttons}
}

// ApplyConfig apply the configs from the channel to KeyboardButton
func (b *KeyboardButton) ApplyConfig(buttonConfig map[string]any) {
	bgColor := strings.TrimSpace(fmt.Sprint(buttonConfig["bg_color"]))
	textStyle := strings.TrimSpace(fmt.Sprint(buttonConfig["text"]))
	textSize := strings.TrimSpace(fmt.Sprint(buttonConfig["text_size"]))

	if len(bgColor) == 7 {
		b.BgColor = bgColor
	}

	if strings.Contains(textStyle, "*") {
		b.Text = strings.Replace(textStyle, "*", b.Text, 1)
	}
	if textSizes[textSize] {
		b.TextSize = textSize
	}
}

// StringsToRows takes a slice of strings and re-organize it into rows and columns
func StringsToRows(strs []string, maxColumns, maxRowRunes, paddingRunes int) [][]string {
	rows := [][]string{{}}
	curRow := 0
	rowRunes := 0

	colsByRow := []int{6, 3, 2, 1}
	i := 0

	for len(strs) > 0 {
		if len(strs) >= colsByRow[i] {
			rowRunes = 0
			for _, str := range strs[:colsByRow[i]] {
				rowRunes += utf8.RuneCountInString(str) + paddingRunes*2
			}
			if rowRunes <= maxRowRunes || colsByRow[i] == 1 {
				strsCopy := make([]string, colsByRow[i])
				copy(strsCopy, strs[:colsByRow[i]])
				for _, str := range strsCopy {
					rows[curRow] = append(rows[curRow], str)
					strs = append(strs[:0], strs[0+1:]...)
				}
				if len(strs) > 0 {
					rows = append(rows, []string{})
					curRow += 1
					i = 0
				}
			}
		}
		i++
		if i >= len(colsByRow) {
			i = 0
		}
	}
	return rows
}
