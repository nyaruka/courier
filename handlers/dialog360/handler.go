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

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/handlers/meta/whatsapp"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/courier/utils/clogs"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
)

const (
	d3AuthorizationKey = "D360-API-KEY"
)

var (
	// max for the body
	maxMsgLength = 4096
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

//	{
//	  "object":"page",
//	  "entry":[{
//	    "id":"180005062406476",
//	    "time":1514924367082,
//	    "messaging":[{
//	      "sender":  {"id":"1630934236957797"},
//	      "recipient":{"id":"180005062406476"},
//	      "timestamp":1514924366807,
//	      "message":{
//	        "mid":"mid.$cAAD5QiNHkz1m6cyj11guxokwkhi2",
//	        "seq":33116,
//	        "text":"65863634"
//	      }
//	    }]
//	  }]
//	}
type Notifications struct {
	Object string `json:"object"`
	Entry  []struct {
		ID      string            `json:"id"`
		Time    int64             `json:"time"`
		Changes []whatsapp.Change `json:"changes"` // used by WhatsApp
	} `json:"entry"`
}

// receiveEvent is our HTTP handler function for incoming messages and status updates
func (h *handler) receiveEvent(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *Notifications, clog *courier.ChannelLog) ([]courier.Event, error) {

	// is not a 'whatsapp_business_account' object? ignore it
	if payload.Object != "whatsapp_business_account" {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring request")
	}

	// no entries? ignore this request
	if len(payload.Entry) == 0 {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring request, no entries")
	}

	var events []courier.Event
	var data []any

	events, data, err := h.processWhatsAppPayload(ctx, channel, payload, w, r, clog)
	if err != nil {
		return nil, err
	}

	return events, courier.WriteDataResponse(w, http.StatusOK, "Events Handled", data)
}

func (h *handler) processWhatsAppPayload(ctx context.Context, channel courier.Channel, payload *Notifications, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, []any, error) {
	// the list of events we deal with
	events := make([]courier.Event, 0, 2)

	// the list of data we will return in our response
	data := make([]any, 0, 2)

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

				urn, err := urns.New(urns.WhatsApp, msg.From)
				if err != nil {
					return nil, nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "invalid whatsapp id")
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
					mediaURL, err = h.resolveMediaURL(channel, msg.Audio.ID, clog)
				} else if msg.Type == "voice" && msg.Voice != nil {
					text = msg.Voice.Caption
					mediaURL, err = h.resolveMediaURL(channel, msg.Voice.ID, clog)
				} else if msg.Type == "button" && msg.Button != nil {
					text = msg.Button.Text
				} else if msg.Type == "document" && msg.Document != nil {
					text = msg.Document.Caption
					mediaURL, err = h.resolveMediaURL(channel, msg.Document.ID, clog)
				} else if msg.Type == "image" && msg.Image != nil {
					text = msg.Image.Caption
					mediaURL, err = h.resolveMediaURL(channel, msg.Image.ID, clog)
				} else if msg.Type == "video" && msg.Video != nil {
					text = msg.Video.Caption
					mediaURL, err = h.resolveMediaURL(channel, msg.Video.ID, clog)
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

				msgStatus, found := whatsapp.StatusMapping[status.Status]
				if !found {
					if whatsapp.IgnoreStatuses[status.Status] {
						data = append(data, courier.NewInfoData(fmt.Sprintf("ignoring status: %s", status.Status)))
					} else {
						handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, fmt.Sprintf("unknown status: %s", status.Status))
					}
					continue
				}

				for _, statusError := range status.Errors {
					clog.Error(courier.ErrorExternal(strconv.Itoa(statusError.Code), statusError.Title))
				}

				event := h.Backend().NewStatusUpdateByExternalID(channel, status.ID, msgStatus, clog)
				err := h.Backend().WriteStatusUpdate(ctx, event)
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
	req.Header.Set(d3AuthorizationKey, token)
	return req, nil
}

var _ courier.AttachmentRequestBuilder = (*handler)(nil)

