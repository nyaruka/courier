package whatsapp_legacy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/redisx"
	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	"golang.org/x/mod/semver"
)

const (
	configNamespace  = "fb_namespace"
	configHSMSupport = "hsm_support"

	d3AuthorizationKey = "D360-API-KEY"

	channelTypeWa  = "WA"
	channelTypeD3  = "D3"
	channelTypeTXW = "TXW"

	mediaCacheKeyPattern = "whatsapp_media_%s"

	interactiveMsgMinSupVersion = "v2.35.2"
)

var (
	retryParam = ""

	failedMediaCache *cache.Cache
)

func init() {
	courier.RegisterHandler(newWAHandler(courier.ChannelType(channelTypeWa), "WhatsApp"))
	courier.RegisterHandler(newWAHandler(courier.ChannelType(channelTypeD3), "360Dialog"))
	courier.RegisterHandler(newWAHandler(courier.ChannelType(channelTypeTXW), "TextIt"))

	failedMediaCache = cache.New(15*time.Minute, 15*time.Minute)
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
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMultiReceive, handlers.JSONPayload(h, h.receiveEvents))
	return nil
}

//	{
//	  "statuses": [{
//	    "id": "9712A34B4A8B6AD50F",
//	    "recipient_id": "16315555555",
//	    "status": "sent",
//	    "timestamp": "1518694700"
//	  }],
//	  "messages": [ {
//	    "from": "16315555555",
//	    "id": "3AF99CB6BE490DCAF641",
//	    "timestamp": "1518694235",
//	    "text": {
//	      "body": "Hello this is an answer"
//	    },
//	    "type": "text"
//	  }]
//	}
type eventsPayload struct {
	Contacts []struct {
		Profile struct {
			Name string `json:"name"`
		} `json:"profile"`
		WaID string `json:"wa_id"`
	} `json:"contacts"`
	Messages []struct {
		From      string `json:"from"      validate:"required"`
		ID        string `json:"id"        validate:"required"`
		Timestamp string `json:"timestamp" validate:"required"`
		Type      string `json:"type"      validate:"required"`
		Text      struct {
			Body string `json:"body"`
		} `json:"text"`
		Audio *struct {
			File     string `json:"file"      validate:"required"`
			ID       string `json:"id"        validate:"required"`
			Link     string `json:"link"`
			MimeType string `json:"mime_type" validate:"required"`
			Sha256   string `json:"sha256"    validate:"required"`
		} `json:"audio"`
		Button *struct {
			Payload string `json:"payload"`
			Text    string `json:"text"    validate:"required"`
		} `json:"button"`
		Document *struct {
			File     string `json:"file"      validate:"required"`
			ID       string `json:"id"        validate:"required"`
			Link     string `json:"link"`
			MimeType string `json:"mime_type" validate:"required"`
			Sha256   string `json:"sha256"    validate:"required"`
			Caption  string `json:"caption"`
			Filename string `json:"filename"`
		} `json:"document"`
		Image *struct {
			File     string `json:"file"      validate:"required"`
			ID       string `json:"id"        validate:"required"`
			Link     string `json:"link"`
			MimeType string `json:"mime_type" validate:"required"`
			Sha256   string `json:"sha256"    validate:"required"`
			Caption  string `json:"caption"`
		} `json:"image"`
		Interactive *struct {
			ButtonReply *struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"button_reply"`
			ListReply *struct {
				ID          string `json:"id"`
				Title       string `json:"title"`
				Description string `json:"description"`
			} `json:"list_reply"`
			Type string `json:"type"`
		}
		Location *struct {
			Address   string  `json:"address"   validate:"required"`
			Latitude  float32 `json:"latitude"  validate:"required"`
			Longitude float32 `json:"longitude" validate:"required"`
			Name      string  `json:"name"      validate:"required"`
			URL       string  `json:"url"       validate:"required"`
		} `json:"location"`
		Video *struct {
			File     string `json:"file"      validate:"required"`
			ID       string `json:"id"        validate:"required"`
			Link     string `json:"link"`
			MimeType string `json:"mime_type" validate:"required"`
			Sha256   string `json:"sha256"    validate:"required"`
		} `json:"video"`
		Voice *struct {
			File     string `json:"file"      validate:"required"`
			ID       string `json:"id"        validate:"required"`
			Link     string `json:"link"`
			MimeType string `json:"mime_type" validate:"required"`
			Sha256   string `json:"sha256"    validate:"required"`
		} `json:"voice"`
	} `json:"messages"`
	Statuses []struct {
		ID        string `json:"id"           validate:"required"`
		Timestamp string `json:"timestamp"    validate:"required"`
		Status    string `json:"status"       validate:"required"`
	} `json:"statuses"`
}

// receiveEvents is our HTTP handler function for incoming messages and status updates
func (h *handler) receiveEvents(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *eventsPayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	events := make([]courier.Event, 0, 2)

	// the list of data we will return in our response
	data := make([]any, 0, 2)

	seenMsgIDs := make(map[string]bool, 2)

	var contactNames = make(map[string]string)
	for _, contact := range payload.Contacts {
		contactNames[contact.WaID] = contact.Profile.Name
	}

	// first deal with any received messages
	for _, msg := range payload.Messages {
		if seenMsgIDs[msg.ID] {
			continue
		}

		// create our date from the timestamp
		ts, err := strconv.ParseInt(msg.Timestamp, 10, 64)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("invalid timestamp: %s", msg.Timestamp))
		}
		date := time.Unix(ts, 0).UTC()

		// create our URN
		urn, err := urns.NewWhatsAppURN(msg.From)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}

		text := ""
		mediaURL := ""

		if msg.Type == "text" {
			text = msg.Text.Body
		} else if msg.Type == "audio" && msg.Audio != nil {
			mediaURL, err = resolveMediaURL(channel, msg.Audio.ID)
		} else if msg.Type == "button" && msg.Button != nil {
			text = msg.Button.Text
		} else if msg.Type == "document" && msg.Document != nil {
			text = msg.Document.Caption
			mediaURL, err = resolveMediaURL(channel, msg.Document.ID)
		} else if msg.Type == "image" && msg.Image != nil {
			text = msg.Image.Caption
			mediaURL, err = resolveMediaURL(channel, msg.Image.ID)
		} else if msg.Type == "interactive" {
			if msg.Interactive.Type == "button_reply" {
				text = msg.Interactive.ButtonReply.Title
			} else {
				text = msg.Interactive.ListReply.Title
			}
		} else if msg.Type == "location" && msg.Location != nil {
			mediaURL = fmt.Sprintf("geo:%f,%f", msg.Location.Latitude, msg.Location.Longitude)
		} else if msg.Type == "video" && msg.Video != nil {
			mediaURL, err = resolveMediaURL(channel, msg.Video.ID)
		} else if msg.Type == "voice" && msg.Voice != nil {
			mediaURL, err = resolveMediaURL(channel, msg.Voice.ID)
		} else {
			// we received a message type we do not support.
			courier.LogRequestError(r, channel, fmt.Errorf("unsupported message type %s", msg.Type))
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
			return nil, err
		}

		events = append(events, event)
		data = append(data, courier.NewMsgReceiveData(event))
		seenMsgIDs[msg.ID] = true
	}

	// now with any status updates
	for _, status := range payload.Statuses {
		msgStatus, found := waStatusMapping[status.Status]
		if !found {
			if waIgnoreStatuses[status.Status] {
				data = append(data, courier.NewInfoData(fmt.Sprintf("ignoring status: %s", status.Status)))
			} else {
				handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unknown status: %s", status.Status))
			}
			continue
		}

		event := h.Backend().NewStatusUpdateByExternalID(channel, status.ID, msgStatus, clog)
		err := h.Backend().WriteStatusUpdate(ctx, event)
		if err != nil {
			return nil, err
		}

		events = append(events, event)
		data = append(data, courier.NewStatusData(event))
	}

	return events, courier.WriteDataResponse(w, http.StatusOK, "Events Handled", data)
}

func resolveMediaURL(channel courier.Channel, mediaID string) (string, error) {
	// sometimes WA will send an attachment with status=undownloaded and no ID
	if mediaID == "" {
		return "", nil
	}

	urlStr := channel.StringConfigForKey(courier.ConfigBaseURL, "")
	url, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("invalid base url set for WA channel: %s", err)
	}

	mediaPath, _ := url.Parse("/v1/media")
	mediaEndpoint := url.ResolveReference(mediaPath).String()

	fileURL := fmt.Sprintf("%s/%s", mediaEndpoint, mediaID)

	return fileURL, nil
}

// BuildAttachmentRequest to download media for message attachment with Bearer token set
func (h *handler) BuildAttachmentRequest(ctx context.Context, b courier.Backend, channel courier.Channel, attachmentURL string, clog *courier.ChannelLog) (*http.Request, error) {
	token := channel.StringConfigForKey(courier.ConfigAuthToken, "")
	if token == "" {
		return nil, fmt.Errorf("missing token for WA channel")
	}

	// set the access token as the authorization header
	req, _ := http.NewRequest(http.MethodGet, attachmentURL, nil)
	setWhatsAppAuthHeader(&req.Header, channel)
	return req, nil
}

var _ courier.AttachmentRequestBuilder = (*handler)(nil)

var waStatusMapping = map[string]courier.MsgStatus{
	"sending":   courier.MsgStatusWired,
	"sent":      courier.MsgStatusSent,
	"delivered": courier.MsgStatusDelivered,
	"read":      courier.MsgStatusDelivered,
	"failed":    courier.MsgStatusFailed,
}

var waIgnoreStatuses = map[string]bool{
	"deleted": true,
}

// {
//   "to": "16315555555",
//   "type": "text | audio | document | image",
//   "text": {
//     "body": "text message"
//   }
//	 "audio": {
//     "id": "the-audio-id"
// 	 }
//	 "document": {
//     "id": "the-document-id"
//     "caption": "the optional document caption"
// 	 }
//	 "image": {
//     "id": "the-image-id"
//     "caption": "the optional image caption"
// 	 }
//	 "video": {
//     "id": "the-video-id"
//     "caption": "the optional video caption"
//   }
// }

type mtTextPayload struct {
	To         string `json:"to"    validate:"required"`
	Type       string `json:"type"  validate:"required"`
	PreviewURL bool   `json:"preview_url,omitempty"`
	Text       struct {
		Body string `json:"body" validate:"required"`
	} `json:"text"`
}

type mtInteractivePayload struct {
	To          string `json:"to" validate:"required"`
	Type        string `json:"type" validate:"required"`
	Interactive struct {
		Type   string `json:"type" validate:"required"` //"text" | "image" | "video" | "document"
		Header *struct {
			Type     string `json:"type"`
			Text     string `json:"text,omitempty"`
			Video    string `json:"video,omitempty"`
			Image    string `json:"image,omitempty"`
			Document string `json:"document,omitempty"`
		} `json:"header,omitempty"`
		Body struct {
			Text string `json:"text"`
		} `json:"body" validate:"required"`
		Footer *struct {
			Text string `json:"text"`
		} `json:"footer,omitempty"`
		Action struct {
			Button   string      `json:"button,omitempty"`
			Sections []mtSection `json:"sections,omitempty"`
			Buttons  []mtButton  `json:"buttons,omitempty"`
		} `json:"action" validate:"required"`
	} `json:"interactive"`
}

type mtSection struct {
	Title string         `json:"title,omitempty"`
	Rows  []mtSectionRow `json:"rows" validate:"required"`
}

type mtSectionRow struct {
	ID          string `json:"id" validate:"required"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

type mtButton struct {
	Type  string `json:"type" validate:"required"`
	Reply struct {
		ID    string `json:"id" validate:"required"`
		Title string `json:"title" validate:"required"`
	} `json:"reply" validate:"required"`
}

type mediaObject struct {
	ID       string `json:"id,omitempty"`
	Link     string `json:"link,omitempty"`
	Caption  string `json:"caption,omitempty"`
	Filename string `json:"filename,omitempty"`
}

type LocalizableParam struct {
	Default string `json:"default"`
}

type Param struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type Component struct {
	Type       string  `json:"type"`
	Parameters []Param `json:"parameters,omitempty"`
}

type templatePayload struct {
	To       string `json:"to"`
	Type     string `json:"type"`
	Template struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
		Language  struct {
			Policy string `json:"policy"`
			Code   string `json:"code"`
		} `json:"language"`
		Components []Component `json:"components"`
	} `json:"template"`
}

type hsmPayload struct {
	To   string `json:"to"`
	Type string `json:"type"`
	HSM  struct {
		Namespace   string `json:"namespace"`
		ElementName string `json:"element_name"`
		Language    struct {
			Policy string `json:"policy"`
			Code   string `json:"code"`
		} `json:"language"`
		LocalizableParams []LocalizableParam `json:"localizable_params"`
	} `json:"hsm"`
}

type mtAudioPayload struct {
	To    string       `json:"to"    validate:"required"`
	Type  string       `json:"type"  validate:"required"`
	Audio *mediaObject `json:"audio"`
}

type mtDocumentPayload struct {
	To       string       `json:"to"    validate:"required"`
	Type     string       `json:"type"  validate:"required"`
	Document *mediaObject `json:"document"`
}

type mtImagePayload struct {
	To    string       `json:"to"    validate:"required"`
	Type  string       `json:"type"  validate:"required"`
	Image *mediaObject `json:"image"`
}

type mtVideoPayload struct {
	To    string       `json:"to" validate:"required"`
	Type  string       `json:"type" validate:"required"`
	Video *mediaObject `json:"video"`
}

type mtErrorPayload struct {
	Errors []struct {
		Code    int    `json:"code"`
		Title   string `json:"title"`
		Details string `json:"details"`
	} `json:"errors"`
}

// whatsapp only allows messages up to 4096 chars
const maxMsgLength = 4096

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	conn := h.Backend().RedisPool().Get()
	defer conn.Close()

	// get our token
	token := msg.Channel().StringConfigForKey(courier.ConfigAuthToken, "")
	if token == "" {
		return nil, fmt.Errorf("missing token for WA channel")
	}

	urlStr := msg.Channel().StringConfigForKey(courier.ConfigBaseURL, "")
	url, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid base url set for WA channel: %s", err)
	}
	sendPath, _ := url.Parse("/v1/messages")

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)

	var wppID string

	payloads, err := buildPayloads(msg, h, clog)

	fail := payloads == nil && err != nil
	if fail {
		return nil, err
	}

	for i, payload := range payloads {
		externalID := ""
		wppID, externalID, err = h.sendWhatsAppMsg(conn, msg, sendPath, payload, clog)
		if err != nil {
			break
		}

		// if this is our first message, record the external id
		if i == 0 {
			status.SetExternalID(externalID)
		}
	}

	// we are wired it there were no errors
	if err == nil {
		// so update contact URN if wppID != ""
		if wppID != "" {
			newURN, _ := urns.NewWhatsAppURN(wppID)
			err = status.SetURNUpdate(msg.URN(), newURN)

			if err != nil {
				clog.RawError(err)
			}
		}
		status.SetStatus(courier.MsgStatusWired)
	}

	return status, nil
}

