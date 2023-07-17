package dialog360

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
	"github.com/pkg/errors"
)

const (
	d3AuthorizationKey = "D360-API-KEY"
)

var (
	// max for the body
	maxMsgLength = 1000
)

func init() {
	courier.RegisterHandler(newWAHandler(courier.ChannelType("D3C"), "360Dialog"))
}

type handler struct {
	handlers.BaseHandler
}

func newWAHandler(channelType courier.ChannelType, name string) courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(channelType, name)}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", handlers.JSONPayload(h, h.receiveEvent))
	return nil
}

// receiveEvent is our HTTP handler function for incoming messages and status updates
func (h *handler) receiveEvent(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *handlers.WACMOPayload, clog *courier.ChannelLog) ([]courier.Event, error) {

	// is not a 'whatsapp_business_account' object? ignore it
	if payload.Object != "whatsapp_business_account" {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring request")
	}

	// no entries? ignore this request
	if len(payload.Entry) == 0 {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring request, no entries")
	}

	var events []courier.Event
	var data []interface{}

	events, data, err := h.processCloudWhatsAppPayload(ctx, channel, payload, w, r, clog)
	if err != nil {
		return nil, err
	}

	return events, courier.WriteDataResponse(w, http.StatusOK, "Events Handled", data)
}

func (h *handler) processCloudWhatsAppPayload(ctx context.Context, channel courier.Channel, payload *handlers.WACMOPayload, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, []interface{}, error) {
	// the list of events we deal with
	events := make([]courier.Event, 0, 2)

	// the list of data we will return in our response
	data := make([]interface{}, 0, 2)

	var contactNames = make(map[string]string)

	// for each entry
	for _, entry := range payload.Entry {
		if len(entry.Changes) == 0 {
			continue
		}

		for _, change := range entry.Changes {

			for _, contact := range change.Value.Contacts {
				contactNames[contact.WaID] = contact.Profile.Name
			}

			for _, msg := range change.Value.Messages {
				// create our date from the timestamp
				ts, err := strconv.ParseInt(msg.Timestamp, 10, 64)
				if err != nil {
					return nil, nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, fmt.Sprintf("invalid timestamp: %s", msg.Timestamp))
				}
				date := time.Unix(ts, 0).UTC()

				urn, err := urns.NewWhatsAppURN(msg.From)
				if err != nil {
					return nil, nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, err.Error())
				}

				for _, msgError := range msg.Errors {
					clog.Error(courier.ErrorExternal(strconv.Itoa(msgError.Code), msgError.Title))
				}

				text := ""
				mediaURL := ""

				if msg.Type == "text" {
					text = msg.Text.Body
				} else if msg.Type == "audio" && msg.Audio != nil {
					text = msg.Audio.Caption
					mediaURL, err = resolveMediaURL(channel, msg.Audio.ID, clog)
				} else if msg.Type == "voice" && msg.Voice != nil {
					text = msg.Voice.Caption
					mediaURL, err = resolveMediaURL(channel, msg.Voice.ID, clog)
				} else if msg.Type == "button" && msg.Button != nil {
					text = msg.Button.Text
				} else if msg.Type == "document" && msg.Document != nil {
					text = msg.Document.Caption
					mediaURL, err = resolveMediaURL(channel, msg.Document.ID, clog)
				} else if msg.Type == "image" && msg.Image != nil {
					text = msg.Image.Caption
					mediaURL, err = resolveMediaURL(channel, msg.Image.ID, clog)
				} else if msg.Type == "video" && msg.Video != nil {
					text = msg.Video.Caption
					mediaURL, err = resolveMediaURL(channel, msg.Video.ID, clog)
				} else if msg.Type == "location" && msg.Location != nil {
					mediaURL = fmt.Sprintf("geo:%f,%f", msg.Location.Latitude, msg.Location.Longitude)
				} else if msg.Type == "interactive" && msg.Interactive.Type == "button_reply" {
					text = msg.Interactive.ButtonReply.Title
				} else if msg.Type == "interactive" && msg.Interactive.Type == "list_reply" {
					text = msg.Interactive.ListReply.Title
				} else {
					// we received a message type we do not support.
					courier.LogRequestError(r, channel, fmt.Errorf("unsupported message type %s", msg.Type))
					continue
				}

				// create our message
				ev := h.Backend().NewIncomingMsg(channel, urn, text, clog).WithReceivedOn(date).WithExternalID(msg.ID).WithContactName(contactNames[msg.From])
				event := h.Backend().CheckExternalIDSeen(ev)

				// we had an error downloading media
				if err != nil {
					courier.LogRequestError(r, channel, err)
				}

				if mediaURL != "" {
					event.WithAttachment(mediaURL)
				}

				err = h.Backend().WriteMsg(ctx, event, clog)
				if err != nil {
					return nil, nil, err
				}

				h.Backend().WriteExternalIDSeen(event)

				events = append(events, event)
				data = append(data, courier.NewMsgReceiveData(event))

			}

			for _, status := range change.Value.Statuses {

				msgStatus, found := handlers.WaStatusMapping[status.Status]
				if !found {
					if handlers.WaIgnoreStatuses[status.Status] {
						data = append(data, courier.NewInfoData(fmt.Sprintf("ignoring status: %s", status.Status)))
					} else {
						handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, fmt.Sprintf("unknown status: %s", status.Status))
					}
					continue
				}

				for _, statusError := range status.Errors {
					clog.Error(courier.ErrorExternal(strconv.Itoa(statusError.Code), statusError.Title))
				}

				event := h.Backend().NewMsgStatusForExternalID(channel, status.ID, msgStatus, clog)
				err := h.Backend().WriteMsgStatus(ctx, event)

				// we don't know about this message, just tell them we ignored it
				if err == courier.ErrMsgNotFound {
					data = append(data, courier.NewInfoData(fmt.Sprintf("message id: %s not found, ignored", status.ID)))
					continue
				}

				if err != nil {
					return nil, nil, err
				}

				events = append(events, event)
				data = append(data, courier.NewStatusData(event))

			}

			for _, chError := range change.Value.Errors {
				clog.Error(courier.ErrorExternal(strconv.Itoa(chError.Code), chError.Title))
			}

		}

	}
	return events, data, nil
}