func (h *handler) resolveMediaURL(channel courier.Channel, mediaID string, clog *courier.ChannelLog) (string, error) {
	// sometimes WA will send an attachment with status=undownloaded and no ID
	if mediaID == "" {
		return "", nil
	}

	token := channel.StringConfigForKey(courier.ConfigAuthToken, "")
	if token == "" {
		return "", fmt.Errorf("missing token for D3C channel")
	}

	urlStr := channel.StringConfigForKey(courier.ConfigBaseURL, "")
	url, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("invalid base url set for D3C channel: %s", err)
	}

	mediaPath, _ := url.Parse(mediaID)
	mediaURL := url.ResolveReference(mediaPath).String()

	req, _ := http.NewRequest(http.MethodGet, mediaURL, nil)
	req.Header.Set(d3AuthorizationKey, token)

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("failed to request media URL for D3C channel: %s", err)
	}

	fbFileURL, err := jsonparser.GetString(respBody, "url")
	if err != nil {
		return "", fmt.Errorf("missing url field in response for D3C media: %s", err)
	}

	fileURL := strings.ReplaceAll(fbFileURL, "https://lookaside.fbsbx.com", urlStr)

	return fileURL, nil
}

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	accessToken := msg.Channel().StringConfigForKey(courier.ConfigAuthToken, "")
	urlStr := msg.Channel().StringConfigForKey(courier.ConfigBaseURL, "")
	url, err := url.Parse(urlStr)
	if accessToken == "" || err != nil {
		return courier.ErrChannelConfig
	}
	sendURL, _ := url.Parse("/messages")

	hasCaption := false

	msgParts := make([]string, 0)
	if msg.Text() != "" {
		msgParts = handlers.SplitMsgByChannel(msg.Channel(), msg.Text(), maxMsgLength)
	}
	qrs := msg.QuickReplies()
	menuButton := handlers.GetText("Menu", msg.Locale())

	var payloadAudio whatsapp.SendRequest

	// do we have a template?
	if msg.Templating() != nil {
		payload := whatsapp.SendRequest{MessagingProduct: "whatsapp", RecipientType: "individual", To: msg.URN().Path()}
		payload.Type = "template"
		payload.Template = whatsapp.GetTemplatePayload(msg.Templating())

		err := h.requestD3C(payload, accessToken, res, sendURL, clog)
		if err != nil {
			return err
		}

	} else {

		for i := 0; i < len(msgParts)+len(msg.Attachments()); i++ {
			payload := whatsapp.SendRequest{MessagingProduct: "whatsapp", RecipientType: "individual", To: msg.URN().Path()}

			if len(msg.Attachments()) == 0 {

				if i < (len(msgParts) + len(msg.Attachments()) - 1) {
					// this is still a msg part
					text := &whatsapp.Text{PreviewURL: false}
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
							clog.Error(clogs.NewLogError("", "", "too many quick replies D3C supports only up to 10 quick replies"))
							qrs = qrs[:10]
						}

						// We can use buttons
						if len(qrs) <= 3 {
							interactive := whatsapp.Interactive{Type: "button", Body: struct {
								Text string "json:\"text\""
							}{Text: msgParts[i-len(msg.Attachments())]}}

							btns := make([]whatsapp.Button, len(qrs))
							for i, qr := range qrs {
								btns[i] = whatsapp.Button{
									Type: "reply",
								}
								btns[i].Reply.ID = fmt.Sprint(i)
								btns[i].Reply.Title = qr
							}
							interactive.Action = &struct {
								Button   string             "json:\"button,omitempty\""
								Sections []whatsapp.Section "json:\"sections,omitempty\""
								Buttons  []whatsapp.Button  "json:\"buttons,omitempty\""
							}{Buttons: btns}
							payload.Interactive = &interactive
						} else {
							interactive := whatsapp.Interactive{Type: "list", Body: struct {
								Text string "json:\"text\""
							}{Text: msgParts[i-len(msg.Attachments())]}}

							section := whatsapp.Section{
								Rows: make([]whatsapp.SectionRow, len(qrs)),
							}
							for i, qr := range qrs {
								section.Rows[i] = whatsapp.SectionRow{
									ID:    fmt.Sprint(i),
									Title: qr,
								}
							}

							interactive.Action = &struct {
								Button   string             "json:\"button,omitempty\""
								Sections []whatsapp.Section "json:\"sections,omitempty\""
								Buttons  []whatsapp.Button  "json:\"buttons,omitempty\""
							}{Button: menuButton, Sections: []whatsapp.Section{
								section,
							}}

							payload.Interactive = &interactive
						}
					} else {
						// this is still a msg part
						text := &whatsapp.Text{PreviewURL: false}
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
				media := whatsapp.Media{Link: attURL}

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
						clog.Error(clogs.NewLogError("", "", "too many quick replies D3C supports only up to 10 quick replies"))
						qrs = qrs[:10]
					}

					// We can use buttons
					if len(qrs) <= 3 {
						interactive := whatsapp.Interactive{Type: "button", Body: struct {
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
								image := whatsapp.Media{
									Link: attURL,
								}
								interactive.Header = &struct {
									Type     string          "json:\"type\""
									Text     string          "json:\"text,omitempty\""
									Video    *whatsapp.Media "json:\"video,omitempty\""
									Image    *whatsapp.Media "json:\"image,omitempty\""
									Document *whatsapp.Media "json:\"document,omitempty\""
								}{Type: "image", Image: &image}
							} else if attType == "video" {
								video := whatsapp.Media{
									Link: attURL,
								}
								interactive.Header = &struct {
									Type     string          "json:\"type\""
									Text     string          "json:\"text,omitempty\""
									Video    *whatsapp.Media "json:\"video,omitempty\""
									Image    *whatsapp.Media "json:\"image,omitempty\""
									Document *whatsapp.Media "json:\"document,omitempty\""
								}{Type: "video", Video: &video}
							} else if attType == "document" {
								filename, err := utils.BasePathForURL(attURL)
								if err != nil {
									return err
								}
								document := whatsapp.Media{
									Link:     attURL,
									Filename: filename,
								}
								interactive.Header = &struct {
									Type     string          "json:\"type\""
									Text     string          "json:\"text,omitempty\""
									Video    *whatsapp.Media "json:\"video,omitempty\""
									Image    *whatsapp.Media "json:\"image,omitempty\""
									Document *whatsapp.Media "json:\"document,omitempty\""
								}{Type: "document", Document: &document}
							} else if attType == "audio" {
								payloadAudio = whatsapp.SendRequest{MessagingProduct: "whatsapp", RecipientType: "individual", To: msg.URN().Path(), Type: "audio", Audio: &whatsapp.Media{Link: attURL}}
								err := h.requestD3C(payloadAudio, accessToken, res, sendURL, clog)
								if err != nil {
									return nil
								}
							} else {
								interactive.Type = "button"
								interactive.Body.Text = msgParts[i]
							}
						}

						btns := make([]whatsapp.Button, len(qrs))
						for i, qr := range qrs {
							btns[i] = whatsapp.Button{
								Type: "reply",
							}
							btns[i].Reply.ID = fmt.Sprint(i)
							btns[i].Reply.Title = qr
						}
						interactive.Action = &struct {
							Button   string             "json:\"button,omitempty\""
							Sections []whatsapp.Section "json:\"sections,omitempty\""
							Buttons  []whatsapp.Button  "json:\"buttons,omitempty\""
						}{Buttons: btns}
						payload.Interactive = &interactive

					} else {
						interactive := whatsapp.Interactive{Type: "list", Body: struct {
							Text string "json:\"text\""
						}{Text: msgParts[i-len(msg.Attachments())]}}

						section := whatsapp.Section{
							Rows: make([]whatsapp.SectionRow, len(qrs)),
						}
						for i, qr := range qrs {
							section.Rows[i] = whatsapp.SectionRow{
								ID:    fmt.Sprint(i),
								Title: qr,
							}
						}

						interactive.Action = &struct {
							Button   string             "json:\"button,omitempty\""
							Sections []whatsapp.Section "json:\"sections,omitempty\""
							Buttons  []whatsapp.Button  "json:\"buttons,omitempty\""
						}{Button: menuButton, Sections: []whatsapp.Section{
							section,
						}}

						payload.Interactive = &interactive
					}
				} else {
					// this is still a msg part
					text := &whatsapp.Text{PreviewURL: false}
					payload.Type = "text"
					if strings.Contains(msgParts[i-len(msg.Attachments())], "https://") || strings.Contains(msgParts[i-len(msg.Attachments())], "http://") {
						text.PreviewURL = true
					}
					text.Body = msgParts[i-len(msg.Attachments())]
					payload.Text = text
				}
			}

			err := h.requestD3C(payload, accessToken, res, sendURL, clog)
			if err != nil {
				return err
			}

			if hasCaption {
				break
			}
		}
	}

	return nil
}

func (h *handler) requestD3C(payload whatsapp.SendRequest, accessToken string, res *courier.SendResult, wacPhoneURL *url.URL, clog *courier.ChannelLog) error {
	jsonBody := jsonx.MustMarshal(payload)

	req, err := http.NewRequest(http.MethodPost, wacPhoneURL.String(), bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}

	req.Header.Set(d3AuthorizationKey, accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 == 5 {
		return courier.ErrConnectionFailed
	}
	respPayload := &whatsapp.SendResponse{}
	err = json.Unmarshal(respBody, respPayload)
	if err != nil {
		return courier.ErrResponseUnparseable
	}

	if respPayload.Error.Code != 0 {
		return courier.ErrFailedWithReason(strconv.Itoa(respPayload.Error.Code), respPayload.Error.Message)
	}

	externalID := respPayload.Messages[0].ID
	if externalID != "" {
		res.AddExternalID(externalID)
	}
	return nil
}
