package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
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
		Audio struct {
			File     string `json:"file"`
			ID       string `json:"id"`
			Link     string `json:"link"`
			MimeType string `json:"mime_type"`
			Sha256   string `json:"sha256"`
		} `json:"audio"`
		Document struct {
			File     string `json:"file"`
			ID       string `json:"id"`
			Link     string `json:"link"`
			MimeType string `json:"mime_type"`
			Sha256   string `json:"sha256"`
			Caption  string `json:"caption"`
		} `json:"document"`
		Image struct {
			File     string `json:"file"`
			ID       string `json:"id"`
			Link     string `json:"link"`
			MimeType string `json:"mime_type"`
			Sha256   string `json:"sha256"`
			Caption  string `json:"caption"`
		} `json:"image"`
		Location struct {
			Address   string  `json:"address"`
			Latitude  float32 `json:"latitude"`
			Longitude float32 `json:"longitude"`
			Name      string  `json:"name"`
			URL       string  `json:"url"`
		} `json:"location"`
		Video struct {
			File     string `json:"file"`
			ID       string `json:"id"`
			Link     string `json:"link"`
			MimeType string `json:"mime_type"`
			Sha256   string `json:"sha256"`
		} `json:"video"`
		Voice struct {
			File     string `json:"file"`
			ID       string `json:"id"`
			Link     string `json:"link"`
			MimeType string `json:"mime_type"`
			Sha256   string `json:"sha256"`
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
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("Unsupported mesasge type %s", msg.Type))
		}

		// create our message
		event := h.Backend().NewIncomingMsg(channel, urn, text).WithReceivedOn(date).WithExternalID(msg.ID)

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
	token := channel.StringConfigForKey(courier.ConfigAuthToken, "")
	if token == "" {
		return "", fmt.Errorf("Missing token for WA channel")
	}

	urlStr := channel.StringConfigForKey(courier.ConfigBaseURL, "")
	url, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("invalid base url set for WA channel: %s", err)
	}
	sendPath, _ := url.Parse("/v1/messages")
	sendURL := url.ResolveReference(sendPath).String()

	fileURL := fmt.Sprintf("%s/v1/media/%s", sendURL, mediaID)

	return fileURL, nil
}

// BuildDownloadMediaRequest to download media for message attachment with Bearer token set
func (h *handler) BuildDownloadMediaRequest(ctx context.Context, b courier.Backend, channel courier.Channel, attachmentURL string) (*http.Request, error) {
	token := channel.StringConfigForKey(courier.ConfigAuthToken, "")

	// first fetch our media
	req, _ := http.NewRequest(http.MethodGet, attachmentURL, nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
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
//   "type": "text",
//   "text": {
//     "body": "text message"
//   }
// }
type mtPayload struct {
	To   string `json:"to"    validate:"required"`
	Type string `json:"type"`
	Text struct {
		Body string `json:"body"  validate:"required"`
	} `json:"text"`
}

// whatsapp only allows messages up to 4096 chars
const maxMsgLength = 4096

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
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

	// TODO: figure out sending media

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	parts := handlers.SplitMsg(msg.Text(), maxMsgLength)
	for i, part := range parts {
		payload := mtPayload{
			To:   msg.URN().Path(),
			Type: "text",
		}
		payload.Text.Body = part

		jsonBody, err := json.Marshal(payload)
		if err != nil {
			return status, err
		}

		req, _ := http.NewRequest(http.MethodPost, sendURL, bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
		rr, err := utils.MakeHTTPRequest(req)

		// record our status and log
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
		status.AddLog(log)
		if err != nil {
			return status, nil
		}

		// was this an error?
		errorTitle, _ := jsonparser.GetString([]byte(rr.Body), "errors", "[0]", "title")
		if errorTitle != "" {
			log.WithError("Message Send Error", errors.Errorf("received error from send endpoint"))
			return status, nil
		}

		// grab the id
		externalID, err := jsonparser.GetString([]byte(rr.Body), "messages", "[0]", "id")
		if err != nil {
			log.WithError("Message Send Error", errors.Errorf("unable to get messages.0.id from body"))
			return status, nil
		}

		// if this is our first message, record the external id
		if i == 0 {
			status.SetExternalID(externalID)
		}
	}

	status.SetStatus(courier.MsgWired)
	return status, nil
}