// WriteRequestError writes the passed in error to our response writer
func (h *handler) WriteRequestError(ctx context.Context, w http.ResponseWriter, err error) error {
	return courier.WriteError(w, http.StatusOK, err)
}

func buildPayloads(msg courier.MsgOut, h *handler, clog *courier.ChannelLog) ([]any, error) {
	var payloads []any
	var err error

	parts := handlers.SplitMsgByChannel(msg.Channel(), msg.Text(), maxMsgLength)

	qrs := msg.QuickReplies()
	langCode := getSupportedLanguage(msg.Locale())
	wppVersion := msg.Channel().ConfigForKey("version", "0").(string)
	isInteractiveMsgCompatible := semver.Compare(wppVersion, interactiveMsgMinSupVersion)
	isInteractiveMsg := (isInteractiveMsgCompatible >= 0) && (len(qrs) > 0)

	textAsCaption := false

	if len(msg.Attachments()) > 0 {
		for attachmentCount, attachment := range msg.Attachments() {

			mimeType, mediaURL := handlers.SplitAttachment(attachment)
			mediaID, err := h.fetchMediaID(msg, mimeType, mediaURL, clog)
			if err != nil {
				slog.Error("error while uploading media to whatsapp", "error", err, "channel_uuid", msg.Channel().UUID())
			}
			fileURL := mediaURL
			if err == nil && mediaID != "" {
				mediaURL = ""
			}
			mediaPayload := &mediaObject{ID: mediaID, Link: mediaURL}
			if strings.HasPrefix(mimeType, "audio") {
				payload := mtAudioPayload{
					To:   msg.URN().Path(),
					Type: "audio",
				}
				payload.Audio = mediaPayload
				payloads = append(payloads, payload)
			} else if strings.HasPrefix(mimeType, "application") {
				payload := mtDocumentPayload{
					To:   msg.URN().Path(),
					Type: "document",
				}
				if attachmentCount == 0 && !isInteractiveMsg {
					mediaPayload.Caption = msg.Text()
					textAsCaption = true
				}
				mediaPayload.Filename, err = utils.BasePathForURL(fileURL)

				// Logging error
				if err != nil {
					slog.Error("Error while parsing the media URL", "error", err, "channel_uuid", msg.Channel().UUID())
				}
				payload.Document = mediaPayload
				payloads = append(payloads, payload)
			} else if strings.HasPrefix(mimeType, "image") {
				payload := mtImagePayload{
					To:   msg.URN().Path(),
					Type: "image",
				}
				if attachmentCount == 0 && !isInteractiveMsg {
					mediaPayload.Caption = msg.Text()
					textAsCaption = true
				}
				payload.Image = mediaPayload
				payloads = append(payloads, payload)
			} else if strings.HasPrefix(mimeType, "video") {
				payload := mtVideoPayload{
					To:   msg.URN().Path(),
					Type: "video",
				}
				if attachmentCount == 0 && !isInteractiveMsg {
					mediaPayload.Caption = msg.Text()
					textAsCaption = true
				}
				payload.Video = mediaPayload
				payloads = append(payloads, payload)
			} else {
				clog.Error(courier.ErrorMediaUnsupported(mimeType))
				break
			}
		}

		if !textAsCaption && !isInteractiveMsg {
			for _, part := range parts {

				//check if you have a link
				var payload mtTextPayload
				if strings.Contains(part, "https://") || strings.Contains(part, "http://") {
					payload = mtTextPayload{
						To:         msg.URN().Path(),
						Type:       "text",
						PreviewURL: true,
					}
				} else {
					payload = mtTextPayload{
						To:   msg.URN().Path(),
						Type: "text",
					}
				}
				payload.Text.Body = part
				payloads = append(payloads, payload)
			}
		}

		if isInteractiveMsg {
			for i, part := range parts {
				if i < (len(parts) - 1) { //if split into more than one message, the first parts will be text and the last interactive
					payload := mtTextPayload{
						To:   msg.URN().Path(),
						Type: "text",
					}
					payload.Text.Body = part
					payloads = append(payloads, payload)

				} else {
					payload := mtInteractivePayload{
						To:   msg.URN().Path(),
						Type: "interactive",
					}

					// up to 3 qrs the interactive message will be button type, otherwise it will be list
					if len(qrs) <= 3 {
						payload.Interactive.Type = "button"
						payload.Interactive.Body.Text = part
						btns := make([]mtButton, len(qrs))
						for i, qr := range qrs {
							btns[i] = mtButton{
								Type: "reply",
							}
							btns[i].Reply.ID = fmt.Sprint(i)
							btns[i].Reply.Title = qr
						}
						payload.Interactive.Action.Buttons = btns
						payloads = append(payloads, payload)
					} else {
						payload.Interactive.Type = "list"
						payload.Interactive.Body.Text = part
						payload.Interactive.Action.Button = "Menu"
						section := mtSection{
							Rows: make([]mtSectionRow, len(qrs)),
						}
						for i, qr := range qrs {
							section.Rows[i] = mtSectionRow{
								ID:    fmt.Sprint(i),
								Title: qr,
							}
						}
						payload.Interactive.Action.Sections = []mtSection{
							section,
						}
						payloads = append(payloads, payload)
					}
				}
			}
		}

	} else {
		// do we have a template?
		templating, err := h.getTemplating(msg)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to decode template: %s for channel: %s", string(msg.Metadata()), msg.Channel().UUID())
		}
		if templating != nil {
			namespace := templating.Namespace
			if namespace == "" {
				namespace = msg.Channel().StringConfigForKey(configNamespace, "")
			}
			if namespace == "" {
				return nil, errors.Errorf("cannot send template message without Facebook namespace for channel: %s", msg.Channel().UUID())
			}

			payload := templatePayload{
				To:   msg.URN().Path(),
				Type: "template",
			}
			payload.Template.Namespace = namespace
			payload.Template.Name = templating.Template.Name
			payload.Template.Language.Policy = "deterministic"
			payload.Template.Language.Code = langCode

			component := &Component{Type: "body"}

			for _, v := range templating.Variables {
				component.Parameters = append(component.Parameters, Param{Type: "text", Text: v})
			}
			payload.Template.Components = append(payload.Template.Components, *component)

			payloads = append(payloads, payload)

		} else {

			if isInteractiveMsg {
				for i, part := range parts {
					if i < (len(parts) - 1) { //if split into more than one message, the first parts will be text and the last interactive
						payload := mtTextPayload{
							To:   msg.URN().Path(),
							Type: "text",
						}
						payload.Text.Body = part
						payloads = append(payloads, payload)

					} else {
						payload := mtInteractivePayload{
							To:   msg.URN().Path(),
							Type: "interactive",
						}

						// up to 3 qrs the interactive message will be button type, otherwise it will be list
						if len(qrs) <= 3 {
							payload.Interactive.Type = "button"
							payload.Interactive.Body.Text = part
							btns := make([]mtButton, len(qrs))
							for i, qr := range qrs {
								btns[i] = mtButton{
									Type: "reply",
								}
								btns[i].Reply.ID = fmt.Sprint(i)
								btns[i].Reply.Title = qr
							}
							payload.Interactive.Action.Buttons = btns
							payloads = append(payloads, payload)
						} else {
							payload.Interactive.Type = "list"
							payload.Interactive.Body.Text = part
							payload.Interactive.Action.Button = "Menu"
							section := mtSection{
								Rows: make([]mtSectionRow, len(qrs)),
							}
							for i, qr := range qrs {
								section.Rows[i] = mtSectionRow{
									ID:    fmt.Sprint(i),
									Title: qr,
								}
							}
							payload.Interactive.Action.Sections = []mtSection{
								section,
							}
							payloads = append(payloads, payload)
						}
					}
				}
			} else {
				for _, part := range parts {

					//check if you have a link
					var payload mtTextPayload
					if strings.Contains(part, "https://") || strings.Contains(part, "http://") {
						payload = mtTextPayload{
							To:         msg.URN().Path(),
							Type:       "text",
							PreviewURL: true,
						}
					} else {
						payload = mtTextPayload{
							To:   msg.URN().Path(),
							Type: "text",
						}
					}
					payload.Text.Body = part
					payloads = append(payloads, payload)
				}
			}
		}
	}
	return payloads, err
}

