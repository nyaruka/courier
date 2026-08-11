package whatsapp

import (
	"context"
	"fmt"
	"strings"

	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/courier/v26/utils"
	"github.com/nyaruka/courier/v26/utils/clogs"
	"github.com/nyaruka/gocommon/urns"
)

func GetMsgPayloads(ctx context.Context, msg *models.MsgOut, maxMsgLength int, clog *models.ChannelLog) ([]SendRequest, error) {
	if msg.Templating() != nil {
		return []SendRequest{newBasePayload(msg).withTemplate(msg.Templating())}, nil
	}
	return buildContentPayloads(msg, maxMsgLength, clog)
}

// RecipientFields returns the to and recipient field values for the given URN. A business-scoped user ID -
// a whatsapp URN in the CC.xxx form or a bsuid URN - goes in the recipient field; a phone number goes in the
// to field.
func RecipientFields(urn urns.URN) (to, recipient string) {
	if urn.Scheme() == urns.BSUID.Prefix || urns.IsWhatsAppBSUID(urn) {
		return "", urn.Path()
	}
	return urn.Path(), ""
}

// newBasePayload creates a SendRequest with common fields populated.
func newBasePayload(msg *models.MsgOut) SendRequest {
	request := SendRequest{MessagingProduct: "whatsapp", RecipientType: "individual"}
	request.To, request.Recipient = RecipientFields(msg.URN())
	return request
}

func (p SendRequest) withTemplate(templating *models.Templating) SendRequest {
	p.Type = "template"
	p.Template = GetTemplatePayload(templating)
	return p
}

// maxCaptionAndBodyLength is the limit for media captions and interactive message bodies, which is lower than the
// limit for plain text bodies
const maxCaptionAndBodyLength = 1024

// buildContentPayloads constructs payloads for a non-template message with text, attachments, and quick replies.
func buildContentPayloads(msg *models.MsgOut, maxMsgLength int, clog *models.ChannelLog) ([]SendRequest, error) {
	sqrs := handlers.FilterSupportedQuickReplies(msg.QuickReplies(), clog, models.QuickReplyTypeText, models.QuickReplyTypeLocation, models.QuickReplyTypeForm, models.QuickReplyTypeURL)
	qrs := handlers.FilterQuickRepliesByType(sqrs, models.QuickReplyTypeText)
	locationQRs := handlers.FilterQuickRepliesByType(sqrs, models.QuickReplyTypeLocation)
	formQRs := handlers.FilterQuickRepliesByType(sqrs, models.QuickReplyTypeForm)
	urlQRs := handlers.FilterQuickRepliesByType(sqrs, models.QuickReplyTypeURL)
	menuButton := handlers.GetText("Menu", msg.Locale())

	// if text could end up as a media caption or an interactive message body, use their lower length limit
	if len(msg.Attachments()) > 0 || len(qrs) > 0 || len(locationQRs) > 0 || len(formQRs) > 0 || len(urlQRs) > 0 {
		maxMsgLength = min(maxMsgLength, maxCaptionAndBodyLength)
	}

	msgParts := splitText(msg, maxMsgLength)

	qrsAsList := shouldUseList(qrs)

	// truncate quick replies to max 10
	if len(qrs) > 10 {
		clog.Error(&clogs.Error{Message: "too many quick replies WhatsApp supports only up to 10 quick replies"})
		qrs = qrs[:10]
	}

	var payloads []SendRequest

	// determine if the attachment can be used as a header in an interactive message
	hasHeaderAttachment := false
	if len(msg.Attachments()) > 0 && len(qrs) > 0 && len(qrs) <= 3 && len(locationQRs) == 0 && len(formQRs) == 0 && len(urlQRs) == 0 && !qrsAsList {
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
		if attType != "audio" && len(msgParts) == 1 && len(msg.Attachments()) == 1 && len(qrs) == 0 && len(locationQRs) == 0 && len(formQRs) == 0 && len(urlQRs) == 0 {
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

		case isLastPart && len(formQRs) > 0:
			payloads = append(payloads, buildFlowPayload(msg, part, formQRs[0]))

		case isLastPart && len(urlQRs) > 0:
			payloads = append(payloads, buildCTAURLPayload(msg, part, urlQRs[0]))

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

func splitText(msg *models.MsgOut, maxMsgLength int) []string {
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

func buildTextPayload(msg *models.MsgOut, body string) SendRequest {
	p := newBasePayload(msg)
	p.Type = "text"
	p.Text = &Text{Body: body, PreviewURL: hasURLPreview(body)}
	return p
}

func buildMediaPayload(msg *models.MsgOut, attachmentIdx int, caption string) (SendRequest, error) {
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

func buildLocationRequestPayload(msg *models.MsgOut, body string) SendRequest {
	p := newBasePayload(msg)
	p.Type = "interactive"
	interactive := Interactive{Type: "location_request_message", Body: struct {
		Text string `json:"text"`
	}{Text: body}}
	interactive.Action = &Action{Name: "send_location"}
	p.Interactive = &interactive
	return p
}

func buildFlowPayload(msg *models.MsgOut, body string, qr models.QuickReply) SendRequest {
	p := newBasePayload(msg)
	p.Type = "interactive"
	interactive := Interactive{Type: "flow", Body: struct {
		Text string `json:"text"`
	}{Text: body}}
	interactive.Action = &Action{
		Name:       "flow",
		Parameters: &ActionParameters{FlowMessageVersion: "3", FlowID: qr.Extra, FlowCTA: qr.GetText()},
	}
	p.Interactive = &interactive
	return p
}

func buildCTAURLPayload(msg *models.MsgOut, body string, qr models.QuickReply) SendRequest {
	p := newBasePayload(msg)
	p.Type = "interactive"
	interactive := Interactive{Type: "cta_url", Body: struct {
		Text string `json:"text"`
	}{Text: body}}
	interactive.Action = &Action{
		Name:       "cta_url",
		Parameters: &ActionParameters{DisplayText: qr.GetText(), URL: qr.Extra},
	}
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

func buildButtonPayload(msg *models.MsgOut, body string, qrs []models.QuickReply, useAttachmentHeader bool) ([]SendRequest, error) {
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
			interactive.Header = &Header{Type: "image", Image: &Media{Link: attURL}}
		case "video":
			interactive.Header = &Header{Type: "video", Video: &Media{Link: attURL}}
		case "document":
			filename, err := utils.BasePathForURL(attURL)
			if err != nil {
				return nil, err
			}
			interactive.Header = &Header{Type: "document", Document: &Media{Link: attURL, Filename: filename}}
		}
	}

	interactive.Action = &Action{Buttons: buildButtons(qrs)}
	p.Interactive = &interactive
	payloads = append(payloads, p)
	return payloads, nil
}

func buildListPayload(msg *models.MsgOut, body string, qrs []models.QuickReply, menuButton string) SendRequest {
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

	interactive.Action = &Action{Button: menuButton, Sections: []Section{section}}
	p.Interactive = &interactive
	return p
}
