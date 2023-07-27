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
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMultiReceive, handlers.JSONPayload(h, h.receiveEvent))
	return nil
}

var waStatusMapping = map[string]courier.MsgStatusValue{
	"sent":      courier.MsgSent,
	"delivered": courier.MsgDelivered,
	"read":      courier.MsgDelivered,
	"failed":    courier.MsgFailed,
}

var waIgnoreStatuses = map[string]bool{
	"deleted": true,
}

type Sender struct {
	ID      string `json:"id"`
	UserRef string `json:"user_ref,omitempty"`
}

type User struct {
	ID string `json:"id"`
}

// {
//   "object":"page",
//   "entry":[{
//     "id":"180005062406476",
//     "time":1514924367082,
//     "messaging":[{
//       "sender":  {"id":"1630934236957797"},
//       "recipient":{"id":"180005062406476"},
//       "timestamp":1514924366807,
//       "message":{
//         "mid":"mid.$cAAD5QiNHkz1m6cyj11guxokwkhi2",
//         "seq":33116,
//         "text":"65863634"
//       }
//     }]
//   }]
// }

type wacMedia struct {
	Caption  string `json:"caption"`
	Filename string `json:"filename"`
	ID       string `json:"id"`
	Mimetype string `json:"mime_type"`
	SHA256   string `json:"sha256"`
}
type moPayload struct {
	Object string `json:"object"`
	Entry  []struct {
		ID      string `json:"id"`
		Time    int64  `json:"time"`
		Changes []struct {
			Field string `json:"field"`
			Value struct {
				MessagingProduct string `json:"messaging_product"`
				Metadata         *struct {
					DisplayPhoneNumber string `json:"display_phone_number"`
					PhoneNumberID      string `json:"phone_number_id"`
				} `json:"metadata"`
				Contacts []struct {
					Profile struct {
						Name string `json:"name"`
					} `json:"profile"`
					WaID string `json:"wa_id"`
				} `json:"contacts"`
				Messages []struct {
					ID        string `json:"id"`
					From      string `json:"from"`
					Timestamp string `json:"timestamp"`
					Type      string `json:"type"`
					Context   *struct {
						Forwarded           bool   `json:"forwarded"`
						FrequentlyForwarded bool   `json:"frequently_forwarded"`
						From                string `json:"from"`
						ID                  string `json:"id"`
					} `json:"context"`
					Text struct {
						Body string `json:"body"`
					} `json:"text"`
					Image    *wacMedia `json:"image"`
					Audio    *wacMedia `json:"audio"`
					Video    *wacMedia `json:"video"`
					Document *wacMedia `json:"document"`
					Voice    *wacMedia `json:"voice"`
					Location *struct {
						Latitude  float64 `json:"latitude"`
						Longitude float64 `json:"longitude"`
						Name      string  `json:"name"`
						Address   string  `json:"address"`
					} `json:"location"`
					Button *struct {
						Text    string `json:"text"`
						Payload string `json:"payload"`
					} `json:"button"`
					Interactive struct {
						Type        string `json:"type"`
						ButtonReply struct {
							ID    string `json:"id"`
							Title string `json:"title"`
						} `json:"button_reply,omitempty"`
						ListReply struct {
							ID    string `json:"id"`
							Title string `json:"title"`
						} `json:"list_reply,omitempty"`
					} `json:"interactive,omitempty"`
					Errors []struct {
						Code  int    `json:"code"`
						Title string `json:"title"`
					} `json:"errors"`
				} `json:"messages"`
				Statuses []struct {
					ID           string `json:"id"`
					RecipientID  string `json:"recipient_id"`
					Status       string `json:"status"`
					Timestamp    string `json:"timestamp"`
					Type         string `json:"type"`
					Conversation *struct {
						ID     string `json:"id"`
						Origin *struct {
							Type string `json:"type"`
						} `json:"origin"`
					} `json:"conversation"`
					Pricing *struct {
						PricingModel string `json:"pricing_model"`
						Billable     bool   `json:"billable"`
						Category     string `json:"category"`
					} `json:"pricing"`
					Errors []struct {
						Code  int    `json:"code"`
						Title string `json:"title"`
					} `json:"errors"`
				} `json:"statuses"`
				Errors []struct {
					Code  int    `json:"code"`
					Title string `json:"title"`
				} `json:"errors"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

// receiveEvent is our HTTP handler function for incoming messages and status updates
func (h *handler) receiveEvent(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *moPayload, clog *courier.ChannelLog) ([]courier.Event, error) {

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

func (h *handler) processCloudWhatsAppPayload(ctx context.Context, channel courier.Channel, payload *moPayload, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, []interface{}, error) {
	// the list of events we deal with
	events := make([]courier.Event, 0, 2)

	// the list of data we will return in our response
	data := make([]interface{}, 0, 2)

	seenMsgIDs := make(map[string]bool)
	contactNames := make(map[string]string)

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
				if seenMsgIDs[msg.ID] {
					continue
				}

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
				event := h.Backend().NewIncomingMsg(channel, urn, text, msg.ID, clog).WithReceivedOn(date).WithContactName(contactNames[msg.From])

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

				events = append(events, event)
				data = append(data, courier.NewMsgReceiveData(event))
				seenMsgIDs[msg.ID] = true
			}

			for _, status := range change.Value.Statuses {

				msgStatus, found := waStatusMapping[status.Status]
				if !found {
					if waIgnoreStatuses[status.Status] {
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

type wacMTMedia struct {
	ID       string `json:"id,omitempty"`
	Link     string `json:"link,omitempty"`
	Caption  string `json:"caption,omitempty"`
	Filename string `json:"filename,omitempty"`
}

type wacMTSection struct {
	Title string            `json:"title,omitempty"`
	Rows  []wacMTSectionRow `json:"rows" validate:"required"`
}

type wacMTSectionRow struct {
	ID          string `json:"id" validate:"required"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

type wacMTButton struct {
	Type  string `json:"type" validate:"required"`
	Reply struct {
		ID    string `json:"id" validate:"required"`
		Title string `json:"title" validate:"required"`
	} `json:"reply" validate:"required"`
}

type wacParam struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type wacComponent struct {
	Type    string      `json:"type"`
	SubType string      `json:"sub_type"`
	Index   string      `json:"index"`
	Params  []*wacParam `json:"parameters"`
}

type wacText struct {
	Body       string `json:"body"`
	PreviewURL bool   `json:"preview_url"`
}

type wacLanguage struct {
	Policy string `json:"policy"`
	Code   string `json:"code"`
}

type wacTemplate struct {
	Name       string          `json:"name"`
	Language   *wacLanguage    `json:"language"`
	Components []*wacComponent `json:"components"`
}

type wacInteractive struct {
	Type   string `json:"type"`
	Header *struct {
		Type     string      `json:"type"`
		Text     string      `json:"text,omitempty"`
		Video    *wacMTMedia `json:"video,omitempty"`
		Image    *wacMTMedia `json:"image,omitempty"`
		Document *wacMTMedia `json:"document,omitempty"`
	} `json:"header,omitempty"`
	Body struct {
		Text string `json:"text"`
	} `json:"body" validate:"required"`
	Footer *struct {
		Text string `json:"text"`
	} `json:"footer,omitempty"`
	Action *struct {
		Button   string         `json:"button,omitempty"`
		Sections []wacMTSection `json:"sections,omitempty"`
		Buttons  []wacMTButton  `json:"buttons,omitempty"`
	} `json:"action,omitempty"`
}

type wacMTPayload struct {
	MessagingProduct string `json:"messaging_product"`
	RecipientType    string `json:"recipient_type"`
	To               string `json:"to"`
	Type             string `json:"type"`

	Text *wacText `json:"text,omitempty"`

	Document *wacMTMedia `json:"document,omitempty"`
	Image    *wacMTMedia `json:"image,omitempty"`
	Audio    *wacMTMedia `json:"audio,omitempty"`
	Video    *wacMTMedia `json:"video,omitempty"`

	Interactive *wacInteractive `json:"interactive,omitempty"`

	Template *wacTemplate `json:"template,omitempty"`
}

type wacMTResponse struct {
	Messages []*struct {
		ID string `json:"id"`
	} `json:"messages"`
	Error struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
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
	lang := getSupportedLanguage(msg.Locale())

	var payloadAudio wacMTPayload

	for i := 0; i < len(msgParts)+len(msg.Attachments()); i++ {
		payload := wacMTPayload{MessagingProduct: "whatsapp", RecipientType: "individual", To: msg.URN().Path()}

		if len(msg.Attachments()) == 0 {
			// do we have a template?
			templating, err := h.getTemplating(msg)
			if err != nil {
				return nil, errors.Wrapf(err, "unable to decode template: %s for channel: %s", string(msg.Metadata()), msg.Channel().UUID())
			}
			if templating != nil {

				payload.Type = "template"

				template := wacTemplate{Name: templating.Template.Name, Language: &wacLanguage{Policy: "deterministic", Code: lang.code}}
				payload.Template = &template

				component := &wacComponent{Type: "body"}

				for _, v := range templating.Variables {
					component.Params = append(component.Params, &wacParam{Type: "text", Text: v})
				}
				template.Components = append(payload.Template.Components, component)

			} else {
				if i < (len(msgParts) + len(msg.Attachments()) - 1) {
					// this is still a msg part
					text := &wacText{PreviewURL: false}
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
							interactive := wacInteractive{Type: "button", Body: struct {
								Text string "json:\"text\""
							}{Text: msgParts[i-len(msg.Attachments())]}}

							btns := make([]wacMTButton, len(qrs))
							for i, qr := range qrs {
								btns[i] = wacMTButton{
									Type: "reply",
								}
								btns[i].Reply.ID = fmt.Sprint(i)
								btns[i].Reply.Title = qr
							}
							interactive.Action = &struct {
								Button   string         "json:\"button,omitempty\""
								Sections []wacMTSection "json:\"sections,omitempty\""
								Buttons  []wacMTButton  "json:\"buttons,omitempty\""
							}{Buttons: btns}
							payload.Interactive = &interactive
						} else if len(qrs) <= 10 {
							interactive := wacInteractive{Type: "list", Body: struct {
								Text string "json:\"text\""
							}{Text: msgParts[i-len(msg.Attachments())]}}

							section := wacMTSection{
								Rows: make([]wacMTSectionRow, len(qrs)),
							}
							for i, qr := range qrs {
								section.Rows[i] = wacMTSectionRow{
									ID:    fmt.Sprint(i),
									Title: qr,
								}
							}

							interactive.Action = &struct {
								Button   string         "json:\"button,omitempty\""
								Sections []wacMTSection "json:\"sections,omitempty\""
								Buttons  []wacMTButton  "json:\"buttons,omitempty\""
							}{Button: lang.menu, Sections: []wacMTSection{
								section,
							}}

							payload.Interactive = &interactive
						} else {
							return nil, fmt.Errorf("too many quick replies WAC supports only up to 10 quick replies")
						}
					} else {
						// this is still a msg part
						text := &wacText{PreviewURL: false}
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
			media := wacMTMedia{Link: attURL}

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
					interactive := wacInteractive{Type: "button", Body: struct {
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
							image := wacMTMedia{
								Link: attURL,
							}
							interactive.Header = &struct {
								Type     string      "json:\"type\""
								Text     string      "json:\"text,omitempty\""
								Video    *wacMTMedia "json:\"video,omitempty\""
								Image    *wacMTMedia "json:\"image,omitempty\""
								Document *wacMTMedia "json:\"document,omitempty\""
							}{Type: "image", Image: &image}
						} else if attType == "video" {
							video := wacMTMedia{
								Link: attURL,
							}
							interactive.Header = &struct {
								Type     string      "json:\"type\""
								Text     string      "json:\"text,omitempty\""
								Video    *wacMTMedia "json:\"video,omitempty\""
								Image    *wacMTMedia "json:\"image,omitempty\""
								Document *wacMTMedia "json:\"document,omitempty\""
							}{Type: "video", Video: &video}
						} else if attType == "document" {
							filename, err := utils.BasePathForURL(attURL)
							if err != nil {
								return nil, err
							}
							document := wacMTMedia{
								Link:     attURL,
								Filename: filename,
							}
							interactive.Header = &struct {
								Type     string      "json:\"type\""
								Text     string      "json:\"text,omitempty\""
								Video    *wacMTMedia "json:\"video,omitempty\""
								Image    *wacMTMedia "json:\"image,omitempty\""
								Document *wacMTMedia "json:\"document,omitempty\""
							}{Type: "document", Document: &document}
						} else if attType == "audio" {
							var zeroIndex bool
							if i == 0 {
								zeroIndex = true
							}
							payloadAudio = wacMTPayload{MessagingProduct: "whatsapp", RecipientType: "individual", To: msg.URN().Path(), Type: "audio", Audio: &wacMTMedia{Link: attURL}}
							status, err := requestD3C(payloadAudio, accessToken, status, sendURL, zeroIndex, clog)
							if err != nil {
								return status, nil
							}
						} else {
							interactive.Type = "button"
							interactive.Body.Text = msgParts[i]
						}
					}

					btns := make([]wacMTButton, len(qrs))
					for i, qr := range qrs {
						btns[i] = wacMTButton{
							Type: "reply",
						}
						btns[i].Reply.ID = fmt.Sprint(i)
						btns[i].Reply.Title = qr
					}
					interactive.Action = &struct {
						Button   string         "json:\"button,omitempty\""
						Sections []wacMTSection "json:\"sections,omitempty\""
						Buttons  []wacMTButton  "json:\"buttons,omitempty\""
					}{Buttons: btns}
					payload.Interactive = &interactive

				} else if len(qrs) <= 10 {
					interactive := wacInteractive{Type: "list", Body: struct {
						Text string "json:\"text\""
					}{Text: msgParts[i-len(msg.Attachments())]}}

					section := wacMTSection{
						Rows: make([]wacMTSectionRow, len(qrs)),
					}
					for i, qr := range qrs {
						section.Rows[i] = wacMTSectionRow{
							ID:    fmt.Sprint(i),
							Title: qr,
						}
					}

					interactive.Action = &struct {
						Button   string         "json:\"button,omitempty\""
						Sections []wacMTSection "json:\"sections,omitempty\""
						Buttons  []wacMTButton  "json:\"buttons,omitempty\""
					}{Button: lang.menu, Sections: []wacMTSection{
						section,
					}}

					payload.Interactive = &interactive
				} else {
					return nil, fmt.Errorf("too many quick replies WAC supports only up to 10 quick replies")
				}
			} else {
				// this is still a msg part
				text := &wacText{PreviewURL: false}
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

func requestD3C(payload wacMTPayload, accessToken string, status courier.MsgStatus, wacPhoneURL *url.URL, zeroIndex bool, clog *courier.ChannelLog) (courier.MsgStatus, error) {
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
	respPayload := &wacMTResponse{}
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

func (h *handler) getTemplating(msg courier.Msg) (*MsgTemplating, error) {
	if len(msg.Metadata()) == 0 {
		return nil, nil
	}

	metadata := &struct {
		Templating *MsgTemplating `json:"templating"`
	}{}
	if err := json.Unmarshal(msg.Metadata(), metadata); err != nil {
		return nil, err
	}

	if metadata.Templating == nil {
		return nil, nil
	}

	if err := utils.Validate(metadata.Templating); err != nil {
		return nil, errors.Wrapf(err, "invalid templating definition")
	}

	return metadata.Templating, nil
}

type MsgTemplating struct {
	Template struct {
		Name string `json:"name" validate:"required"`
		UUID string `json:"uuid" validate:"required"`
	} `json:"template" validate:"required,dive"`
	Namespace string   `json:"namespace"`
	Variables []string `json:"variables"`
}

func getSupportedLanguage(lc courier.Locale) languageInfo {
	// look for exact match
	if lang := supportedLanguages[lc]; lang.code != "" {
		return lang
	}

	// if we have a country, strip that off and look again for a match
	l, c := lc.ToParts()
	if c != "" {
		if lang := supportedLanguages[courier.Locale(l)]; lang.code != "" {
			return lang
		}
	}
	return supportedLanguages["eng"] // fallback to English
}

type languageInfo struct {
	code string
	menu string // translation of "Menu"
}

// Mapping from engine locales to supported languages. Note that these are not all valid BCP47 codes, e.g. fil
// see https://developers.facebook.com/docs/whatsapp/api/messages/message-templates/
var supportedLanguages = map[courier.Locale]languageInfo{
	"afr":    {code: "af", menu: "Kieslys"},   // Afrikaans
	"sqi":    {code: "sq", menu: "Menu"},      // Albanian
	"ara":    {code: "ar", menu: "قائمة"},     // Arabic
	"aze":    {code: "az", menu: "Menu"},      // Azerbaijani
	"ben":    {code: "bn", menu: "Menu"},      // Bengali
	"bul":    {code: "bg", menu: "Menu"},      // Bulgarian
	"cat":    {code: "ca", menu: "Menu"},      // Catalan
	"zho":    {code: "zh_CN", menu: "菜单"},     // Chinese
	"zho-CN": {code: "zh_CN", menu: "菜单"},     // Chinese (CHN)
	"zho-HK": {code: "zh_HK", menu: "菜单"},     // Chinese (HKG)
	"zho-TW": {code: "zh_TW", menu: "菜单"},     // Chinese (TAI)
	"hrv":    {code: "hr", menu: "Menu"},      // Croatian
	"ces":    {code: "cs", menu: "Menu"},      // Czech
	"dah":    {code: "da", menu: "Menu"},      // Danish
	"nld":    {code: "nl", menu: "Menu"},      // Dutch
	"eng":    {code: "en", menu: "Menu"},      // English
	"eng-GB": {code: "en_GB", menu: "Menu"},   // English (UK)
	"eng-US": {code: "en_US", menu: "Menu"},   // English (US)
	"est":    {code: "et", menu: "Menu"},      // Estonian
	"fil":    {code: "fil", menu: "Menu"},     // Filipino
	"fin":    {code: "fi", menu: "Menu"},      // Finnish
	"fra":    {code: "fr", menu: "Menu"},      // French
	"kat":    {code: "ka", menu: "Menu"},      // Georgian
	"deu":    {code: "de", menu: "Menü"},      // German
	"ell":    {code: "el", menu: "Menu"},      // Greek
	"guj":    {code: "gu", menu: "Menu"},      // Gujarati
	"hau":    {code: "ha", menu: "Menu"},      // Hausa
	"enb":    {code: "he", menu: "תפריט"},     // Hebrew
	"hin":    {code: "hi", menu: "Menu"},      // Hindi
	"hun":    {code: "hu", menu: "Menu"},      // Hungarian
	"ind":    {code: "id", menu: "Menu"},      // Indonesian
	"gle":    {code: "ga", menu: "Roghchlár"}, // Irish
	"ita":    {code: "it", menu: "Menu"},      // Italian
	"jpn":    {code: "ja", menu: "Menu"},      // Japanese
	"kan":    {code: "kn", menu: "Menu"},      // Kannada
	"kaz":    {code: "kk", menu: "Menu"},      // Kazakh
	"kin":    {code: "rw_RW", menu: "Menu"},   // Kinyarwanda
	"kor":    {code: "ko", menu: "Menu"},      // Korean
	"kir":    {code: "ky_KG", menu: "Menu"},   // Kyrgyzstan
	"lao":    {code: "lo", menu: "Menu"},      // Lao
	"lav":    {code: "lv", menu: "Menu"},      // Latvian
	"lit":    {code: "lt", menu: "Menu"},      // Lithuanian
	"mal":    {code: "ml", menu: "Menu"},      // Malayalam
	"mkd":    {code: "mk", menu: "Menu"},      // Macedonian
	"msa":    {code: "ms", menu: "Menu"},      // Malay
	"mar":    {code: "mr", menu: "Menu"},      // Marathi
	"nob":    {code: "nb", menu: "Menu"},      // Norwegian
	"fas":    {code: "fa", menu: "Menu"},      // Persian
	"pol":    {code: "pl", menu: "Menu"},      // Polish
	"por":    {code: "pt_PT", menu: "Menu"},   // Portuguese
	"por-BR": {code: "pt_BR", menu: "Menu"},   // Portuguese (BR)
	"por-PT": {code: "pt_PT", menu: "Menu"},   // Portuguese (POR)
	"pan":    {code: "pa", menu: "Menu"},      // Punjabi
	"ron":    {code: "ro", menu: "Menu"},      // Romanian
	"rus":    {code: "ru", menu: "Menu"},      // Russian
	"srp":    {code: "sr", menu: "Menu"},      // Serbian
	"slk":    {code: "sk", menu: "Menu"},      // Slovak
	"slv":    {code: "sl", menu: "Menu"},      // Slovenian
	"spa":    {code: "es", menu: "Menú"},      // Spanish
	"spa-AR": {code: "es_AR", menu: "Menú"},   // Spanish (ARG)
	"spa-ES": {code: "es_ES", menu: "Menú"},   // Spanish (SPA)
	"spa-MX": {code: "es_MX", menu: "Menú"},   // Spanish (MEX)
	"swa":    {code: "sw", menu: "Menyu"},     // Swahili
	"swe":    {code: "sv", menu: "Menu"},      // Swedish
	"tam":    {code: "ta", menu: "Menu"},      // Tamil
	"tel":    {code: "te", menu: "Menu"},      // Telugu
	"tha":    {code: "th", menu: "Menu"},      // Thai
	"tur":    {code: "tr", menu: "Menu"},      // Turkish
	"ukr":    {code: "uk", menu: "Menu"},      // Ukrainian
	"urd":    {code: "ur", menu: "Menu"},      // Urdu
	"uzb":    {code: "uz", menu: "Menu"},      // Uzbek
	"vie":    {code: "vi", menu: "Menu"},      // Vietnamese
	"zul":    {code: "zu", menu: "Menu"},      // Zulu
}
