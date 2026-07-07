package whatsapp

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/nyaruka/courier/v26"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/courier/v26/utils"
	"github.com/nyaruka/courier/v26/utils/clogs"
	"github.com/nyaruka/gocommon/urns"
)

func GetMsgPayloads(ctx context.Context, msg courier.MsgOut, maxMsgLength int, clog *courier.ChannelLog) ([]SendRequest, error) {
	if msg.Templating() != nil {
		return []SendRequest{newBasePayload(msg).withTemplate(msg.Templating())}, nil
	}
	return buildContentPayloads(msg, maxMsgLength, clog)
}

// allDigitsRegex matches a WhatsApp identity that is a phone number rather than a business-scoped user ID.
var allDigitsRegex = regexp.MustCompile(`^[0-9]+$`)

// RecipientFields returns the to and recipient field values for the given URN. The recipient field is used only
// for business-scoped user IDs (a whatsapp URN whose path is not all digits, e.g. "US.1234"); phone numbers go
// in the to field. A whatsapp URN may carry a phone number too (legacy contacts stored the sender phone as a
// whatsapp URN with all digits), so we route those to the to field just like tel URNs.
func RecipientFields(urn urns.URN) (to, recipient string) {
	path := urn.Path()
	if urn.Scheme() == urns.WhatsApp.Prefix && !allDigitsRegex.MatchString(path) {
		return "", path
	}
	// tel URNs (and legacy all-digit whatsapp URNs) carry a phone number; the WhatsApp API doesn't want the leading +
	return strings.TrimPrefix(path, "+"), ""
}

// newBasePayload creates a SendRequest with common fields populated.
func newBasePayload(msg courier.MsgOut) SendRequest {
	request := SendRequest{MessagingProduct: "whatsapp", RecipientType: "individual"}
	request.To, request.Recipient = RecipientFields(msg.URN())
	return request
}

func (p SendRequest) withTemplate(templating *models.Templating) SendRequest {
	p.Type = "template"
	p.Template = GetTemplatePayload(templating)
	return p
}

// buildContentPayloads constructs payloads for a non-template message with text, attachments, and quick replies.
func buildContentPayloads(msg courier.MsgOut, maxMsgLength int, clog *courier.ChannelLog) ([]SendRequest, error) {
	msgParts := splitText(msg, maxMsgLength)
	qrs := handlers.FilterQuickRepliesByType(msg.QuickReplies(), "text")
	locationQRs := handlers.FilterQuickRepliesByType(msg.QuickReplies(), "location")
	menuButton := handlers.GetText("Menu", msg.Locale())

	qrsAsList := shouldUseList(qrs)

	// truncate quick replies to max 10
	if len(qrs) > 10 {
		clog.Error(&clogs.Error{Message: "too many quick replies WhatsApp supports only up to 10 quick replies"})
		qrs = qrs[:10]
	}

	var payloads []SendRequest

	// determine if the attachment can be used as a header in an interactive message
	hasHeaderAttachment := false
	if len(msg.Attachments()) > 0 && len(qrs) > 0 && len(qrs) <= 3 && len(locationQRs) == 0 && !qrsAsList {
		attType, _ := handlers.SplitAttachment(msg.Attachments()[0])
		attType = strings.Split(attType, "/")[0]
		// only certain media types can be used as an interactive header
		if attType == "image" || attType == "video" || attType == "document" {
			hasHeaderAttachment = true
		}
	}

	// 1. send attachments that need to go as standalone media messages
	for i, att := range msg.Attachments() {
		if hasHeaderAttachment && i == 0 {
			continue // this attachment will be used as a header below
		}

		caption := ""
		attType, _ := handlers.SplitAttachment(att)
		attType = strings.Split(attType, "/")[0]

		// only non-audio single attachment messages can have captions
		if attType != "audio" && len(msgParts) == 1 && len(msg.Attachments()) == 1 && len(qrs) == 0 && len(locationQRs) == 0 {
			caption = msgParts[0]
		}

		p, err := buildMediaPayload(msg, i, caption)
		if err != nil {
			return nil, err
		}
		payloads = append(payloads, p)

		if caption != "" {
			return payloads, nil // text was used as caption, we're done
		}
	}

	// 2. send text parts
	for i, part := range msgParts {
		isLastPart := i == len(msgParts)-1

		switch {
		case isLastPart && len(locationQRs) > 0:
			payloads = append(payloads, buildLocationRequestPayload(msg, part))

		case isLastPart && len(qrs) > 0 && !qrsAsList:
			ps, err := buildButtonPayload(msg, part, qrs, hasHeaderAttachment)
			if err != nil {
				return nil, err
			}
			payloads = append(payloads, ps...)

		case isLastPart && len(qrs) > 0 && qrsAsList:
			payloads = append(payloads, buildListPayload(msg, part, qrs, menuButton))

		default:
			payloads = append(payloads, buildTextPayload(msg, part))
		}
	}
	return payloads, nil
}

func splitText(msg courier.MsgOut, maxMsgLength int) []string {
	if msg.Text() != "" {
		return handlers.SplitMsgByChannel(msg.Channel(), msg.Text(), maxMsgLength)
	}
	return nil
}

