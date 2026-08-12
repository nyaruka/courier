package turn

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/courier/v26/handlers/meta/whatsapp"
	"github.com/nyaruka/gocommon/cache"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/vkutil"
)

var (
	// max for the body
	maxMsgLength    = 4096
	configNamespace = "fb_namespace"

	mediaCacheKeyPattern = "turn_whatsapp_media_%s"
	failedMediaCache     *cache.Local[string, bool]
)

func init() {
	channels.RegisterHandler(newHandler())

	failedMediaCache = cache.NewLocal[string, bool](nil, 15*time.Minute)
	failedMediaCache.Start()
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() channels.Handler {
	return &handler{handlers.NewBaseHandler(models.ChannelType("TRN"), "Turn.io WhatsApp")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(r *channels.Routes) error {
	r.Add(h, http.MethodPost, "receive", models.ChannelLogTypeMultiReceive, handlers.JSONPayload(h, h.receiveEvents))

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
		WaID   string `json:"wa_id"`
		UserID string `json:"user_id"`
	} `json:"contacts"`
	Messages []struct {
		From      string `json:"from"`
		FromBSUID string `json:"from_bsuid"`
		ID        string `json:"id"        validate:"required"`
		GroupID   string `json:"group_id,omitempty"`
		Timestamp string `json:"timestamp" validate:"required"`
		Type      string `json:"type"      validate:"required"`
		Text      struct {
			Body string `json:"body"`
		} `json:"text"`
		Audio *struct {
			ID string `json:"id"`
		} `json:"audio"`
		Button *struct {
			Text string `json:"text"`
		} `json:"button"`
		Document *struct {
			ID      string `json:"id"`
			Caption string `json:"caption"`
		} `json:"document"`
		Image *struct {
			ID      string `json:"id"`
			Caption string `json:"caption"`
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
			NFMReply *struct {
				Name         string `json:"name"`
				Body         string `json:"body"`
				ResponseJSON string `json:"response_json"`
			} `json:"nfm_reply"`
			Type string `json:"type"`
		} `json:"interactive"`
		Location *struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		} `json:"location"`
		Video *struct {
			ID      string `json:"id"`
			Caption string `json:"caption"`
		} `json:"video"`
		Voice *struct {
			ID string `json:"id"`
		} `json:"voice"`
		Errors []whatsapp.WAError `json:"errors"`
	} `json:"messages"`
	Statuses []struct {
		ID        string             `json:"id"           validate:"required"`
		Timestamp string             `json:"timestamp"    validate:"required"`
		Status    string             `json:"status"       validate:"required"`
		Errors    []whatsapp.WAError `json:"errors"`
	} `json:"statuses"`
}

// receiveEvents is our HTTP handler function for incoming messages and status updates
func (h *handler) receiveEvents(ctx context.Context, channel *models.Channel, w http.ResponseWriter, r *http.Request, payload *eventsPayload, clog *models.ChannelLog) ([]channels.Event, error) {
	events := make([]channels.Event, 0, 2)

	// the list of data we will return in our response
	data := make([]any, 0, 2)

	seenMsgIDs := make(map[string]bool, 2)

	// contacts are keyed by both identifiers they can carry, as a message from a user with a username may
	// only reference them by their user_id
	var contactNames = make(map[string]string)
	for _, contact := range payload.Contacts {
		if contact.WaID != "" {
			contactNames[contact.WaID] = contact.Profile.Name
		}
		if contact.UserID != "" {
			contactNames[contact.UserID] = contact.Profile.Name
		}
	}

	// first deal with any received messages
	for _, msg := range payload.Messages {
		if seenMsgIDs[msg.ID] {
			continue
		}

		if msg.GroupID != "" {
			data = append(data, channels.NewInfoData("ignoring group message"))
			continue
		}

		// create our date from the timestamp
		ts, err := strconv.ParseInt(msg.Timestamp, 10, 64)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("invalid timestamp: %s", msg.Timestamp))
		}
		date := time.Unix(ts, 0).UTC()

		// create our URN from the sender's phone number, falling back to their business-scoped user ID as a user
		// who has adopted a WhatsApp username can have their phone number omitted from the webhook entirely
		sender := cmp.Or(msg.From, msg.FromBSUID)

		urn, err := urns.New(urns.WhatsApp, sender)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, errors.New("invalid whatsapp id"))
		}

		for _, msgError := range msg.Errors {
			msgError.ErrorChannelLog(clog)
		}

		text := ""
		mediaURL := ""
		var msgPayload json.RawMessage
		supported := true

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
		} else if msg.Type == "interactive" && msg.Interactive != nil && msg.Interactive.Type == "button_reply" && msg.Interactive.ButtonReply != nil {
			text = msg.Interactive.ButtonReply.Title
		} else if msg.Type == "interactive" && msg.Interactive != nil && msg.Interactive.Type == "list_reply" && msg.Interactive.ListReply != nil {
			text = msg.Interactive.ListReply.Title
		} else if msg.Type == "interactive" && msg.Interactive != nil && msg.Interactive.Type == "nfm_reply" && msg.Interactive.NFMReply != nil {
			text = msg.Interactive.NFMReply.Body
			// attach the response JSON as the msg payload if it's a valid JSON object
			raw := strings.TrimSpace(msg.Interactive.NFMReply.ResponseJSON)
			if strings.HasPrefix(raw, "{") && json.Valid([]byte(raw)) {
				msgPayload = json.RawMessage(raw)
			} else if msg.Interactive.NFMReply.ResponseJSON != "" {
				channels.LogRequestError(r, channel, errors.New("nfm_reply response_json is not a valid JSON object"))
			}
		} else if msg.Type == "location" && msg.Location != nil {
			mediaURL = fmt.Sprintf("geo:%f,%f", msg.Location.Latitude, msg.Location.Longitude)
		} else if msg.Type == "video" && msg.Video != nil {
			text = msg.Video.Caption
			mediaURL, err = resolveMediaURL(channel, msg.Video.ID)
		} else if msg.Type == "voice" && msg.Voice != nil {
			mediaURL, err = resolveMediaURL(channel, msg.Voice.ID)
		} else {
			supported = false
		}

		// we received a message type we do not support - don't create an empty message for it
		if !supported {
			channels.LogRequestError(r, channel, fmt.Errorf("unsupported message type %s", msg.Type))
			continue
		}

		// create our message
		event := models.NewIncomingMsg(channel, urn, text, msg.ID, clog).WithReceivedOn(date).WithContactName(contactNames[sender])

		// we had an error downloading media
		if err != nil {
			channels.LogRequestError(r, channel, err)
		}

		if mediaURL != "" {
			event.WithAttachment(mediaURL)
		}

		if msgPayload != nil {
			event.WithPayload(msgPayload)
		}

		// if we have a from_bsuid, add it as a secondary whatsapp URN (unless it's already the primary URN)
		if msg.FromBSUID != "" {
			userIDURN, urnErr := urns.New(urns.WhatsApp, msg.FromBSUID)
			if urnErr == nil {
				if userIDURN != urn {
					event.WithNewURN(userIDURN, models.NewURNAppend)
				}
			} else {
				channels.LogRequestError(r, channel, fmt.Errorf("invalid from_bsuid for whatsapp URN: %w", urnErr))
			}
		}

		err = models.WriteMsg(ctx, h.Runtime(), event, clog)
		if err != nil {
			return nil, err
		}

		events = append(events, event)
		data = append(data, channels.NewMsgReceiveData(event))
		seenMsgIDs[msg.ID] = true
	}

	// now with any status updates
	for _, status := range payload.Statuses {
		msgStatus, found := turnWaStatusMapping[status.Status]
		if !found {
			if turnWaIgnoreStatuses[status.Status] {
				data = append(data, channels.NewInfoData(fmt.Sprintf("ignoring status: %s", status.Status)))
			} else {
				handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, fmt.Sprintf("unknown status: %s", status.Status))
			}
			continue
		}

		for _, statusError := range status.Errors {
			statusError.ErrorChannelLog(clog)
		}

		event := models.NewStatusUpdateByExternalID(channel, status.ID, msgStatus, clog)
		err := models.WriteStatusUpdate(ctx, h.Runtime(), event)
		if err != nil {
			return nil, err
		}

		events = append(events, event)
		data = append(data, channels.NewStatusData(event))
	}

	return events, channels.WriteDataResponse(w, http.StatusOK, "Events Handled", data)
}