// fetchMediaID tries to fetch the id for the uploaded media, setting the result in redis.
func (h *handler) fetchMediaID(msg courier.MsgOut, mimeType, mediaURL string, clog *courier.ChannelLog) (string, error) {
	// check in cache first
	rc := h.Backend().RedisPool().Get()
	defer rc.Close()

	cacheKey := fmt.Sprintf(mediaCacheKeyPattern, msg.Channel().UUID())
	mediaCache := redisx.NewIntervalHash(cacheKey, time.Hour*24, 2)
	mediaID, err := mediaCache.Get(rc, mediaURL)
	if err != nil {
		return "", errors.Wrapf(err, "error reading media id from redis: %s : %s", cacheKey, mediaURL)
	} else if mediaID != "" {
		return mediaID, nil
	}

	// check in failure cache
	failKey := fmt.Sprintf("%s-%s", msg.Channel().UUID(), mediaURL)
	found, _ := failedMediaCache.Get(failKey)

	// any non nil value means we cached a failure, don't try again until our cache expires
	if found != nil {
		return "", nil
	}

	// download media
	req, err := http.NewRequest("GET", mediaURL, nil)
	if err != nil {
		return "", errors.Wrapf(err, "error building media request")
	}

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		failedMediaCache.Set(failKey, true, cache.DefaultExpiration)
		return "", nil
	}

	// upload media to WhatsApp
	baseURL := msg.Channel().StringConfigForKey(courier.ConfigBaseURL, "")
	url, err := url.Parse(baseURL)
	if err != nil {
		return "", errors.Wrapf(err, "invalid base url set for WA channel: %s", baseURL)
	}
	dockerMediaURL, _ := url.Parse("/v1/media")

	req, err = http.NewRequest("POST", dockerMediaURL.String(), bytes.NewReader(respBody))
	if err != nil {
		return "", errors.Wrapf(err, "error building request to media endpoint")
	}
	setWhatsAppAuthHeader(&req.Header, msg.Channel())
	mediaType, _ := httpx.DetectContentType(respBody)
	req.Header.Add("Content-Type", mediaType)

	resp, respBody, err = h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		failedMediaCache.Set(failKey, true, cache.DefaultExpiration)
		return "", errors.Wrapf(err, "error uploading media to whatsapp")
	}

	// take uploaded media id
	mediaID, err = jsonparser.GetString(respBody, "media", "[0]", "id")
	if err != nil {
		return "", errors.Wrapf(err, "error reading media id from response")
	}

	// put in cache
	err = mediaCache.Set(rc, mediaURL, mediaID)
	if err != nil {
		return "", errors.Wrapf(err, "error setting media id in cache")
	}

	return mediaID, nil
}