func shouldUseList(qrs []models.QuickReply) bool {
	for i, qr := range qrs {
		if i > 2 || qr.Extra != "" {
			return true
		}
	}
	return false
}

func hasURLPreview(text string) bool {
	return strings.Contains(text, "https://") || strings.Contains(text, "http://")
}

func buildTextPayload(msg courier.MsgOut, body string) SendRequest {
	p := newBasePayload(msg)
	p.Type = "text"
	p.Text = &Text{Body: body, PreviewURL: hasURLPreview(body)}
	return p
}

func buildMediaPayload(msg courier.MsgOut, attachmentIdx int, caption string) (SendRequest, error) {
	p := newBasePayload(msg)
	attType, attURL := handlers.SplitAttachment(msg.Attachments()[attachmentIdx])
	attType = strings.Split(attType, "/")[0]
	if attType == "application" {
		attType = "document"
	}
	p.Type = attType
	media := Media{Link: attURL, Caption: caption}

	switch attType {
	case "image":
		p.Image = &media
	case "audio":
		p.Audio = &media
	case "video":
		p.Video = &media
	case "document":
		filename, err := utils.BasePathForURL(attURL)
		if err != nil {
			filename = ""
		}
		media.Filename = filename
		p.Document = &media
	}
	return p, nil
}

func buildLocationRequestPayload(msg courier.MsgOut, body string) SendRequest {
	p := newBasePayload(msg)
	p.Type = "interactive"
	interactive := Interactive{Type: "location_request_message", Body: struct {
		Text string `json:"text"`
	}{Text: body}}
	interactive.Action = &struct {
		Name     string    `json:"name,omitempty"`
		Button   string    `json:"button,omitempty"`
		Sections []Section `json:"sections,omitempty"`
		Buttons  []Button  `json:"buttons,omitempty"`
	}{Name: "send_location"}
	p.Interactive = &interactive
	return p
}

func buildButtons(qrs []models.QuickReply) []Button {
	btns := make([]Button, len(qrs))
	for i, qr := range qrs {
		btns[i] = Button{Type: "reply"}
		btns[i].Reply.ID = fmt.Sprint(i)
		btns[i].Reply.Title = qr.Text
	}
	return btns
}

func buildButtonPayload(msg courier.MsgOut, body string, qrs []models.QuickReply, useAttachmentHeader bool) ([]SendRequest, error) {
	var payloads []SendRequest
	p := newBasePayload(msg)
	p.Type = "interactive"

	interactive := Interactive{Type: "button", Body: struct {
		Text string `json:"text"`
	}{Text: body}}

	if useAttachmentHeader {
		attType, attURL := handlers.SplitAttachment(msg.Attachments()[0])
		attType = strings.Split(attType, "/")[0]
		if attType == "application" {
			attType = "document"
		}

		switch attType {
		case "image":
			interactive.Header = &struct {
				Type     string `json:"type"`
				Text     string `json:"text,omitempty"`
				Video    *Media `json:"video,omitempty"`
				Image    *Media `json:"image,omitempty"`
				Document *Media `json:"document,omitempty"`
			}{Type: "image", Image: &Media{Link: attURL}}
		case "video":
			interactive.Header = &struct {
				Type     string `json:"type"`
				Text     string `json:"text,omitempty"`
				Video    *Media `json:"video,omitempty"`
				Image    *Media `json:"image,omitempty"`
				Document *Media `json:"document,omitempty"`
			}{Type: "video", Video: &Media{Link: attURL}}
		case "document":
			filename, err := utils.BasePathForURL(attURL)
			if err != nil {
				return nil, err
			}
			interactive.Header = &struct {
				Type     string `json:"type"`
				Text     string `json:"text,omitempty"`
				Video    *Media `json:"video,omitempty"`
				Image    *Media `json:"image,omitempty"`
				Document *Media `json:"document,omitempty"`
			}{Type: "document", Document: &Media{Link: attURL, Filename: filename}}
		}
	}

	interactive.Action = &struct {
		Name     string    `json:"name,omitempty"`
		Button   string    `json:"button,omitempty"`
		Sections []Section `json:"sections,omitempty"`
		Buttons  []Button  `json:"buttons,omitempty"`
	}{Buttons: buildButtons(qrs)}
	p.Interactive = &interactive
	payloads = append(payloads, p)
	return payloads, nil
}

func buildListPayload(msg courier.MsgOut, body string, qrs []models.QuickReply, menuButton string) SendRequest {
	p := newBasePayload(msg)
	p.Type = "interactive"

	interactive := Interactive{Type: "list", Body: struct {
		Text string `json:"text"`
	}{Text: body}}

	section := Section{Rows: make([]SectionRow, len(qrs))}
	for i, qr := range qrs {
		section.Rows[i] = SectionRow{
			ID:          fmt.Sprint(i),
			Title:       qr.Text,
			Description: qr.Extra,
		}
	}

	interactive.Action = &struct {
		Name     string    `json:"name,omitempty"`
		Button   string    `json:"button,omitempty"`
		Sections []Section `json:"sections,omitempty"`
		Buttons  []Button  `json:"buttons,omitempty"`
	}{Button: menuButton, Sections: []Section{section}}
	p.Interactive = &interactive
	return p
}
