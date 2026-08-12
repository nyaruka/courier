package whatsapp

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/courier/v26/utils"
	"github.com/nyaruka/gocommon/stringsx"
	"github.com/nyaruka/gocommon/svclogs"
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

// character limits for quick reply texts rendered in interactive messages
const (
	maxButtonTextLength   = 20 // reply button titles and CTA URL display text
	maxFlowCTALength      = 30 // flow button text
	maxListRowTitleLength = 24
	maxListRowDescLength  = 72
)

// truncateQuickReplyText truncates the given quick reply text to the given character limit, logging a channel
// error if it was too long to be sent as is.
func truncateQuickReplyText(clog *models.ChannelLog, text string, limit int) string {
	truncated := stringsx.TruncateEllipsis(text, limit)
	if truncated != text {
		clog.Error(&svclogs.Error{Message: fmt.Sprintf("quick reply text '%s' exceeds the %d character limit and will be truncated", text, limit)})
	}
	return truncated
}

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

	// log any quick replies that will be dropped - because interactive messages require body text, or because a
	// different kind of quick reply takes precedence on this message
	if len(sqrs) > 0 && len(msgParts) == 0 {
		for _, qr := range sqrs {
			clog.Error(&svclogs.Error{Message: fmt.Sprintf("quick reply of type %s can't be sent on a message with no text", qr.Type)})
		}
	} else {
		logDroppedQuickReplies(clog, locationQRs, formQRs, urlQRs, qrs)
	}

	qrsAsList := shouldUseList(qrs)

	// truncate quick replies to max 10 - only relevant if text quick replies are what will be rendered
	if len(locationQRs) == 0 && len(formQRs) == 0 && len(urlQRs) == 0 && len(qrs) > 10 {
		clog.Error(&svclogs.Error{Message: "too many quick replies WhatsApp supports only up to 10 quick replies"})
		qrs = qrs[:10]
	}

	var payloads []SendRequest

	// determine if the attachment can be used as a header in an interactive message - button (max 3, not shown as a
	// list) and CTA URL messages support media headers, but interactive messages require body text so without text
	// the attachment has to be sent as a standalone media message
	headerCapable := len(urlQRs) > 0 || (len(qrs) > 0 && len(qrs) <= 3 && !qrsAsList)
	hasHeaderAttachment := false
	if len(msg.Attachments()) > 0 && len(msgParts) > 0 && headerCapable && len(locationQRs) == 0 && len(formQRs) == 0 {
		attType, _ := handlers.SplitAttachment(msg.Attachments()[0])
		attType = strings.Split(attType, "/")[0]
		// only certain media types can be used as an interactive header (application/* is sent as a document)
		if attType == "image" || attType == "video" || attType == "document" || attType == "application" {
			hasHeaderAttachment = true
		}
	}

	// 1. send attachments that need to go as standalone media messages
	for i, att := range msg.Attachments() {
		if hasHeaderAttachment && i == 0 {
			continue // this attachment will be used as a header below
		}

		caption := ""
		contentType, _ := handlers.SplitAttachment(att)
		attType := strings.Split(contentType, "/")[0]

		// skip attachment types that can't be sent as media messages
		if attType != "image" && attType != "audio" && attType != "video" && attType != "application" && attType != "document" {
			clog.Error(models.ErrorMediaUnsupported(contentType))
			continue
		}

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
			payloads = append(payloads, buildFlowPayload(msg, part, formQRs[0], clog))

		case isLastPart && len(urlQRs) > 0:
			p, err := buildCTAURLPayload(msg, part, urlQRs[0], hasHeaderAttachment, clog)
			if err != nil {
				return nil, err
			}
			payloads = append(payloads, p)

		case isLastPart && len(qrs) > 0 && !qrsAsList:
			p, err := buildButtonPayload(msg, part, qrs, hasHeaderAttachment, clog)
			if err != nil {
				return nil, err
			}
			payloads = append(payloads, p)

		case isLastPart && len(qrs) > 0 && qrsAsList:
			payloads = append(payloads, buildListPayload(msg, part, qrs, menuButton, clog))

		default:
			payloads = append(payloads, buildTextPayload(msg, part))
		}
	}
	return payloads, nil
}