func (h *handler) sendWhatsAppMsg(rc redis.Conn, msg courier.MsgOut, sendPath *url.URL, payload any, clog *courier.ChannelLog) (string, string, error) {
	jsonBody := jsonx.MustMarshal(payload)

	req, _ := http.NewRequest(http.MethodPost, sendPath.String(), bytes.NewReader(jsonBody))
	req.Header = buildWhatsAppHeaders(msg.Channel())

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil {
		return "", "", err
	}

	if resp != nil && (resp.StatusCode == 429 || resp.StatusCode == 503) {
		rateLimitKey := fmt.Sprintf("rate_limit:%s", msg.Channel().UUID())
		rc.Do("SET", rateLimitKey, "engaged")

		// The rate limit is 50 requests per second
		// We pause sending 2 seconds so the limit count is reset
		// TODO: In the future we should the header value when available
		rc.Do("EXPIRE", rateLimitKey, 2)

		return "", "", errors.New("received rate-limit response from send endpoint")
	}

	errPayload := &mtErrorPayload{}
	err = json.Unmarshal(respBody, errPayload)

	// handle send msg errors
	if err == nil && len(errPayload.Errors) > 0 {
		if hasTiersError(*errPayload) {
			rateLimitBulkKey := fmt.Sprintf("rate_limit_bulk:%s", msg.Channel().UUID())
			rc.Do("SET", rateLimitBulkKey, "engaged")

			// The WA tiers spam rate limit hit
			// We pause the bulk queue for 24 hours and 5min
			rc.Do("EXPIRE", rateLimitBulkKey, (60*60*24)+(5*60))

			err := errors.Errorf("received error from send endpoint: %s", errPayload.Errors[0].Title)
			return "", "", err
		}

		if !hasWhatsAppContactError(*errPayload) {
			err := errors.Errorf("received error from send endpoint: %s", errPayload.Errors[0].Title)
			return "", "", err
		}
		// check contact
		baseURL := fmt.Sprintf("%s://%s", sendPath.Scheme, sendPath.Host)
		checkResp, err := h.checkWhatsAppContact(msg.Channel(), baseURL, msg.URN(), clog)
		if checkResp == nil {
			return "", "", err
		}
		if err != nil {
			return "", "", err
		}
		// update contact URN and msg destiny with returned wpp id
		wppID, err := jsonparser.GetString(checkResp, "contacts", "[0]", "wa_id")

		if err == nil {
			var updatedPayload any

			// handle msg type casting
			switch v := payload.(type) {
			case mtTextPayload:
				v.To = wppID
				updatedPayload = v
			case mtImagePayload:
				v.To = wppID
				updatedPayload = v
			case mtVideoPayload:
				v.To = wppID
				updatedPayload = v
			case mtAudioPayload:
				v.To = wppID
				updatedPayload = v
			case mtDocumentPayload:
				v.To = wppID
				updatedPayload = v
			case templatePayload:
				v.To = wppID
				updatedPayload = v
			case hsmPayload:
				v.To = wppID
				updatedPayload = v
			}
			// marshal updated payload
			if updatedPayload != nil {
				payload = updatedPayload
				jsonBody = jsonx.MustMarshal(payload)
			}
		}
		// try send msg again
		reqRetry, err := http.NewRequest(http.MethodPost, sendPath.String(), bytes.NewReader(jsonBody))
		if err != nil {
			return "", "", err
		}
		reqRetry.Header = buildWhatsAppHeaders(msg.Channel())

		if retryParam != "" {
			reqRetry.URL.RawQuery = fmt.Sprintf("%s=1", retryParam)
		}

		retryResp, retryRespBody, err := h.RequestHTTP(reqRetry, clog)
		if err != nil || retryResp.StatusCode/100 != 2 {
			return "", "", errors.New("error making retry request")
		}
		externalID, err := getSendWhatsAppMsgId(retryRespBody)
		return wppID, externalID, err
	}
	externalID, err := getSendWhatsAppMsgId(respBody)
	if err != nil {
		return "", "", err
	}
	wppID, err := jsonparser.GetString(respBody, "contacts", "[0]", "wa_id")
	if wppID != "" && wppID != msg.URN().Path() {
		return wppID, externalID, err
	}
	return "", externalID, nil
}