// BuildAttachmentRequest to download media for message attachment with Bearer token set
func (h *handler) BuildAttachmentRequest(ctx context.Context, b courier.Backend, channel courier.Channel, attachmentURL string, clog *courier.ChannelLog) (*http.Request, error) {
	token := channel.StringConfigForKey(courier.ConfigAuthToken, "")
	if token == "" {
		return nil, fmt.Errorf("missing token for D3C channel")
	}

	// set the access token as the authorization header
	req, _ := http.NewRequest(http.MethodGet, attachmentURL, nil)
	req.Header.Set("User-Agent", utils.HTTPUserAgent)
	req.Header.Set(d3AuthorizationKey, token)
	return req, nil
}

var _ courier.AttachmentRequestBuilder = (*handler)(nil)

func resolveMediaURL(channel courier.Channel, mediaID string, clog *courier.ChannelLog) (string, error) {
	// sometimes WA will send an attachment with status=undownloaded and no ID
	if mediaID == "" {
		return "", nil
	}

	urlStr := channel.StringConfigForKey(courier.ConfigBaseURL, "")
	url, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("invalid base url set for D3C channel: %s", err)
	}

	mediaPath, _ := url.Parse("/whatsapp_business/attachments/")
	mediaEndpoint := url.ResolveReference(mediaPath).String()

	fileURL := fmt.Sprintf("%s?mid=%s", mediaEndpoint, mediaID)

	return fileURL, nil
}

