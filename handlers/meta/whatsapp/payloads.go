package whatsapp

import (
	"context"
	"fmt"
	"strings"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/courier/utils/clogs"
)

func GetMsgPayloads(ctx context.Context, msg courier.MsgOut, maxMsgLength int, clog *courier.ChannelLog) ([]SendRequest, error) {
	requestPayloads := make([]SendRequest, 0)

	if msg.Templating() != nil {
		payload := SendRequest{MessagingProduct: "whatsapp", RecipientType: "individual", To: msg.URN().Path()}
		payload.Type = "template"
		payload.Template = GetTemplatePayload(msg.Templating())
		requestPayloads = append(requestPayloads, payload)
		return requestPayloads, nil
	} else {
		hasCaption := false

		msgParts := make([]string, 0)
		if msg.Text() != "" {
			msgParts = handlers.SplitMsgByChannel(msg.Channel(), msg.Text(), maxMsgLength)
		}
		qrs := msg.QuickReplies()

		qrsAsList := false
		for i, qr := range qrs {
			if i > 2 || qr.Extra != "" {
				qrsAsList = true
			}
		}

		menuButton := handlers.GetText("Menu", msg.Locale())

		for i := 0; i < len(msgParts)+len(msg.Attachments()); i++ {
			payload := SendRequest{MessagingProduct: "whatsapp", RecipientType: "individual", To: msg.URN().Path()}

			if len(msg.Attachments()) == 0 {

				if i < (len(msgParts) + len(msg.Attachments()) - 1) {
					// this is still a msg part
					text := &Text{PreviewURL: false}
					payload.Type = "text"
					if strings.Contains(msgParts[i-len(msg.Attachments())], "https://") || strings.Contains(msgParts[i-len(msg.Attachments())], "http://") {
						text.PreviewURL = true
					}
					text.Body = msgParts[i-len(msg.Attachments())]
					payload.Text = text
				} else {
					if len(qrs) > 0 {
						payload.Type = "interactive"

						// if we have more than 10 quick replies, truncate and add channel error
						if len(qrs) > 10 {
							clog.Error(&clogs.Error{Message: "too many quick replies WhatsApp supports only up to 10 quick replies"})
							qrs = qrs[:10]
						}

						// We can use buttons
						if !qrsAsList {
							interactive := Interactive{Type: "button", Body: struct {
								Text string "json:\"text\""
							}{Text: msgParts[i-len(msg.Attachments())]}}

							btns := make([]Button, len(qrs))
							for i, qr := range qrs {
								btns[i] = Button{
									Type: "reply",
								}
								btns[i].Reply.ID = fmt.Sprint(i)
								btns[i].Reply.Title = qr.Text
							}
							interactive.Action = &struct {
								Button   string    "json:\"button,omitempty\""
								Sections []Section "json:\"sections,omitempty\""
								Buttons  []Button  "json:\"buttons,omitempty\""
							}{Buttons: btns}
							payload.Interactive = &interactive
						} else {
							interactive := Interactive{Type: "list", Body: struct {
								Text string "json:\"text\""
							}{Text: msgParts[i-len(msg.Attachments())]}}

							section := Section{
								Rows: make([]SectionRow, len(qrs)),
							}
							for i, qr := range qrs {
								section.Rows[i] = SectionRow{
									ID:          fmt.Sprint(i),
									Title:       qr.Text,
									Description: qr.Extra,
								}
							}

							interactive.Action = &struct {
								Button   string    "json:\"button,omitempty\""
								Sections []Section "json:\"sections,omitempty\""
								Buttons  []Button  "json:\"buttons,omitempty\""
							}{Button: menuButton, Sections: []Section{
								section,
							}}

							payload.Interactive = &interactive
						}
					} else {
						// this is still a msg part
						text := &Text{PreviewURL: false}
						payload.Type = "text"
						if strings.Contains(msgParts[i-len(msg.Attachments())], "https://") || strings.Contains(msgParts[i-len(msg.Attachments())], "http://") {
							text.PreviewURL = true
						}
						text.Body = msgParts[i-len(msg.Attachments())]
						payload.Text = text
					}
				}

			} else if i < len(msg.Attachments()) && (len(qrs) == 0 || len(qrs) > 3) {
				attType, attURL := handlers.SplitAttachment(msg.Attachments()[i])
				attType = strings.Split(attType, "/")[0]
				if attType == "application" {
					attType = "document"
				}
				payload.Type = attType
				media := Media{Link: attURL}

				if len(msgParts) == 1 && attType != "audio" && len(msg.Attachments()) == 1 && len(msg.QuickReplies()) == 0 {
					media.Caption = msgParts[i]
					hasCaption = true
				}

				if attType == "image" {
					payload.Image = &media
				} else if attType == "audio" {
					payload.Audio = &media
				} else if attType == "video" {
					payload.Video = &media
				} else if attType == "document" {
					filename, err := utils.BasePathForURL(attURL)
					if err != nil {
						filename = ""
					}
					if filename != "" {
						media.Filename = filename
					}
					payload.Document = &media
				}
			} else {
				if len(qrs) > 0 {
					payload.Type = "interactive"
					// if we have more than 10 quick replies, truncate and add channel error
					if len(qrs) > 10 {
						clog.Error(&clogs.Error{Message: "too many quick replies D3C supports only up to 10 quick replies"})
						qrs = qrs[:10]
					}

					// We can use buttons
					if len(qrs) <= 3 {
						interactive := Interactive{Type: "button", Body: struct {
							Text string "json:\"text\""
						}{Text: msgParts[i]}}

						if len(msg.Attachments()) > 0 {
							hasCaption = true
							attType, attURL := handlers.SplitAttachment(msg.Attachments()[i])
							attType = strings.Split(attType, "/")[0]
							if attType == "application" {
								attType = "document"
							}
							if attType == "image" {
								image := Media{
									Link: attURL,
								}
								interactive.Header = &struct {
									Type     string "json:\"type\""
									Text     string "json:\"text,omitempty\""
									Video    *Media "json:\"video,omitempty\""
									Image    *Media "json:\"image,omitempty\""
									Document *Media "json:\"document,omitempty\""
								}{Type: "image", Image: &image}
							} else if attType == "video" {
								video := Media{
									Link: attURL,
								}
								interactive.Header = &struct {
									Type     string "json:\"type\""
									Text     string "json:\"text,omitempty\""
									Video    *Media "json:\"video,omitempty\""
									Image    *Media "json:\"image,omitempty\""
									Document *Media "json:\"document,omitempty\""
								}{Type: "video", Video: &video}
							} else if attType == "document" {
								filename, err := utils.BasePathForURL(attURL)
								if err != nil {
									return requestPayloads, err
								}
								document := Media{
									Link:     attURL,
									Filename: filename,
								}
								interactive.Header = &struct {
									Type     string "json:\"type\""
									Text     string "json:\"text,omitempty\""
									Video    *Media "json:\"video,omitempty\""
									Image    *Media "json:\"image,omitempty\""
									Document *Media "json:\"document,omitempty\""
								}{Type: "document", Document: &document}
							} else if attType == "audio" {
								payloadAudio := SendRequest{MessagingProduct: "whatsapp", RecipientType: "individual", To: msg.URN().Path(), Type: "audio", Audio: &Media{Link: attURL}}

								requestPayloads = append(requestPayloads, payloadAudio)

							} else {
								interactive.Type = "button"
								interactive.Body.Text = msgParts[i]
							}
						}

						btns := make([]Button, len(qrs))
						for i, qr := range qrs {
							btns[i] = Button{
								Type: "reply",
							}
							btns[i].Reply.ID = fmt.Sprint(i)
							btns[i].Reply.Title = qr.Text
						}
						interactive.Action = &struct {
							Button   string    "json:\"button,omitempty\""
							Sections []Section "json:\"sections,omitempty\""
							Buttons  []Button  "json:\"buttons,omitempty\""
						}{Buttons: btns}
						payload.Interactive = &interactive

					} else {
						interactive := Interactive{Type: "list", Body: struct {
							Text string "json:\"text\""
						}{Text: msgParts[i-len(msg.Attachments())]}}

						section := Section{
							Rows: make([]SectionRow, len(qrs)),
						}
						for i, qr := range qrs {
							section.Rows[i] = SectionRow{
								ID:    fmt.Sprint(i),
								Title: qr.Text,
							}
						}

						interactive.Action = &struct {
							Button   string    "json:\"button,omitempty\""
							Sections []Section "json:\"sections,omitempty\""
							Buttons  []Button  "json:\"buttons,omitempty\""
						}{Button: menuButton, Sections: []Section{
							section,
						}}

						payload.Interactive = &interactive
					}
				} else {
					// this is still a msg part
					text := &Text{PreviewURL: false}
					payload.Type = "text"
					if strings.Contains(msgParts[i-len(msg.Attachments())], "https://") || strings.Contains(msgParts[i-len(msg.Attachments())], "http://") {
						text.PreviewURL = true
					}
					text.Body = msgParts[i-len(msg.Attachments())]
					payload.Text = text
				}
			}
			requestPayloads = append(requestPayloads, payload)

			if hasCaption {
				return requestPayloads, nil
			}
		}
		return requestPayloads, nil
	}
}
