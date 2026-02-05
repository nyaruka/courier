package whatsapp

import (
	"context"
	"fmt"
	"strings"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/courier/utils/clogs"
)

func GetMsgPayloads(ctx context.Context, msg courier.MsgOut, maxMsgLength int, clog *courier.ChannelLog) ([]SendRequest, error) {
	if msg.Templating() != nil {
		return buildTemplatePayload(msg), nil
	}
	return buildRegularPayloads(msg, maxMsgLength, clog)
}

func buildTemplatePayload(msg courier.MsgOut) []SendRequest {
	payload := SendRequest{
		MessagingProduct: "whatsapp",
		RecipientType:    "individual",
		To:               msg.URN().Path(),
		Type:             "template",
		Template:         GetTemplatePayload(msg.Templating()),
	}
	return []SendRequest{payload}
}

func buildRegularPayloads(msg courier.MsgOut, maxMsgLength int, clog *courier.ChannelLog) ([]SendRequest, error) {
	msgParts := getMsgParts(msg, maxMsgLength)
	qrs := handlers.FilterQuickRepliesByType(msg.QuickReplies(), "text")
	locationQRs := handlers.FilterQuickRepliesByType(msg.QuickReplies(), "location")

	qrsAsList := shouldUseQRsList(qrs)
	menuButton := handlers.GetText("Menu", msg.Locale())

	requestPayloads := make([]SendRequest, 0)
	hasCaption := false

	totalItems := len(msgParts) + len(msg.Attachments())

	// Send audio attachments separately when there are also quick replies
	if len(msg.Attachments()) > 0 && len(locationQRs) == 0 && len(qrs) > 0 {
		for i := 0; i < len(msg.Attachments()); i++ {
			attType, attURL := handlers.SplitAttachment(msg.Attachments()[i])
			attType = normalizeAttachmentType(attType)

			if attType == "audio" {
				totalItems--
				payloadAudio := SendRequest{MessagingProduct: "whatsapp", RecipientType: "individual", To: msg.URN().Path(), Type: "audio", Audio: &Media{Link: attURL}}
				requestPayloads = append(requestPayloads, payloadAudio)
			}
		}
	}

	for i := 0; i < totalItems; i++ {
		payload := SendRequest{
			MessagingProduct: "whatsapp",
			RecipientType:    "individual",
			To:               msg.URN().Path(),
		}

		if i < len(msg.Attachments()) && (len(qrs) == 0 || len(qrs) > 3 || len(locationQRs) > 0) {
			err := buildAttachmentPayload(&payload, msg, msgParts, i, &hasCaption)
			if err != nil {
				return nil, err
			}
		} else {
			err := buildTextPayload(&payload, msg, msgParts, qrs, locationQRs, qrsAsList, menuButton, i, clog, &hasCaption)
			if err != nil {
				return nil, err
			}
		}

		requestPayloads = append(requestPayloads, payload)

		if hasCaption {
			break
		}
	}

	return requestPayloads, nil
}

func getMsgParts(msg courier.MsgOut, maxMsgLength int) []string {
	if msg.Text() == "" {
		return []string{}
	}
	return handlers.SplitMsgByChannel(msg.Channel(), msg.Text(), maxMsgLength)
}

func shouldUseQRsList(qrs []models.QuickReply) bool {
	for i, qr := range qrs {
		if i > 2 || qr.Extra != "" {
			return true
		}
	}
	return false
}

func buildAttachmentPayload(payload *SendRequest, msg courier.MsgOut, msgParts []string, i int, hasCaption *bool) error {
	attType, attURL := handlers.SplitAttachment(msg.Attachments()[i])
	attType = normalizeAttachmentType(attType)

	payload.Type = attType
	media := Media{Link: attURL}

	if shouldAddCaption(msgParts, attType, msg) {
		media.Caption = msgParts[0]
		*hasCaption = true
	}

	return setMediaField(payload, attType, &media, attURL)
}

func normalizeAttachmentType(attType string) string {
	attType = strings.Split(attType, "/")[0]
	if attType == "application" {
		return "document"
	}
	return attType
}

func shouldAddCaption(msgParts []string, attType string, msg courier.MsgOut) bool {
	return len(msgParts) == 1 &&
		attType != "audio" &&
		len(msg.Attachments()) == 1 &&
		len(msg.QuickReplies()) == 0
}

func setMediaField(payload *SendRequest, attType string, media *Media, attURL string) error {
	switch attType {
	case "image":
		payload.Image = media
	case "audio":
		payload.Audio = media
	case "video":
		payload.Video = media
	case "document":
		filename, err := utils.BasePathForURL(attURL)
		if err != nil {
			filename = ""
		}
		if filename != "" {
			media.Filename = filename
		}
		payload.Document = media
	}
	return nil
}

