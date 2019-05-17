package whatsapp

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
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
	"github.com/pkg/errors"
)

const (
	configNamespace = "fb_namespace"
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("WA"), "WhatsApp")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveEvent)
	return nil
}

// {
//   "statuses": [{
//     "id": "9712A34B4A8B6AD50F",
//     "recipient_id": "16315555555",
//     "status": "sent",
//     "timestamp": "1518694700"
//   }],
//   "messages": [ {
//     "from": "16315555555",
//     "id": "3AF99CB6BE490DCAF641",
//     "timestamp": "1518694235",
//     "text": {
//       "body": "Hello this is an answer"
//     },
//     "type": "text"
//   }]
// }
type eventPayload struct {
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
		Document *struct {
			File     string `json:"file"      validate:"required"`
			ID       string `json:"id"        validate:"required"`
			Link     string `json:"link"`
			MimeType string `json:"mime_type" validate:"required"`
			Sha256   string `json:"sha256"    validate:"required"`
			Caption  string `json:"caption"`
		} `json:"document"`
		Image *struct {
			File     string `json:"file"      validate:"required"`
			ID       string `json:"id"        validate:"required"`
			Link     string `json:"link"`
			MimeType string `json:"mime_type" validate:"required"`
			Sha256   string `json:"sha256"    validate:"required"`
			Caption  string `json:"caption"`
		} `json:"image"`
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
		ID          string `json:"id"           validate:"required"`
		RecipientID string `json:"recipient_id" validate:"required"`
		Timestamp   string `json:"timestamp"    validate:"required"`
		Status      string `json:"status"       validate:"required"`
	} `json:"statuses"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveEvent(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	payload := &eventPayload{}
	err := handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// the list of events we deal with
	events := make([]courier.Event, 0, 2)

	// the list of data we will return in our response
	data := make([]interface{}, 0, 2)

	// first deal with any received messages
	for _, msg := range payload.Messages {
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
		} else if msg.Type == "audio" {
			mediaURL, err = resolveMediaURL(channel, msg.Audio.ID)
		} else if msg.Type == "document" {
			text = msg.Document.Caption
			mediaURL, err = resolveMediaURL(channel, msg.Document.ID)
		} else if msg.Type == "image" {
			text = msg.Image.Caption
			mediaURL, err = resolveMediaURL(channel, msg.Image.ID)
		} else if msg.Type == "location" {
			mediaURL = fmt.Sprintf("geo:%f,%f", msg.Location.Latitude, msg.Location.Longitude)
		} else if msg.Type == "video" {
			mediaURL, err = resolveMediaURL(channel, msg.Video.ID)
		} else if msg.Type == "voice" {
			mediaURL, err = resolveMediaURL(channel, msg.Voice.ID)
		} else {
			// we received a message type we do not support.
			courier.LogRequestError(r, channel, fmt.Errorf("unsupported message type %s", msg.Type))
		}

		// create our message
		ev := h.Backend().NewIncomingMsg(channel, urn, text).WithReceivedOn(date).WithExternalID(msg.ID)
		event := h.Backend().CheckExternalIDSeen(ev)

		// we had an error downloading media
		if err != nil {
			courier.LogRequestError(r, channel, err)
		}

		if mediaURL != "" {
			event.WithAttachment(mediaURL)
		}

		err = h.Backend().WriteMsg(ctx, event)
		if err != nil {
			return nil, err
		}

		h.Backend().WriteExternalIDSeen(event)

		events = append(events, event)
		data = append(data, courier.NewMsgReceiveData(event))
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

		event := h.Backend().NewMsgStatusForExternalID(channel, status.ID, msgStatus)
		err := h.Backend().WriteMsgStatus(ctx, event)

		// we don't know about this message, just tell them we ignored it
		if err == courier.ErrMsgNotFound {
			data = append(data, courier.NewInfoData(fmt.Sprintf("message id: %s not found, ignored", status.ID)))
			continue
		}

		if err != nil {
			return nil, err
		}

		events = append(events, event)
		data = append(data, courier.NewStatusData(event))
	}

	return events, courier.WriteDataResponse(ctx, w, http.StatusOK, "Events Handled", data)
}

func resolveMediaURL(channel courier.Channel, mediaID string) (string, error) {
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

// BuildDownloadMediaRequest to download media for message attachment with Bearer token set
func (h *handler) BuildDownloadMediaRequest(ctx context.Context, b courier.Backend, channel courier.Channel, attachmentURL string) (*http.Request, error) {
	token := channel.StringConfigForKey(courier.ConfigAuthToken, "")
	if token == "" {
		return nil, fmt.Errorf("missing token for WA channel")
	}

	// set the access token as the authorization header
	req, _ := http.NewRequest(http.MethodGet, attachmentURL, nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("User-Agent", utils.HTTPUserAgent)
	return req, nil
}

var waStatusMapping = map[string]courier.MsgStatusValue{
	"sending":   courier.MsgWired,
	"sent":      courier.MsgSent,
	"delivered": courier.MsgDelivered,
	"read":      courier.MsgDelivered,
	"failed":    courier.MsgFailed,
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
	To   string `json:"to"    validate:"required"`
	Type string `json:"type"  validate:"required"`
	Text struct {
		Body string `json:"body" validate:"required"`
	} `json:"text"`
}

type mediaObject struct {
	ID string `json:"id" validate:"required"`
}

type LocalizableParam struct {
	Default string `json:"default"`
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

type captionedMediaObject struct {
	ID      string `json:"id" validate:"required"`
	Caption string `json:"caption,omitempty"`
}

type mtAudioPayload struct {
	To    string       `json:"to"    validate:"required"`
	Type  string       `json:"type"  validate:"required"`
	Audio *mediaObject `json:"audio"`
}

type mtDocumentPayload struct {
	To       string                `json:"to"    validate:"required"`
	Type     string                `json:"type"  validate:"required"`
	Document *captionedMediaObject `json:"document"`
}

type mtImagePayload struct {
	To    string                `json:"to"    validate:"required"`
	Type  string                `json:"type"  validate:"required"`
	Image *captionedMediaObject `json:"image"`
}

type mtVideoPayload struct {
	To    string                `json:"to" validate: "required"`
	Type  string                `json:"type" validate: "required"`
	Video *captionedMediaObject `json:"video"`
}

// whatsapp only allows messages up to 4096 chars
const maxMsgLength = 4096

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	start := time.Now()
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
	sendURL := url.ResolveReference(sendPath).String()

	mediaPath, _ := url.Parse("/v1/media")
	mediaURL := url.ResolveReference(mediaPath).String()

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	var log *courier.ChannelLog

	if len(msg.Attachments()) > 0 {
		for attachmentCount, attachment := range msg.Attachments() {

			mimeType, s3url := handlers.SplitAttachment(attachment)
			mediaID := ""
			mediaID, log, err = uploadMediaToWhatsApp(msg, mediaURL, token, mimeType, s3url)
			status.AddLog(log)

			if err != nil {
				log.WithError("Unable to upload media to WhatsApp server", err)
				break
			}

			externalID := ""
			if strings.HasPrefix(mimeType, "audio") {
				payload := mtAudioPayload{
					To:   msg.URN().Path(),
					Type: "audio",
				}
				payload.Audio = &mediaObject{ID: mediaID}
				externalID, log, err = sendWhatsAppMsg(msg, sendURL, token, payload)

			} else if strings.HasPrefix(mimeType, "application") {
				payload := mtDocumentPayload{
					To:   msg.URN().Path(),
					Type: "document",
				}

				if attachmentCount == 0 {
					payload.Document = &captionedMediaObject{ID: mediaID, Caption: msg.Text()}
				} else {
					payload.Document = &captionedMediaObject{ID: mediaID}
				}
				externalID, log, err = sendWhatsAppMsg(msg, sendURL, token, payload)

			} else if strings.HasPrefix(mimeType, "image") {
				payload := mtImagePayload{
					To:   msg.URN().Path(),
					Type: "image",
				}
				if attachmentCount == 0 {
					payload.Image = &captionedMediaObject{ID: mediaID, Caption: msg.Text()}
				} else {
					payload.Image = &captionedMediaObject{ID: mediaID}
				}
				externalID, log, err = sendWhatsAppMsg(msg, sendURL, token, payload)
			} else if strings.HasPrefix(mimeType, "video") {
				payload := mtVideoPayload{
					To:   msg.URN().Path(),
					Type: "video",
				}
				if attachmentCount == 0 {
					payload.Video = &captionedMediaObject{ID: mediaID, Caption: msg.Text()}
				} else {
					payload.Video = &captionedMediaObject{ID: mediaID}
				}
				externalID, log, err = sendWhatsAppMsg(msg, sendURL, token, payload)
			} else {
				duration := time.Since(start)
				err = fmt.Errorf("unknown attachment mime type: %s", mimeType)
				log = courier.NewChannelLogFromError("Error sending message", msg.Channel(), msg.ID(), duration, err)
			}

			// if we have a log, add it to our status
			if log != nil {
				status.AddLog(log)
			}

			// break out on errors
			if err != nil {
				break
			}

			// set our external id if we have one
			if attachmentCount == 0 {
				status.SetExternalID(externalID)
			}
		}

	} else {
		// do we have a template?
		var templating *MsgTemplating
		templating, err = h.getTemplate(msg)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to decode template: %s for channel: %s", string(msg.Metadata()), msg.Channel().UUID())
		}

		if templating != nil {
			namespace := msg.Channel().StringConfigForKey(configNamespace, "")
			if namespace == "" {
				return nil, errors.Errorf("cannot send template message without Facebook namespace for channel: %s", msg.Channel().UUID())
			}

			payload := &hsmPayload{
				To:   msg.URN().Path(),
				Type: "hsm",
			}
			payload.HSM.Namespace = namespace
			payload.HSM.ElementName = templating.Template.Name
			payload.HSM.Language.Policy = "deterministic"
			payload.HSM.Language.Code = templating.Language
			for _, v := range templating.Variables {
				payload.HSM.LocalizableParams = append(payload.HSM.LocalizableParams, LocalizableParam{Default: v})
			}

			externalID, log, err := sendWhatsAppMsg(msg, sendURL, token, payload)
			status.AddLog(log)

			if err != nil {
				log.WithError("Error sending message", err)
				return status, nil
			}

			status.SetExternalID(externalID)
		} else {
			parts := handlers.SplitMsg(msg.Text(), maxMsgLength)
			externalID := ""
			for i, part := range parts {
				payload := mtTextPayload{
					To:   msg.URN().Path(),
					Type: "text",
				}
				payload.Text.Body = part

				externalID, log, err = sendWhatsAppMsg(msg, sendURL, token, payload)
				status.AddLog(log)

				if err != nil {
					log.WithError("Error sending message", err)
					break
				}

				// if this is our first message, record the external id
				if i == 0 {
					status.SetExternalID(externalID)
				}
			}
		}
	}

	// we are wired it there were no errors
	if err == nil {
		status.SetStatus(courier.MsgWired)
	}

	return status, nil
}

func uploadMediaToWhatsApp(msg courier.Msg, url string, token string, attachmentMimeType string, attachmentURL string) (string, *courier.ChannelLog, error) {
	// retrieve the media to be sent from S3
	req, _ := http.NewRequest(http.MethodGet, attachmentURL, nil)
	s3rr, err := utils.MakeHTTPRequest(req)
	if err != nil {
		return "", courier.NewChannelLogFromRR("Media Fetch", msg.Channel(), msg.ID(), s3rr), err
	}

	// upload it to WhatsApp in exchange for a media id
	waReq, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(s3rr.Body))
	waReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	waReq.Header.Set("Content-Type", attachmentMimeType)
	waReq.Header.Set("User-Agent", utils.HTTPUserAgent)
	wArr, err := utils.MakeHTTPRequest(waReq)

	log := courier.NewChannelLogFromRR("Media Upload success", msg.Channel(), msg.ID(), wArr)

	if err != nil {
		return "", log, err
	}

	mediaID, err := jsonparser.GetString(wArr.Body, "media", "[0]", "id")
	if err != nil {
		return "", log, err
	}

	return mediaID, log, nil
}

func sendWhatsAppMsg(msg courier.Msg, url string, token string, payload interface{}) (string, *courier.ChannelLog, error) {
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		log := courier.NewChannelLog("unable to build JSON body", msg.Channel(), msg.ID(), "", "", courier.NilStatusCode, "", "", time.Duration(0), err)
		return "", log, err
	}

	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("User-Agent", utils.HTTPUserAgent)
	rr, err := utils.MakeHTTPRequest(req)

	log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)

	errorTitle, err := jsonparser.GetString(rr.Body, "errors", "[0]", "title")
	if errorTitle != "" {
		err = errors.Errorf("received error from send endpoint: %s", errorTitle)
		return "", log, err
	}

	// grab the id
	externalID, err := jsonparser.GetString(rr.Body, "messages", "[0]", "id")
	if err != nil {
		err := errors.Errorf("unable to get message id from response body")
		return "", log, err
	}

	return externalID, log, err
}

func (h *handler) getTemplate(msg courier.Msg) (*MsgTemplating, error) {
	mdJSON := msg.Metadata()
	if len(mdJSON) == 0 {
		return nil, nil
	}
	metadata := &TemplateMetadata{}
	err := json.Unmarshal(mdJSON, metadata)
	if err != nil {
		return nil, err
	}
	templating := metadata.Templating
	if templating == nil {
		return nil, nil
	}

	// check our template is valid
	err = handlers.Validate(templating)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid templating definition")
	}

	// map our language from iso639-3 to the WA country / iso638-2 pair
	language, found := languageMap[templating.Language]
	if !found {
		return nil, fmt.Errorf("unable to find mapping for language: %s", templating.Language)
	}
	templating.Language = language

	return templating, err
}

type TemplateMetadata struct {
	Templating *MsgTemplating `json:"templating"`
}

type MsgTemplating struct {
	Template struct {
		Name string `json:"name" validate:"required"`
		UUID string `json:"uuid" validate:"required"`
	} `json:"template" validate:"required,dive"`
	Language  string   `json:"language" validate:"required"`
	Variables []string `json:"variables"`
}

// mapping from iso639-3 to WA language code
var languageMap = map[string]string{
	"afr": "af",    // Afrikaans
	"sqi": "sq",    // Albanian
	"ara": "ar",    // Arabic
	"aze": "az",    // Azerbaijani
	"ben": "bn",    // Bengali
	"bul": "bg",    // Bulgarian
	"cat": "ca",    // Catalan
	"zho": "zh_CN", // Chinese (CHN)
	// zh_HK Chinese (HKG) (unsupported, use zh_CN)
	// zh_TW Chinese (TAI) (unsupported, use zh_CN)
	"hrv": "hr", //Croatian
	"ces": "cs", // Czech
	"dah": "da", // Danish
	"nld": "nl", // Dutch
	"eng": "en", // English
	// en_GB English (UK) (unsupported, use en)
	// en_US English (US) (unsupported, use en)
	"est": "et",  // Estonian
	"fil": "fil", // Filipino
	"fin": "fi",  // Finnish
	"fra": "fr",  // French
	"deu": "de",  // German
	"ell": "el",  // Greek
	"gul": "gu",  // Gujarati
	"enb": "he",  // Hebrew
	"hin": "hi",  // Hindi
	"hun": "hu",  // Hungarian
	"ind": "id",  // Indonesian
	"gle": "ga",  // Irish
	"ita": "it",  // Italian
	"jpn": "ja",  // Japanese
	"kan": "kn",  // Kannada
	"kaz": "kk",  // Kazakh
	"kor": "ko",  // Korean
	"lao": "lo",  // Lao
	"jav": "lv",  // Latvian
	"lit": "lt",  // Lithuanian
	"mkd": "mk",  // Macedonian
	"msa": "ms",  // Malay
	"mar": "mr",  // Marathi
	"nob": "nb",  // Norwegian
	"fas": "fa",  // Persian
	"pol": "pl",  // Polish
	// "pt_BR" Portuguese (BR)  (unsupported, use pt_PT)
	"por": "pt_PT", // Portuguese (POR)
	"pan": "pa",    // Punjabi
	"ron": "ro",    // Romanian
	"rus": "ru",    // Russian
	"srp": "sr",    // Serbian
	"slk": "sk",    // Slovak
	"slv": "sl",    // Slovenian
	"spa": "es",    // Spanish
	// es_AR Spanish (ARG) (unsupported, use es)
	// es_ES Spanish (SPA) (unsupported, use es)
	// es_MX Spanish (MEX) (unsupported, use es)
	"swa": "sw", // Swahili
	"swe": "sv", // Swedish
	"tam": "ta", // Tamil
	"tel": "te", // Telugu
	"tha": "th", // Thai
	"tur": "tr", // Turkish
	"ukr": "uk", // Ukrainian
	"urd": "ur", // Urdu
	"uzb": "uz", // Uzbek
	"vie": "vi", // Vietnamese
}