// logDroppedQuickReplies logs a channel error for each quick reply that won't be sent because a message can only
// render one kind of quick reply - location, form, url or text in order of precedence - and at most one form or
// url button. Extra location quick replies aren't logged since they all collapse into the same location request.
func logDroppedQuickReplies(clog *models.ChannelLog, locationQRs, formQRs, urlQRs, textQRs []models.QuickReply) {
	var winning string
	var dropped []models.QuickReply

	switch {
	case len(locationQRs) > 0:
		winning = models.QuickReplyTypeLocation
		dropped = slices.Concat(formQRs, urlQRs, textQRs)
	case len(formQRs) > 0:
		winning = models.QuickReplyTypeForm
		dropped = slices.Concat(formQRs[1:], urlQRs, textQRs)
	case len(urlQRs) > 0:
		winning = models.QuickReplyTypeURL
		dropped = slices.Concat(urlQRs[1:], textQRs)
	default:
		return
	}

	for _, qr := range dropped {
		if qr.Type == winning {
			clog.Error(&svclogs.Error{Message: fmt.Sprintf("only one quick reply of type %s can be sent per message", qr.Type)})
		} else {
			clog.Error(&svclogs.Error{Message: fmt.Sprintf("quick reply of type %s can't be combined with a %s quick reply and won't be sent", qr.Type, winning)})
		}
	}
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

func buildFlowPayload(msg *models.MsgOut, body string, qr models.QuickReply, clog *models.ChannelLog) SendRequest {
	p := newBasePayload(msg)
	p.Type = "interactive"
	interactive := Interactive{Type: "flow", Body: struct {
		Text string `json:"text"`
	}{Text: body}}
	interactive.Action = &Action{
		Name:       "flow",
		Parameters: &ActionParameters{FlowMessageVersion: "3", FlowID: qr.Extra, FlowCTA: truncateQuickReplyText(clog, qr.GetText(), maxFlowCTALength)},
	}
	p.Interactive = &interactive
	return p
}

func buildCTAURLPayload(msg *models.MsgOut, body string, qr models.QuickReply, useAttachmentHeader bool, clog *models.ChannelLog) (SendRequest, error) {
	p := newBasePayload(msg)
	p.Type = "interactive"
	interactive := Interactive{Type: "cta_url", Body: struct {
		Text string `json:"text"`
	}{Text: body}}

	if useAttachmentHeader {
		header, err := buildAttachmentHeader(msg)
		if err != nil {
			return SendRequest{}, err
		}
		interactive.Header = header
	}

	interactive.Action = &Action{
		Name:       "cta_url",
		Parameters: &ActionParameters{DisplayText: truncateQuickReplyText(clog, qr.GetText(), maxButtonTextLength), URL: qr.Extra},
	}
	p.Interactive = &interactive
	return p, nil
}

func buildButtons(qrs []models.QuickReply, clog *models.ChannelLog) []Button {
	btns := make([]Button, len(qrs))
	for i, qr := range qrs {
		btns[i] = Button{Type: "reply"}
		btns[i].Reply.ID = fmt.Sprint(i)
		btns[i].Reply.Title = truncateQuickReplyText(clog, qr.Text, maxButtonTextLength)
	}
	return btns
}

func buildButtonPayload(msg *models.MsgOut, body string, qrs []models.QuickReply, useAttachmentHeader bool, clog *models.ChannelLog) (SendRequest, error) {
	p := newBasePayload(msg)
	p.Type = "interactive"

	interactive := Interactive{Type: "button", Body: struct {
		Text string `json:"text"`
	}{Text: body}}

	if useAttachmentHeader {
		header, err := buildAttachmentHeader(msg)
		if err != nil {
			return SendRequest{}, err
		}
		interactive.Header = header
	}

	interactive.Action = &Action{Buttons: buildButtons(qrs, clog)}
	p.Interactive = &interactive
	return p, nil
}

// buildAttachmentHeader creates an interactive message header from the message's first attachment
func buildAttachmentHeader(msg *models.MsgOut) (*Header, error) {
	attType, attURL := handlers.SplitAttachment(msg.Attachments()[0])
	attType = strings.Split(attType, "/")[0]
	if attType == "application" {
		attType = "document"
	}

	switch attType {
	case "image":
		return &Header{Type: "image", Image: &Media{Link: attURL}}, nil
	case "video":
		return &Header{Type: "video", Video: &Media{Link: attURL}}, nil
	case "document":
		filename, err := utils.BasePathForURL(attURL)
		if err != nil {
			return nil, err
		}
		return &Header{Type: "document", Document: &Media{Link: attURL, Filename: filename}}, nil
	}
	return nil, nil
}

func buildListPayload(msg *models.MsgOut, body string, qrs []models.QuickReply, menuButton string, clog *models.ChannelLog) SendRequest {
	p := newBasePayload(msg)
	p.Type = "interactive"

	interactive := Interactive{Type: "list", Body: struct {
		Text string `json:"text"`
	}{Text: body}}

	section := Section{Rows: make([]SectionRow, len(qrs))}
	for i, qr := range qrs {
		section.Rows[i] = SectionRow{
			ID:          fmt.Sprint(i),
			Title:       truncateQuickReplyText(clog, qr.Text, maxListRowTitleLength),
			Description: truncateQuickReplyText(clog, qr.Extra, maxListRowDescLength),
		}
	}

	interactive.Action = &Action{Button: menuButton, Sections: []Section{section}}
	p.Interactive = &interactive
	return p
}
