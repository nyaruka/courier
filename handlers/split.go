package handlers

import (
	"bytes"
	"strings"

	"github.com/nyaruka/courier"
	"golang.org/x/exp/slices"
)

type MsgPartType int

const (
	MsgPartTypeText MsgPartType = iota
	MsgPartTypeAttachment
	MsgPartTypeCaptionedAttachment
	MsgPartTypeOptIn
)

// MsgPart represents a message part - either Text or Attachment will be set
type MsgPart struct {
	Type       MsgPartType
	Text       string
	Attachment string
	OptIn      *courier.OptInReference
	IsFirst    bool
	IsLast     bool
}

type SplitOptions struct {
	MaxTextLen    int
	MaxCaptionLen int
	Captionable   []MediaType
}

// SplitMsg splits an outgoing message into separate text and attachment parts, with attachment parts first.
func SplitMsg(m courier.MsgOut, opts SplitOptions) []MsgPart {
	text := m.Text()
	attachments := m.Attachments()

	if m.OptIn() != nil {
		return []MsgPart{{Type: MsgPartTypeOptIn, Text: text, OptIn: m.OptIn(), IsFirst: true, IsLast: true}}
	}

	// if we have a single attachment and text we may be able to combine them into a captioned attachment
	if len(attachments) == 1 && len(text) > 0 && (len(text) <= opts.MaxCaptionLen || opts.MaxCaptionLen == 0) {
		att := attachments[0]
		mediaType, _ := SplitAttachment(att)
		mediaType = strings.Split(mediaType, "/")[0]
		if slices.Contains(opts.Captionable, MediaType(mediaType)) {
			return []MsgPart{{Type: MsgPartTypeCaptionedAttachment, Text: text, Attachment: attachments[0], IsFirst: true, IsLast: true}}
		}
	}

	parts := make([]MsgPart, 0, 5)

	for _, a := range attachments {
		parts = append(parts, MsgPart{Type: MsgPartTypeAttachment, Attachment: a})
	}
	for _, t := range SplitMsgByChannel(m.Channel(), text, opts.MaxTextLen) {
		if len(t) > 0 {
			parts = append(parts, MsgPart{Type: MsgPartTypeText, Text: t})
		}
	}

	if len(parts) > 0 {
		parts[0].IsFirst = true
		parts[len(parts)-1].IsLast = true
	}

	return parts
}

// deprecated use SplitMsg instead
func SplitMsgByChannel(channel courier.Channel, text string, maxLength int) []string {
	max := channel.IntConfigForKey(courier.ConfigMaxLength, maxLength)

	return SplitText(text, max)
}

// SplitText splits the passed in string into segments that are at most max length
func SplitText(text string, max int) []string {
	// smaller than our max, just return it
	if len(text) <= max {
		return []string{text}
	}

	parts := make([]string, 0, 2)
	part := bytes.Buffer{}
	for _, r := range text {
		part.WriteRune(r)
		if part.Len() == max || (part.Len() > max-6 && r == ' ') {
			parts = append(parts, strings.TrimSpace(part.String()))
			part.Reset()
		}
	}
	if part.Len() > 0 {
		parts = append(parts, strings.TrimSpace(part.String()))
	}

	return parts
}
