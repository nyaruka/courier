package line

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/urns"
	"github.com/pkg/errors"
)

var (
	replySendURL = "https://api.line.me/v2/bot/message/reply"
	pushSendURL  = "https://api.line.me/v2/bot/message/push"
	mediaDataURL = "https://api-data.line.me/v2/bot/message"
	maxMsgLength = 2000
	maxMsgSend   = 5

	signatureHeader = "X-Line-Signature"
)

// see https://developers.line.biz/en/reference/messaging-api/#message-objects
var mediaSupport = map[handlers.MediaType]handlers.MediaTypeSupport{
	handlers.MediaTypeImage:       {Types: []string{"image/jpeg", "image/png"}, MaxBytes: 10 * 1024 * 1024},
	handlers.MediaTypeAudio:       {Types: []string{"audio/mp4"}, MaxBytes: 200 * 1024 * 1024},
	handlers.MediaTypeVideo:       {Types: []string{"video/mp4"}, MaxBytes: 200 * 1024 * 1024},
	handlers.MediaTypeApplication: {},
}

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("LN"), "Line")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, handlers.JSONPayload(h, h.receiveMessage))
	return nil
}

//	{
//		"events": [
//		  {
//			"replyToken": "nHuyWiB7yP5Zw52FIkcQobQuGDXCTA",
//			"type": "message",
//			"timestamp": 1462629479859,
//			"source": {
//			  "type": "user",
//			  "userId": "U4af4980629..."
//			},
//			"message": {
//			  "id": "325708",
//			  "type": "text",
//			  "text": "Hello, world"
//			}
//		  },
//		  {
//			"replyToken": "nHuyWiB7yP5Zw52FIkcQobQuGDXCTA",
//			"type": "follow",
//			"timestamp": 1462629479859,
//			"source": {
//			  "type": "user",
//			  "userId": "U4af4980629..."
//			}
//		  }
//		]
//	}
type moPayload struct {
	Events []struct {
		ReplyToken string `json:"replyToken"`
		Type       string `json:"type"`
		Timestamp  int64  `json:"timestamp"`
		Source     struct {
			Type   string `json:"type"`
			UserID string `json:"userId"`
		} `json:"source"`
		Message struct {
			ID              string  `json:"id"`
			Type            string  `json:"type"`
			Text            string  `json:"text"`
			Title           string  `json:"title"`
			Address         string  `json:"address"`
			Latitude        float64 `json:"latitude"`
			Longitude       float64 `json:"longitude"`
			ContentProvider struct {
				Type               string `json:"type"`
				OriginalContentURL string `json:"originalContentUrl"`
			} `json:"contentProvider"`
		} `json:"message"`
	} `json:"events"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *moPayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	err := h.validateSignature(channel, r)
	if err != nil {
		return nil, err
	}

	msgs := []courier.MsgIn{}

	for _, lineEvent := range payload.Events {
		if lineEvent.ReplyToken == "" || (lineEvent.Source.Type == "" && lineEvent.Source.UserID == "") || (lineEvent.Message.Type == "" && lineEvent.Message.ID == "") {
			continue
		}

		text := ""
		mediaURL := ""

		lineEventMsgType := lineEvent.Message.Type

		if lineEventMsgType == "text" {
			text = lineEvent.Message.Text

		} else if lineEventMsgType == "audio" || lineEventMsgType == "video" || lineEventMsgType == "image" || lineEventMsgType == "file" {
			if lineEvent.Message.ContentProvider.Type == "line" || lineEventMsgType == "file" {
				mediaURL = buildMediaURL(lineEvent.Message.ID)
			} else if lineEvent.Message.ContentProvider.Type == "external" {
				mediaURL = lineEvent.Message.ContentProvider.OriginalContentURL
			}

		} else if lineEventMsgType == "location" {
			mediaURL = fmt.Sprintf("geo:%f,%f", lineEvent.Message.Latitude, lineEvent.Message.Longitude)
			text = lineEvent.Message.Title
		} else {
			continue
		}

		// create our date from the timestamp (they give us millis, arg is nanos)
		date := time.Unix(0, lineEvent.Timestamp*1000000).UTC()

		urn, err := urns.NewURNFromParts(urns.LineScheme, lineEvent.Source.UserID, "", "")
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}

		msg := h.Backend().NewIncomingMsg(channel, urn, text, lineEvent.ReplyToken, clog).WithReceivedOn(date)

		if mediaURL != "" {
			msg.WithAttachment(mediaURL)
		}

		msgs = append(msgs, msg)
	}

	if len(msgs) == 0 {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring request, no message")
	}

	return handlers.WriteMsgsAndResponse(ctx, h, msgs, w, r, clog)

}

func buildMediaURL(mediaID string) string {
	mediaURL, _ := url.Parse(fmt.Sprintf("%s/%s/content", mediaDataURL, mediaID))
	return mediaURL.String()
}

// BuildAttachmentRequest to download media for message attachment with Bearer token set
func (h *handler) BuildAttachmentRequest(ctx context.Context, b courier.Backend, channel courier.Channel, attachmentURL string, clog *courier.ChannelLog) (*http.Request, error) {
	token := channel.StringConfigForKey(courier.ConfigAuthToken, "")
	if token == "" {
		return nil, fmt.Errorf("missing token for LN channel")
	}

	// set the access token as the authorization header
	req, _ := http.NewRequest(http.MethodGet, attachmentURL, nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	return req, nil
}

var _ courier.AttachmentRequestBuilder = (*handler)(nil)

func (h *handler) validateSignature(channel courier.Channel, r *http.Request) error {
	actual := r.Header.Get(signatureHeader)
	if actual == "" {
		return fmt.Errorf("missing request signature")
	}

	confSecret := channel.ConfigForKey(courier.ConfigSecret, "")
	secret, isStr := confSecret.(string)
	if !isStr || secret == "" {
		return fmt.Errorf("invalid or missing auth token in config")
	}

	expected, err := calculateSignature(secret, r)
	if err != nil {
		return err
	}

	// compare signatures in way that isn't sensitive to a timing attack
	if !hmac.Equal(expected, []byte(actual)) {
		return fmt.Errorf("invalid request signature")
	}

	return nil
}

// see https://developers.line.me/en/docs/messaging-api/reference/#signature-validation
func calculateSignature(secret string, r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	// hash with SHA256
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	hash := mac.Sum(nil)

	// encode with Base64
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(hash)))
	base64.StdEncoding.Encode(encoded, hash)
	return encoded, nil
}

type mtTextMsg struct {
	Type       string        `json:"type"`
	Text       string        `json:"text"`
	QuickReply *mtQuickReply `json:"quickReply,omitempty"`
}

type mtQuickReply struct {
	Items []QuickReplyItem `json:"items"`
}

type QuickReplyItem struct {
	Type   string `json:"type"`
	Action struct {
		Type  string `json:"type"`
		Label string `json:"label"`
		Text  string `json:"text"`
	} `json:"action"`
}

type mtImageMsg struct {
	Type       string `json:"type"`
	URL        string `json:"originalContentUrl"`
	PreviewURL string `json:"previewImageUrl"`
}

type mtVideoMsg struct {
	Type       string `json:"type"`
	URL        string `json:"originalContentUrl"`
	PreviewURL string `json:"previewImageUrl"`
}

type mtAudioMsg struct {
	Type     string `json:"type"`
	URL      string `json:"originalContentUrl"`
	Duration int    `json:"duration"`
}

type mtPayload struct {
	To         string          `json:"to,omitempty"`
	ReplyToken string          `json:"replyToken,omitempty"`
	Messages   json.RawMessage `json:"messages"`
}

type mtResponse struct {
	Message string `json:"message"`
}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	authToken := msg.Channel().StringConfigForKey(courier.ConfigAuthToken, "")
	if authToken == "" {
		return nil, fmt.Errorf("no auth token set for LN channel: %s", msg.Channel().UUID())
	}
	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)

	// all msg parts in JSON
	var jsonMsgs []string
	parts := handlers.SplitMsgByChannel(msg.Channel(), msg.Text(), maxMsgLength)
	qrs := msg.QuickReplies()

	attachments, err := handlers.ResolveAttachments(ctx, h.Backend(), msg.Attachments(), mediaSupport, false)
	if err != nil {
		return nil, errors.Wrap(err, "error resolving attachments")
	}

	// fill all msg parts with attachment parts
	for _, attachment := range attachments {

		var jsonMsg []byte
		var err error

		switch attachment.Type {
		case handlers.MediaTypeImage:
			jsonMsg, err = json.Marshal(mtImageMsg{Type: "image", URL: attachment.Media.URL(), PreviewURL: attachment.Media.URL()})
		case handlers.MediaTypeVideo:
			jsonMsg, err = json.Marshal(mtVideoMsg{Type: "video", URL: attachment.Media.URL(), PreviewURL: attachment.Thumbnail.URL()})
		case handlers.MediaTypeAudio:
			jsonMsg, err = json.Marshal(mtAudioMsg{Type: "audio", URL: attachment.Media.URL(), Duration: attachment.Media.Duration()})
		default:
			jsonMsg, err = json.Marshal(mtTextMsg{Type: "text", Text: attachment.URL})
		}

		if err == nil {
			jsonMsgs = append(jsonMsgs, string(jsonMsg))
		}
	}

	// fill all msg parts with text parts
	for i, part := range parts {
		if i < (len(parts) - 1) {
			if jsonMsg, err := json.Marshal(mtTextMsg{Type: "text", Text: part}); err == nil {
				jsonMsgs = append(jsonMsgs, string(jsonMsg))
			}
		} else {
			mtTextMsg := mtTextMsg{Type: "text", Text: part}
			items := make([]QuickReplyItem, len(qrs))
			for j, qr := range qrs {
				items[j] = QuickReplyItem{Type: "action"}
				items[j].Action.Type = "message"
				items[j].Action.Label = qr
				items[j].Action.Text = qr
			}
			if len(items) > 0 {
				mtTextMsg.QuickReply = &mtQuickReply{Items: items}
			}
			if jsonMsg, err := json.Marshal(mtTextMsg); err == nil {
				jsonMsgs = append(jsonMsgs, string(jsonMsg))
			}
		}
	}

	// send msg parts in batches
	var batch []string
	batchCount := 0

	for i, jsonMsg := range jsonMsgs {
		batch = append(batch, jsonMsg)
		batchCount++

		if batchCount == maxMsgSend || (i == len(jsonMsgs)-1) {
			req, err := buildSendMsgRequest(authToken, msg.URN().Path(), msg.ResponseToExternalID(), batch)
			if err != nil {
				return status, err
			}

			resp, respBody, err := h.RequestHTTP(req, clog)

			if err == nil && resp.StatusCode/100 == 2 {
				batch = []string{}
				batchCount = 0
				continue
			}

			respPayload := &mtResponse{}
			err = json.Unmarshal(respBody, respPayload)
			if err != nil {
				clog.Error(courier.ErrorResponseUnparseable("JSON"))
				return status, nil
			}

			errMsg := respPayload.Message
			if errMsg == "Invalid reply token" {
				req, err = buildSendMsgRequest(authToken, msg.URN().Path(), "", batch)
				if err != nil {
					return status, err
				}

				resp, respBody, _ := h.RequestHTTP(req, clog)

				respPayload := &mtResponse{}
				err = json.Unmarshal(respBody, respPayload)
				if err != nil {
					clog.Error(courier.ErrorResponseUnparseable("JSON"))
					return status, nil
				}

				if resp.StatusCode/100 != 2 {
					clog.Error(courier.ErrorExternal(strconv.Itoa(resp.StatusCode), respPayload.Message))
					return status, nil
				}
			} else {
				clog.Error(courier.ErrorExternal(strconv.Itoa(resp.StatusCode), respPayload.Message))
				return status, err
			}
		}
	}
	status.SetStatus(courier.MsgStatusWired)
	return status, nil
}

func buildSendMsgRequest(authToken, to string, replyToken string, jsonMsgs []string) (*http.Request, error) {
	// convert from string slice to bytes JSON
	rawJsonMsgs := bytes.Buffer{}
	rawJsonMsgs.WriteString("[")

	for i, msgJson := range jsonMsgs {
		rawJsonMsgs.WriteString(msgJson)

		if i < len(jsonMsgs)-1 {
			rawJsonMsgs.WriteString(",")
		}
	}
	rawJsonMsgs.WriteString("]")
	payload := mtPayload{Messages: rawJsonMsgs.Bytes()}
	sendURL := ""

	// check if this sending is a response to user
	if replyToken != "" {
		sendURL = replySendURL
		payload.ReplyToken = replyToken
	} else {
		sendURL = pushSendURL
		payload.To = to
	}
	body := &bytes.Buffer{}

	if err := json.NewEncoder(body).Encode(payload); err != nil {
		return nil, err
	}
	// build our request
	req, err := http.NewRequest(http.MethodPost, sendURL, body)

	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authToken))

	return req, nil
}