func buildTextPayload(payload *SendRequest, msg courier.MsgOut, msgParts []string, qrs []models.QuickReply, locationQRs []models.QuickReply, qrsAsList bool, menuButton string, i int, clog *courier.ChannelLog, hasCaption *bool) error {
	msgIndex := i - len(msg.Attachments())

	if len(locationQRs) > 0 {
		buildLocationRequestPayload(payload, msgParts, msgIndex)
	} else if len(qrs) > 0 {
		return buildInteractivePayload(payload, msg, msgParts, qrs, qrsAsList, menuButton, i, clog, hasCaption)
	} else {
		buildSimpleTextPayload(payload, msgParts, msgIndex)
	}

	return nil
}

func buildLocationRequestPayload(payload *SendRequest, msgParts []string, msgIndex int) {
	payload.Type = "interactive"
	interactive := Interactive{
		Type: "location_request_message",
		Body: struct {
			Text string `json:"text"`
		}{Text: msgParts[msgIndex]},
	}
	interactive.Action = &struct {
		Name     string    `json:"name,omitempty"`
		Button   string    `json:"button,omitempty"`
		Sections []Section `json:"sections,omitempty"`
		Buttons  []Button  `json:"buttons,omitempty"`
	}{Name: "send_location"}
	payload.Interactive = &interactive
}

func buildInteractivePayload(payload *SendRequest, msg courier.MsgOut, msgParts []string, qrs []models.QuickReply, qrsAsList bool, menuButton string, i int, clog *courier.ChannelLog, hasCaption *bool) error {
	payload.Type = "interactive"

	if len(qrs) > 10 {
		clog.Error(&clogs.Error{Message: "too many quick replies WhatsApp supports only up to 10 quick replies"})
		qrs = qrs[:10]
	}

	msgIndex := i - len(msg.Attachments())

	if !qrsAsList {
		return buildButtonInteractive(payload, msg, msgParts, qrs, i, hasCaption)
	} else {
		buildListInteractive(payload, msgParts, qrs, menuButton, msgIndex)
	}

	return nil
}

func buildButtonInteractive(payload *SendRequest, msg courier.MsgOut, msgParts []string, qrs []models.QuickReply, i int, hasCaption *bool) error {
	interactive := Interactive{
		Type: "button",
		Body: struct {
			Text string `json:"text"`
		}{Text: msgParts[i]},
	}

	if len(msg.Attachments()) > 0 {
		*hasCaption = true
		err := addHeaderToInteractive(&interactive, msg.Attachments()[i])
		if err != nil {
			return err
		}
	}

	btns := make([]Button, len(qrs))
	for j, qr := range qrs {
		btns[j] = Button{Type: "reply"}
		btns[j].Reply.ID = fmt.Sprint(j)
		btns[j].Reply.Title = qr.Text
	}

	interactive.Action = &struct {
		Name     string    `json:"name,omitempty"`
		Button   string    `json:"button,omitempty"`
		Sections []Section `json:"sections,omitempty"`
		Buttons  []Button  `json:"buttons,omitempty"`
	}{Buttons: btns}

	payload.Interactive = &interactive
	return nil
}

func addHeaderToInteractive(interactive *Interactive, attachment string) error {
	attType, attURL := handlers.SplitAttachment(attachment)
	attType = normalizeAttachmentType(attType)

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
			return err
		}
		interactive.Header = &struct {
			Type     string `json:"type"`
			Text     string `json:"text,omitempty"`
			Video    *Media `json:"video,omitempty"`
			Image    *Media `json:"image,omitempty"`
			Document *Media `json:"document,omitempty"`
		}{Type: "document", Document: &Media{Link: attURL, Filename: filename}}
	}

	return nil
}

func buildListInteractive(payload *SendRequest, msgParts []string, qrs []models.QuickReply, menuButton string, msgIndex int) {
	interactive := Interactive{
		Type: "list",
		Body: struct {
			Text string `json:"text"`
		}{Text: msgParts[msgIndex]},
	}

	section := Section{Rows: make([]SectionRow, len(qrs))}
	for j, qr := range qrs {
		section.Rows[j] = SectionRow{
			ID:          fmt.Sprint(j),
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

	payload.Interactive = &interactive
}

func buildSimpleTextPayload(payload *SendRequest, msgParts []string, msgIndex int) {
	payload.Type = "text"
	text := &Text{PreviewURL: false}

	msgText := msgParts[msgIndex]
	if strings.Contains(msgText, "https://") || strings.Contains(msgText, "http://") {
		text.PreviewURL = true
	}
	text.Body = msgText
	payload.Text = text
}