func setWhatsAppAuthHeader(header *http.Header, channel courier.Channel) {
	authToken := channel.StringConfigForKey(courier.ConfigAuthToken, "")

	if channel.ChannelType() == channelTypeD3 {
		header.Set(d3AuthorizationKey, authToken)
	} else {
		header.Set("Authorization", fmt.Sprintf("Bearer %s", authToken))
	}
}

func buildWhatsAppHeaders(channel courier.Channel) http.Header {
	header := http.Header{
		"Content-Type": []string{"application/json"},
		"Accept":       []string{"application/json"},
	}
	setWhatsAppAuthHeader(&header, channel)
	return header
}

func hasTiersError(payload mtErrorPayload) bool {
	for _, err := range payload.Errors {
		if err.Code == 471 {
			return true
		}
	}
	return false
}

func hasWhatsAppContactError(payload mtErrorPayload) bool {
	for _, err := range payload.Errors {
		if err.Code == 1006 && err.Title == "Resource not found" && (err.Details == "unknown contact" || err.Details == "Could not retrieve phone number from contact store") {
			return true
		}
	}
	return false
}

func getSendWhatsAppMsgId(resp []byte) (string, error) {
	if externalID, err := jsonparser.GetString(resp, "messages", "[0]", "id"); err == nil {
		return externalID, nil
	} else {
		return "", errors.Errorf("unable to get message id from response body")
	}
}