func resolveMediaURL(channel *models.Channel, mediaID string) (string, error) {
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
func (h *handler) BuildAttachmentRequest(ctx context.Context, channel *models.Channel, attachmentURL string, clog *models.ChannelLog) (*http.Request, error) {
	token := channel.StringConfigForKey(models.ConfigAuthToken, "")
	if token == "" {
		return nil, fmt.Errorf("missing token for TRN channel")
	}

	// set the access token as the authorization header
	req, _ := http.NewRequest(http.MethodGet, attachmentURL, nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	return req, nil
}

var _ channels.AttachmentRequestBuilder = (*handler)(nil)

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

// recipient identifies the message recipient - either a phone number (to) or a business-scoped user ID (recipient)
type recipient struct {
	To        string `json:"to,omitempty"`
	Recipient string `json:"recipient,omitempty"`
}

// newRecipient builds the recipient fields from the message URN.
func newRecipient(msg *models.MsgOut) recipient {
	to, rcpt := whatsapp.RecipientFields(msg.URN())
	return recipient{To: to, Recipient: rcpt}
}

type mtTextPayload struct {
	recipient
	Type       string `json:"type"  validate:"required"`
	PreviewURL bool   `json:"preview_url,omitempty"`
	Text       struct {
		Body string `json:"body" validate:"required"`
	} `json:"text"`
}

// interactive messages have the same shape as their Cloud API counterparts, and their headers CAN reference
// media by link
type mtInteractivePayload struct {
	recipient
	Type        string                `json:"type" validate:"required"`
	Interactive *whatsapp.Interactive `json:"interactive"`
}

// media messages reference media by the id returned from /v1/media - they can't carry a link
type mediaObject struct {
	ID       string `json:"id,omitempty"`
	Caption  string `json:"caption,omitempty"`
	Filename string `json:"filename,omitempty"`
}

type templatePayload struct {
	recipient
	Type     string `json:"type"`
	Template struct {
		Namespace  string                `json:"namespace"`
		Name       string                `json:"name"`
		Language   whatsapp.Language     `json:"language"`
		Components []*whatsapp.Component `json:"components,omitempty"`
	} `json:"template"`
}

type mtAudioPayload struct {
	recipient
	Type  string       `json:"type"  validate:"required"`
	Audio *mediaObject `json:"audio"`
}

type mtDocumentPayload struct {
	recipient
	Type     string       `json:"type"  validate:"required"`
	Document *mediaObject `json:"document"`
}

type mtImagePayload struct {
	recipient
	Type  string       `json:"type"  validate:"required"`
	Image *mediaObject `json:"image"`
}

type mtVideoPayload struct {
	recipient
	Type  string       `json:"type" validate:"required"`
	Video *mediaObject `json:"video"`
}

func (h *handler) buildPayloads(ctx context.Context, msg *models.MsgOut, clog *models.ChannelLog) ([]any, error) {
	rcpt := newRecipient(msg)

	// Template messages must be sent as a single template payload. Mailroom/goflow may also
	// attach preview media and button quick-replies on the same message; those must not be
	// turned into separate image/interactive sends.
	if msg.Templating() != nil {
		// prefer the language mailroom resolved for the template, falling back to deriving one from the msg locale
		langCode := cmp.Or(msg.Templating().Language, getSupportedLanguage(msg.Locale()))
		namespace := msg.Templating().Namespace
		if namespace == "" {
			namespace = msg.Channel().StringConfigForKey(configNamespace, "")
		}
		if namespace == "" {
			return nil, fmt.Errorf("cannot send template message without Facebook namespace for channel: %s", msg.Channel().UUID())
		}

		waTpl := whatsapp.GetTemplatePayload(msg.Templating())

		payload := templatePayload{
			recipient: rcpt,
			Type:      "template",
		}
		payload.Template.Namespace = namespace
		payload.Template.Name = waTpl.Name
		payload.Template.Language = whatsapp.Language{Policy: "deterministic", Code: langCode}
		payload.Template.Components = waTpl.Components

		return []any{payload}, nil
	}

	requests, err := whatsapp.GetMsgPayloads(ctx, msg, maxMsgLength, clog)
	if err != nil {
		return nil, err
	}

	payloads := make([]any, 0, len(requests))
	for _, request := range requests {
		payload, err := h.convertPayload(ctx, msg, rcpt, request, clog)
		if err != nil {
			return nil, err
		}
		payloads = append(payloads, payload)
	}
	return payloads, nil
}

// convertPayload converts a Cloud API style send request from the shared payload builder into this API's format:
// media messages must reference media uploaded to /v1/media by id - unlike template headers and interactive
// headers, they can't carry a link - and URL previews are a top level field.
func (h *handler) convertPayload(ctx context.Context, msg *models.MsgOut, rcpt recipient, request whatsapp.SendRequest, clog *models.ChannelLog) (any, error) {
	switch request.Type {
	case "text":
		payload := mtTextPayload{recipient: rcpt, Type: "text", PreviewURL: request.Text.PreviewURL}
		payload.Text.Body = request.Text.Body
		return payload, nil

	case "image", "audio", "video", "document":
		var media *whatsapp.Media
		switch request.Type {
		case "image":
			media = request.Image
		case "audio":
			media = request.Audio
		case "video":
			media = request.Video
		case "document":
			media = request.Document
		}

		mediaID, err := h.fetchMediaID(ctx, msg, media.Link, clog)
		if err != nil {
			slog.Error("error while uploading media to whatsapp", "error", err, "channel_uuid", msg.Channel().UUID())
		}
		if mediaID == "" {
			return nil, channels.ErrRetryableWithReason("media_upload_failed", "unable to upload media to WhatsApp")
		}

		object := &mediaObject{ID: mediaID, Caption: media.Caption, Filename: media.Filename}
		switch request.Type {
		case "image":
			return mtImagePayload{recipient: rcpt, Type: "image", Image: object}, nil
		case "audio":
			return mtAudioPayload{recipient: rcpt, Type: "audio", Audio: object}, nil
		case "video":
			return mtVideoPayload{recipient: rcpt, Type: "video", Video: object}, nil
		default:
			return mtDocumentPayload{recipient: rcpt, Type: "document", Document: object}, nil
		}

	case "interactive":
		return mtInteractivePayload{recipient: rcpt, Type: "interactive", Interactive: request.Interactive}, nil
	}

	return nil, fmt.Errorf("unsupported payload type: %s", request.Type)
}

// fetchMediaID tries to fetch the id for the uploaded media, setting the result in redis.
func (h *handler) fetchMediaID(ctx context.Context, msg *models.MsgOut, mediaURL string, clog *models.ChannelLog) (string, error) {
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

	// if we cached a failure, don't try again until our cache expires
	if failedMediaCache.Get(failKey) {
		return "", nil
	}

	// download media
	req, err := http.NewRequest("GET", mediaURL, nil)
	if err != nil {
		return "", fmt.Errorf("error building media request: %w", err)
	}

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		failedMediaCache.Set(failKey, true)
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
		failedMediaCache.Set(failKey, true)
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

func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	accessToken := msg.Channel().StringConfigForKey(models.ConfigAuthToken, "")
	urlStr := msg.Channel().StringConfigForKey(models.ConfigBaseURL, "")
	url, err := url.Parse(urlStr)
	if accessToken == "" || err != nil {
		return channels.ErrChannelConfig
	}
	sendURL, _ := url.Parse("/v1/messages")

	requestPayloads, err := h.buildPayloads(ctx, msg, clog)
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

func (h *handler) makeAPIRequest(payload any, accessToken string, res *channels.SendResult, wacPhoneURL *url.URL, clog *models.ChannelLog) error {
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
		return channels.ErrConnectionFailed
	}
	// Turn returns HTTP 429 with its own rate-limit payload (errors[].code=429) and
	// Retry-After / X-Ratelimit-* headers. Treat that as throttled so the message is
	// retried via the errored queue rather than permanently failed.
	if handlers.IsThrottled(resp) {
		return channels.ErrConnectionThrottled
	}

	respPayload := &mtResponsePayload{}
	err = json.Unmarshal(respBody, respPayload)
	if err != nil {
		return channels.ErrResponseUnparseable
	}

	if respPayload.Error.Code == 429 || slices.Contains(whatsapp.WACThrottlingErrorCodes, respPayload.Error.Code) {
		return channels.ErrConnectionThrottled
	}

	if slices.Contains(whatsapp.WACRetryableErrorCodes, respPayload.Error.Code) {
		return channels.ErrRetryableWithReason(strconv.Itoa(respPayload.Error.Code), respPayload.Error.Message)
	}

	if respPayload.Error.Code != 0 || respPayload.Error.Message != "" {
		return channels.ErrFailedWithReason(strconv.Itoa(respPayload.Error.Code), respPayload.Error.Message)
	}

	if len(respPayload.Errors) > 0 {
		if respPayload.Errors[0].Code == 429 || slices.Contains(whatsapp.WACThrottlingErrorCodes, respPayload.Errors[0].Code) {
			return channels.ErrConnectionThrottled
		}
		if slices.Contains(whatsapp.WACRetryableErrorCodes, respPayload.Errors[0].Code) {
			return channels.ErrRetryableWithReason(strconv.Itoa(respPayload.Errors[0].Code), respPayload.Errors[0].Title)
		}
		return channels.ErrFailedWithReason(strconv.Itoa(respPayload.Errors[0].Code), respPayload.Errors[0].Title)
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