// Send implements courier.ChannelHandler
func (h *handler) Send(ctx context.Context, msg courier.Msg, clog *courier.ChannelLog) (courier.MsgStatus, error) {
	conn := h.Backend().RedisPool().Get()
	defer conn.Close()

	// get our token
	// can't do anything without an access token
	accessToken := msg.Channel().StringConfigForKey(courier.ConfigAuthToken, "")
	if accessToken == "" {
		return nil, fmt.Errorf("missing token for D3C channel")
	}

	urlStr := msg.Channel().StringConfigForKey(courier.ConfigBaseURL, "")
	url, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid base url set for D3C channel: %s", err)
	}
	sendURL, _ := url.Parse("/messages")

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored, clog)

	hasCaption := false

	msgParts := make([]string, 0)
	if msg.Text() != "" {
		msgParts = handlers.SplitMsgByChannel(msg.Channel(), msg.Text(), maxMsgLength)
	}
	qrs := msg.QuickReplies()
	lang := handlers.WAGetSupportedLanguage(msg.Locale())

	var payloadAudio handlers.WACMTPayload

	for i := 0; i < len(msgParts)+len(msg.Attachments()); i++ {
		payload := handlers.WACMTPayload{MessagingProduct: "whatsapp", RecipientType: "individual", To: msg.URN().Path()}

		if len(msg.Attachments()) == 0 {
			// do we have a template?
			templating, err := handlers.GetTemplating(msg)
			if err != nil {
				return nil, errors.Wrapf(err, "unable to decode template: %s for channel: %s", string(msg.Metadata()), msg.Channel().UUID())
			}
			if templating != nil {

				payload.Type = "template"

				template := handlers.WACTemplate{Name: templating.Template.Name, Language: &handlers.WACLanguage{Policy: "deterministic", Code: lang.Code}}
				payload.Template = &template

				component := &handlers.WACComponent{Type: "body"}

				for _, v := range templating.Variables {
					component.Params = append(component.Params, &handlers.WACParam{Type: "text", Text: v})
				}
				template.Components = append(payload.Template.Components, component)

			} else {
				if i < (len(msgParts) + len(msg.Attachments()) - 1) {
					// this is still a msg part
					text := &handlers.WACText{PreviewURL: false}
					payload.Type = "text"
					if strings.Contains(msgParts[i-len(msg.Attachments())], "https://") || strings.Contains(msgParts[i-len(msg.Attachments())], "http://") {
						text.PreviewURL = true
					}
					text.Body = msgParts[i-len(msg.Attachments())]
					payload.Text = text
				} else {
					if len(qrs) > 0 {
						payload.Type = "interactive"
						// We can use buttons
						if len(qrs) <= 3 {
							interactive := handlers.WACInteractive{Type: "button", Body: struct {
								Text string "json:\"text\""
							}{Text: msgParts[i-len(msg.Attachments())]}}

							btns := make([]handlers.WACMTButton, len(qrs))
							for i, qr := range qrs {
								btns[i] = handlers.WACMTButton{
									Type: "reply",
								}
								btns[i].Reply.ID = fmt.Sprint(i)
								btns[i].Reply.Title = qr
							}
							interactive.Action = &struct {
								Button   string                  "json:\"button,omitempty\""
								Sections []handlers.WACMTSection "json:\"sections,omitempty\""
								Buttons  []handlers.WACMTButton  "json:\"buttons,omitempty\""
							}{Buttons: btns}
							payload.Interactive = &interactive
						} else if len(qrs) <= 10 {
							interactive := handlers.WACInteractive{Type: "list", Body: struct {
								Text string "json:\"text\""
							}{Text: msgParts[i-len(msg.Attachments())]}}

							section := handlers.WACMTSection{
								Rows: make([]handlers.WACMTSectionRow, len(qrs)),
							}
							for i, qr := range qrs {
								section.Rows[i] = handlers.WACMTSectionRow{
									ID:    fmt.Sprint(i),
									Title: qr,
								}
							}

							interactive.Action = &struct {
								Button   string                  "json:\"button,omitempty\""
								Sections []handlers.WACMTSection "json:\"sections,omitempty\""
								Buttons  []handlers.WACMTButton  "json:\"buttons,omitempty\""
							}{Button: lang.Menu, Sections: []handlers.WACMTSection{
								section,
							}}

							payload.Interactive = &interactive
						} else {
							return nil, fmt.Errorf("too many quick replies WAC supports only up to 10 quick replies")
						}
					} else {
						// this is still a msg part
						text := &handlers.WACText{PreviewURL: false}
						payload.Type = "text"
						if strings.Contains(msgParts[i-len(msg.Attachments())], "https://") || strings.Contains(msgParts[i-len(msg.Attachments())], "http://") {
							text.PreviewURL = true
						}
						text.Body = msgParts[i-len(msg.Attachments())]
						payload.Text = text
					}
				}
			}

		} else if i < len(msg.Attachments()) && (len(qrs) == 0 || len(qrs) > 3) {
			attType, attURL := handlers.SplitAttachment(msg.Attachments()[i])
			attType = strings.Split(attType, "/")[0]
			if attType == "application" {
				attType = "document"
			}
			payload.Type = attType
			media := handlers.WACMTMedia{Link: attURL}

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
				// We can use buttons
				if len(qrs) <= 3 {
					interactive := handlers.WACInteractive{Type: "button", Body: struct {
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
							image := handlers.WACMTMedia{
								Link: attURL,
							}
							interactive.Header = &struct {
								Type     string               "json:\"type\""
								Text     string               "json:\"text,omitempty\""
								Video    *handlers.WACMTMedia "json:\"video,omitempty\""
								Image    *handlers.WACMTMedia "json:\"image,omitempty\""
								Document *handlers.WACMTMedia "json:\"document,omitempty\""
							}{Type: "image", Image: &image}
						} else if attType == "video" {
							video := handlers.WACMTMedia{
								Link: attURL,
							}
							interactive.Header = &struct {
								Type     string               "json:\"type\""
								Text     string               "json:\"text,omitempty\""
								Video    *handlers.WACMTMedia "json:\"video,omitempty\""
								Image    *handlers.WACMTMedia "json:\"image,omitempty\""
								Document *handlers.WACMTMedia "json:\"document,omitempty\""
							}{Type: "video", Video: &video}
						} else if attType == "document" {
							filename, err := utils.BasePathForURL(attURL)
							if err != nil {
								return nil, err
							}
							document := handlers.WACMTMedia{
								Link:     attURL,
								Filename: filename,
							}
							interactive.Header = &struct {
								Type     string               "json:\"type\""
								Text     string               "json:\"text,omitempty\""
								Video    *handlers.WACMTMedia "json:\"video,omitempty\""
								Image    *handlers.WACMTMedia "json:\"image,omitempty\""
								Document *handlers.WACMTMedia "json:\"document,omitempty\""
							}{Type: "document", Document: &document}
						} else if attType == "audio" {
							var zeroIndex bool
							if i == 0 {
								zeroIndex = true
							}
							payloadAudio = handlers.WACMTPayload{MessagingProduct: "whatsapp", RecipientType: "individual", To: msg.URN().Path(), Type: "audio", Audio: &handlers.WACMTMedia{Link: attURL}}
							status, err := requestD3C(payloadAudio, accessToken, status, sendURL, zeroIndex, clog)
							if err != nil {
								return status, nil
							}
						} else {
							interactive.Type = "button"
							interactive.Body.Text = msgParts[i]
						}
					}

					btns := make([]handlers.WACMTButton, len(qrs))
					for i, qr := range qrs {
						btns[i] = handlers.WACMTButton{
							Type: "reply",
						}
						btns[i].Reply.ID = fmt.Sprint(i)
						btns[i].Reply.Title = qr
					}
					interactive.Action = &struct {
						Button   string                  "json:\"button,omitempty\""
						Sections []handlers.WACMTSection "json:\"sections,omitempty\""
						Buttons  []handlers.WACMTButton  "json:\"buttons,omitempty\""
					}{Buttons: btns}
					payload.Interactive = &interactive

				} else if len(qrs) <= 10 {
					interactive := handlers.WACInteractive{Type: "list", Body: struct {
						Text string "json:\"text\""
					}{Text: msgParts[i-len(msg.Attachments())]}}

					section := handlers.WACMTSection{
						Rows: make([]handlers.WACMTSectionRow, len(qrs)),
					}
					for i, qr := range qrs {
						section.Rows[i] = handlers.WACMTSectionRow{
							ID:    fmt.Sprint(i),
							Title: qr,
						}
					}

					interactive.Action = &struct {
						Button   string                  "json:\"button,omitempty\""
						Sections []handlers.WACMTSection "json:\"sections,omitempty\""
						Buttons  []handlers.WACMTButton  "json:\"buttons,omitempty\""
					}{Button: lang.Menu, Sections: []handlers.WACMTSection{
						section,
					}}

					payload.Interactive = &interactive
				} else {
					return nil, fmt.Errorf("too many quick replies WAC supports only up to 10 quick replies")
				}
			} else {
				// this is still a msg part
				text := &handlers.WACText{PreviewURL: false}
				payload.Type = "text"
				if strings.Contains(msgParts[i-len(msg.Attachments())], "https://") || strings.Contains(msgParts[i-len(msg.Attachments())], "http://") {
					text.PreviewURL = true
				}
				text.Body = msgParts[i-len(msg.Attachments())]
				payload.Text = text
			}
		}

		var zeroIndex bool
		if i == 0 {
			zeroIndex = true
		}

		status, err := requestD3C(payload, accessToken, status, sendURL, zeroIndex, clog)
		if err != nil {
			return status, err
		}

		if hasCaption {
			break
		}
	}
	return status, nil
}

func requestD3C(payload handlers.WACMTPayload, accessToken string, status courier.MsgStatus, wacPhoneURL *url.URL, zeroIndex bool, clog *courier.ChannelLog) (courier.MsgStatus, error) {
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return status, err
	}

	req, err := http.NewRequest(http.MethodPost, wacPhoneURL.String(), bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set(d3AuthorizationKey, accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	_, respBody, _ := handlers.RequestHTTP(req, clog)
	respPayload := &handlers.WACMTResponse{}
	err = json.Unmarshal(respBody, respPayload)
	if err != nil {
		clog.Error(courier.ErrorResponseUnparseable("JSON"))
		return status, nil
	}

	if respPayload.Error.Code != 0 {
		clog.Error(courier.ErrorExternal(strconv.Itoa(respPayload.Error.Code), respPayload.Error.Message))
		return status, nil
	}

	externalID := respPayload.Messages[0].ID
	if zeroIndex && externalID != "" {
		status.SetExternalID(externalID)
	}
	// this was wired successfully
	status.SetStatus(courier.MsgWired)
	return status, nil
}