type mtContactCheckPayload struct {
	Blocking   string   `json:"blocking"`
	Contacts   []string `json:"contacts"`
	ForceCheck bool     `json:"force_check"`
}

func (h *handler) checkWhatsAppContact(channel courier.Channel, baseURL string, urn urns.URN, clog *courier.ChannelLog) ([]byte, error) {
	payload := mtContactCheckPayload{
		Blocking:   "wait",
		Contacts:   []string{fmt.Sprintf("+%s", urn.Path())},
		ForceCheck: true,
	}
	reqBody := jsonx.MustMarshal(payload)
	sendURL := fmt.Sprintf("%s/v1/contacts", baseURL)
	req, _ := http.NewRequest(http.MethodPost, sendURL, bytes.NewReader(reqBody))
	req.Header = buildWhatsAppHeaders(channel)

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return nil, errors.New("error checking contact")
	}
	// check contact status
	if status, err := jsonparser.GetString(respBody, "contacts", "[0]", "status"); err == nil {
		if status == "valid" {
			return respBody, nil
		} else {
			return respBody, errors.Errorf(`contact status is "%s"`, status)
		}
	} else {
		return respBody, err
	}
}

func (h *handler) getTemplating(msg courier.MsgOut) (*MsgTemplating, error) {
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

func getSupportedLanguage(lc i18n.Locale) string {
	// look for exact match
	if lang := supportedLanguages[lc]; lang != "" {
		return lang
	}

	// if we have a country, strip that off and look again for a match
	l, c := lc.Split()
	if c != "" {
		if lang := supportedLanguages[i18n.Locale(l)]; lang != "" {
			return lang
		}
	}
	return "en" // fallback to English
}

// Mapping from engine locales to supported languages, see https://developers.facebook.com/docs/whatsapp/api/messages/message-templates/
var supportedLanguages = map[i18n.Locale]string{
	"afr":    "af",    // Afrikaans
	"sqi":    "sq",    // Albanian
	"ara":    "ar",    // Arabic
	"aze":    "az",    // Azerbaijani
	"ben":    "bn",    // Bengali
	"bul":    "bg",    // Bulgarian
	"cat":    "ca",    // Catalan
	"zho":    "zh_CN", // Chinese
	"zho-CN": "zh_CN", // Chinese (CHN)
	"zho-HK": "zh_HK", // Chinese (HKG)
	"zho-TW": "zh_TW", // Chinese (TAI)
	"hrv":    "hr",    // Croatian
	"ces":    "cs",    // Czech
	"dah":    "da",    // Danish
	"nld":    "nl",    // Dutch
	"eng":    "en",    // English
	"eng-GB": "en_GB", // English (UK)
	"eng-US": "en_US", // English (US)
	"est":    "et",    // Estonian
	"fil":    "fil",   // Filipino
	"fin":    "fi",    // Finnish
	"fra":    "fr",    // French
	"kat":    "ka",    // Georgian
	"deu":    "de",    // German
	"ell":    "el",    // Greek
	"guj":    "gu",    // Gujarati
	"hau":    "ha",    // Hausa
	"enb":    "he",    // Hebrew
	"hin":    "hi",    // Hindi
	"hun":    "hu",    // Hungarian
	"ind":    "id",    // Indonesian
	"gle":    "ga",    // Irish
	"ita":    "it",    // Italian
	"jpn":    "ja",    // Japanese
	"kan":    "kn",    // Kannada
	"kaz":    "kk",    // Kazakh
	"kin":    "rw_RW", // Kinyarwanda
	"kor":    "ko",    // Korean
	"kir":    "ky_KG", // Kyrgyzstan
	"lao":    "lo",    // Lao
	"lav":    "lv",    // Latvian
	"lit":    "lt",    // Lithuanian
	"mal":    "ml",    // Malayalam
	"mkd":    "mk",    // Macedonian
	"msa":    "ms",    // Malay
	"mar":    "mr",    // Marathi
	"nob":    "nb",    // Norwegian
	"fas":    "fa",    // Persian
	"pol":    "pl",    // Polish
	"por":    "pt_PT", // Portuguese
	"por-BR": "pt_BR", // Portuguese (BR)
	"por-PT": "pt_PT", // Portuguese (POR)
	"pan":    "pa",    // Punjabi
	"ron":    "ro",    // Romanian
	"rus":    "ru",    // Russian
	"srp":    "sr",    // Serbian
	"slk":    "sk",    // Slovak
	"slv":    "sl",    // Slovenian
	"spa":    "es",    // Spanish
	"spa-AR": "es_AR", // Spanish (ARG)
	"spa-ES": "es_ES", // Spanish (SPA)
	"spa-MX": "es_MX", // Spanish (MEX)
	"swa":    "sw",    // Swahili
	"swe":    "sv",    // Swedish
	"tam":    "ta",    // Tamil
	"tel":    "te",    // Telugu
	"tha":    "th",    // Thai
	"tur":    "tr",    // Turkish
	"ukr":    "uk",    // Ukrainian
	"urd":    "ur",    // Urdu
	"uzb":    "uz",    // Uzbek
	"vie":    "vi",    // Vietnamese
	"zul":    "zu",    // Zulu
}
