package turn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/handlers/meta/whatsapp"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/vkutil"
	"github.com/patrickmn/go-cache"
)

var (
	// max for the body
	maxMsgLength    = 4096
	configNamespace = "fb_namespace"

	mediaCacheKeyPattern = "turn_whatsapp_media_%s"
	failedMediaCache     *cache.Cache
)

func init() {
	courier.RegisterHandler(newHandler())

	failedMediaCache = cache.New(15*time.Minute, 15*time.Minute)
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(models.ChannelType("TRN"), "Turn.io WhatsApp")}
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
		urn, err := urns.New(urns.WhatsApp, msg.From)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, errors.New("invalid whatsapp id"))
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
		} else if msg.Type == "interactive" && msg.Interactive != nil {
			if msg.Interactive.Type == "button_reply" && msg.Interactive.ButtonReply != nil {
				text = msg.Interactive.ButtonReply.Title
			} else if msg.Interactive.Type == "list_reply" && msg.Interactive.ListReply != nil {
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
		event := h.Backend().NewIncomingMsg(ctx, channel, urn, text, msg.ID, clog).WithReceivedOn(date).WithContactName(contactNames[msg.From])

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
		msgStatus, found := turnWaStatusMapping[status.Status]
		if !found {
			if turnWaIgnoreStatuses[status.Status] {
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

	urlStr := channel.StringConfigForKey(models.ConfigBaseURL, "")
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
	token := channel.StringConfigForKey(models.ConfigAuthToken, "")
	if token == "" {
		return nil, fmt.Errorf("missing token for TRN channel")
	}

	// set the access token as the authorization header
	req, _ := http.NewRequest(http.MethodGet, attachmentURL, nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	return req, nil
}

var _ courier.AttachmentRequestBuilder = (*handler)(nil)

var turnWaStatusMapping = map[string]models.MsgStatus{
	"sending":   models.MsgStatusWired,
	"sent":      models.MsgStatusSent,
	"delivered": models.MsgStatusDelivered,
	"read":      models.MsgStatusRead,
	"failed":    models.MsgStatusFailed,
}

var turnWaIgnoreStatuses = map[string]bool{
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
		Components []Component `json:"components,omitempty"`
	} `json:"template"`
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

func buildPayloads(ctx context.Context, msg courier.MsgOut, h *handler, clog *courier.ChannelLog) ([]any, error) {
	var payloads []any
	var err error

	parts := handlers.SplitMsgByChannel(msg.Channel(), msg.Text(), maxMsgLength)

	qrs := msg.QuickReplies()
	qrsAsList := false
	for i, qr := range qrs {
		if i > 2 || qr.Extra != "" {
			qrsAsList = true
		}
	}
	isInteractiveMsg := len(qrs) > 0

	textAsCaption := false

	if len(msg.Attachments()) > 0 {
		for attachmentCount, attachment := range msg.Attachments() {

			mimeType, mediaURL := handlers.SplitAttachment(attachment)
			mediaID, err := h.fetchMediaID(ctx, msg, mediaURL, clog)
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
			} else if strings.HasPrefix(mimeType, "application") || strings.HasPrefix(mimeType, "document") {
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

					// we show buttons
					if !qrsAsList {
						payload.Interactive.Type = "button"
						payload.Interactive.Body.Text = part
						btns := make([]mtButton, len(qrs))
						for btnIdx, qr := range qrs {
							btns[btnIdx] = mtButton{
								Type: "reply",
							}
							btns[btnIdx].Reply.ID = fmt.Sprint(btnIdx)
							btns[btnIdx].Reply.Title = qr.Text
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
						for rowIdx, qr := range qrs {
							section.Rows[rowIdx] = mtSectionRow{
								ID:          fmt.Sprint(rowIdx),
								Title:       qr.Text,
								Description: qr.Extra,
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
		if msg.Templating() != nil {
			langCode := getSupportedLanguage(msg.Locale())
			namespace := msg.Templating().Namespace
			if namespace == "" {
				namespace = msg.Channel().StringConfigForKey(configNamespace, "")
			}
			if namespace == "" {
				return nil, fmt.Errorf("cannot send template message without Facebook namespace for channel: %s", msg.Channel().UUID())
			}

			payload := templatePayload{
				To:   msg.URN().Path(),
				Type: "template",
			}
			payload.Template.Namespace = namespace
			payload.Template.Name = msg.Templating().Template.Name
			payload.Template.Language.Policy = "deterministic"
			payload.Template.Language.Code = langCode

			for _, comp := range msg.Templating().Components {
				// get the variables used by this component in order of their names 1, 2 etc
				compParams := make([]models.TemplatingVariable, 0, len(comp.Variables))

				for _, varName := range slices.Sorted(maps.Keys(comp.Variables)) {
					compParams = append(compParams, msg.Templating().Variables[comp.Variables[varName]])
				}

				if comp.Type == "body" || strings.HasPrefix(comp.Type, "body/") {
					component := &Component{Type: "body"}
					for _, p := range compParams {
						component.Parameters = append(component.Parameters, Param{Type: p.Type, Text: p.Value})
					}
					payload.Template.Components = append(payload.Template.Components, *component)

				}

			}

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

						// we show buttons
						if !qrsAsList {
							payload.Interactive.Type = "button"
							payload.Interactive.Body.Text = part
							btns := make([]mtButton, len(qrs))
							for i, qr := range qrs {
								btns[i] = mtButton{
									Type: "reply",
								}
								btns[i].Reply.ID = fmt.Sprint(i)
								btns[i].Reply.Title = qr.Text
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
									ID:          fmt.Sprint(i),
									Title:       qr.Text,
									Description: qr.Extra,
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
func (h *handler) fetchMediaID(ctx context.Context, msg courier.MsgOut, mediaURL string, clog *courier.ChannelLog) (string, error) {
	// check in cache first
	cacheKey := fmt.Sprintf(mediaCacheKeyPattern, msg.Channel().UUID())
	mediaCache := vkutil.NewIntervalHash(cacheKey, time.Hour*24, 2)

	var mediaID string
	var err error
	h.WithValkeyConn(func(rc redis.Conn) {
		mediaID, err = mediaCache.Get(ctx, rc, mediaURL)
	})

	if err != nil {
		return "", fmt.Errorf("error reading media id from valkey: %s : %s: %w", cacheKey, mediaURL, err)
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
		return "", fmt.Errorf("error building media request: %w", err)
	}

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		failedMediaCache.Set(failKey, true, cache.DefaultExpiration)
		return "", nil
	}

	// upload media to WhatsApp
	baseURL := msg.Channel().StringConfigForKey(models.ConfigBaseURL, "")
	url, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base url set for WA channel: %s: %w", baseURL, err)
	}
	dockerMediaURL, _ := url.Parse("/v1/media")

	req, err = http.NewRequest("POST", dockerMediaURL.String(), bytes.NewReader(respBody))
	if err != nil {
		return "", fmt.Errorf("error building request to media endpoint: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", msg.Channel().StringConfigForKey(models.ConfigAuthToken, "")))
	mediaType, _ := httpx.DetectContentType(respBody)
	req.Header.Add("Content-Type", mediaType)

	resp, respBody, err = h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		failedMediaCache.Set(failKey, true, cache.DefaultExpiration)
		if err != nil {
			return "", fmt.Errorf("error uploading media to whatsapp: %w", err)
		} else {
			return "", fmt.Errorf("non-200 response uploading media to whatsapp")
		}
	}

	// take uploaded media id
	mediaID, err = jsonparser.GetString(respBody, "media", "[0]", "id")
	if err != nil {
		return "", fmt.Errorf("error reading media id from response: %w", err)
	}

	// put in cache
	h.WithValkeyConn(func(rc redis.Conn) {
		err = mediaCache.Set(ctx, rc, mediaURL, mediaID)
	})

	if err != nil {
		return "", fmt.Errorf("error setting media id in cache: %w", err)
	}

	return mediaID, nil
}

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	accessToken := msg.Channel().StringConfigForKey(models.ConfigAuthToken, "")
	urlStr := msg.Channel().StringConfigForKey(models.ConfigBaseURL, "")
	url, err := url.Parse(urlStr)
	if accessToken == "" || err != nil {
		return courier.ErrChannelConfig
	}
	sendURL, _ := url.Parse("/v1/messages")

	requestPayloads, err := buildPayloads(ctx, msg, h, clog)

	//requestPayloads, err := whatsapp.GetMsgPayloads(ctx, msg, maxMsgLength, clog)
	if err != nil {
		return err
	}

	for _, payload := range requestPayloads {
		err := h.makeAPIRequest(payload, accessToken, res, sendURL, clog)
		if err != nil {
			return err
		}
	}

	return nil
}

// Error response payload from Turn.io WhatsApp API obreserved:
// {"meta":{"version":"4.923.9","backend":{"name":"WhatsApp","version":"latest"},"api_status":"stable"},"errors":[{"code":-1,"title":"Bad Request","details":"Could not be parsed, invalid key"}]}
// and docs mentions using the Meta Cloud API Error Codes
// https://whatsapp.turn.io/docs/api/errors
// https://developers.facebook.com/documentation/business-messaging/whatsapp/support/error-codes
// the struct below captures both errors array and error object
type mtResponsePayload struct {
	Errors []struct {
		Code    int    `json:"code"`
		Title   string `json:"title"`
		Details string `json:"details"`
	} `json:"errors"`
	Messages []*struct {
		ID string `json:"id"`
	} `json:"messages"`
	Error struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}

func (h *handler) makeAPIRequest(payload any, accessToken string, res *courier.SendResult, wacPhoneURL *url.URL, clog *courier.ChannelLog) error {
	jsonBody := jsonx.MustMarshal(payload)

	req, err := http.NewRequest(http.MethodPost, wacPhoneURL.String(), bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 == 5 {
		return courier.ErrConnectionFailed
	}

	respPayload := &mtResponsePayload{}
	err = json.Unmarshal(respBody, respPayload)
	if err != nil {
		return courier.ErrResponseUnparseable
	}

	if respPayload.Error.Code != 0 || respPayload.Error.Message != "" {

		if slices.Contains(whatsapp.WACThrottlingErrorCodes, respPayload.Error.Code) {
			return courier.ErrConnectionThrottled
		}

		if respPayload.Error.Code != 0 {
			return courier.ErrFailedWithReason(strconv.Itoa(respPayload.Error.Code), respPayload.Error.Message)
		} else if respPayload.Error.Message != "" {
			return courier.ErrFailedWithReason("0", respPayload.Error.Message)
		}
	}

	if len(respPayload.Errors) > 0 {
		if slices.Contains(whatsapp.WACThrottlingErrorCodes, respPayload.Errors[0].Code) {
			return courier.ErrConnectionThrottled
		}
		return courier.ErrFailedWithReason(strconv.Itoa(respPayload.Errors[0].Code), respPayload.Errors[0].Title)
	}

	if len(respPayload.Messages) > 0 {
		externalID := respPayload.Messages[0].ID
		if externalID != "" {
			res.AddExternalID(externalID)
		}
	}

	return nil
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
