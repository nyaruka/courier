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
		event := h.Backend().NewIncomingMsg(channel, urn, text).WithReceivedOn(date).WithExternalID(msg.ID)

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

		events = append(events, event)
		data = append(data, courier.NewMsgReceiveData(event))
	}

	// now with any status updates
	for _, status := range payload.Statuses {
		msgStatus, found := waStatusMapping[status.Status]
		if !found {
			handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("invalid status: %s", status.Status))
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

	if len(msg.Attachments()) > 0 {
		for attachmentCount, attachment := range msg.Attachments() {

			mimeType, s3url := handlers.SplitAttachment(attachment)
			mediaID, err := uploadMediaToWhatsApp(mediaURL, token, mimeType, s3url)
			if err != nil {
				duration := time.Now().Sub(start)
				log := courier.NewChannelLogFromError("Unable to upload media to WhatsApp server", msg.Channel(), msg.ID(), duration, err)
				status.AddLog(log)
				return status, err
			}

			externalID := ""
			if strings.HasPrefix(mimeType, "audio") {
				payload := mtAudioPayload{
					To:   msg.URN().Path(),
					Type: "audio",
				}
				payload.Audio = &mediaObject{ID: mediaID}
				externalID, err = sendWhatsAppMsg(sendURL, token, payload)

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
				externalID, err = sendWhatsAppMsg(sendURL, token, payload)

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
				externalID, err = sendWhatsAppMsg(sendURL, token, payload)

			} else {
				err = fmt.Errorf("unknown attachment mime type: %s", mimeType)
			}

			if err != nil {
				// record our status and log
				duration := time.Now().Sub(start)
				log := courier.NewChannelLogFromError("Error sending message", msg.Channel(), msg.ID(), duration, err)
				status.AddLog(log)
				return status, err
			}

			status.SetExternalID(externalID)

		}

	} else {
		parts := handlers.SplitMsg(msg.Text(), maxMsgLength)
		for i, part := range parts {
			payload := mtTextPayload{
				To:   msg.URN().Path(),
				Type: "text",
			}
			payload.Text.Body = part

			externalID, err := sendWhatsAppMsg(sendURL, token, payload)
			if err != nil {
				// record our status and log
				duration := time.Now().Sub(start)
				log := courier.NewChannelLogFromError("Error sending message", msg.Channel(), msg.ID(), duration, err)
				status.AddLog(log)
				return status, err
			}

			// if this is our first message, record the external id
			if i == 0 {
				status.SetExternalID(externalID)
			}
		}

	}

	status.SetStatus(courier.MsgWired)
	return status, nil
}

func uploadMediaToWhatsApp(url string, token string, attachmentMimeType string, attachmentURL string) (string, error) {

	// retrieve the media to be sent from S3
	req, _ := http.NewRequest(http.MethodGet, attachmentURL, nil)
	s3rr, err := utils.MakeHTTPRequest(req)
	if err != nil {
		return "", err
	}

	// upload it to WhatsApp in exchange for a media id
	waReq, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(s3rr.Body))
	waReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	waReq.Header.Set("Content-Type", attachmentMimeType)
	waReq.Header.Set("User-Agent", utils.HTTPUserAgent)
	wArr, err := utils.MakeHTTPRequest(waReq)
	if err != nil {
		return "", err
	}

	mediaID, err := jsonparser.GetString(wArr.Body, "media", "[0]", "id")
	if err != nil {
		return "", err
	}

	return mediaID, nil
}

func sendWhatsAppMsg(url string, token string, payload interface{}) (string, error) {

	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("User-Agent", utils.HTTPUserAgent)
	rr, err := utils.MakeHTTPRequest(req)

	errorTitle, err := jsonparser.GetString(rr.Body, "errors", "[0]", "title")
	if errorTitle != "" {
		err = errors.Errorf("received error from send endpoint: %s", errorTitle)
		return "", err
	}

	// grab the id
	externalID, err := jsonparser.GetString(rr.Body, "messages", "[0]", "id")
	if err != nil {
		err := errors.Errorf("unable to get message id from response body")
		return "", err
	}

	return externalID, err
}
