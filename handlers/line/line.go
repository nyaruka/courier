package line

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/buger/jsonparser"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/nyaruka/courier/utils"

	"github.com/nyaruka/gocommon/urns"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
)

var (
	replySendURL = "https://api.line.me/v2/bot/message/reply"
	pushSendURL  = "https://api.line.me/v2/bot/message/push"
	maxMsgLength = 2000
	maxMsgSend   = 5

	signatureHeader = "X-Line-Signature"
)

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
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
	return nil
}

// {
// 	"events": [
// 	  {
// 		"replyToken": "nHuyWiB7yP5Zw52FIkcQobQuGDXCTA",
// 		"type": "message",
// 		"timestamp": 1462629479859,
// 		"source": {
// 		  "type": "user",
// 		  "userId": "U4af4980629..."
// 		},
// 		"message": {
// 		  "id": "325708",
// 		  "type": "text",
// 		  "text": "Hello, world"
// 		}
// 	  },
// 	  {
// 		"replyToken": "nHuyWiB7yP5Zw52FIkcQobQuGDXCTA",
// 		"type": "follow",
// 		"timestamp": 1462629479859,
// 		"source": {
// 		  "type": "user",
// 		  "userId": "U4af4980629..."
// 		}
// 	  }
// 	]
// }
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
			ID   string `json:"id"`
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"message"`
	} `json:"events"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	err := h.validateSignature(channel, r)
	if err != nil {
		return nil, err
	}

	payload := &moPayload{}
	err = handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	msgs := []courier.Msg{}

	for _, lineEvent := range payload.Events {
		if lineEvent.ReplyToken == "" || (lineEvent.Source.Type == "" && lineEvent.Source.UserID == "") || (lineEvent.Message.Type == "" && lineEvent.Message.ID == "" && lineEvent.Message.Text == "") || lineEvent.Message.Type != "text" {
			continue
		}

		// create our date from the timestamp (they give us millis, arg is nanos)
		date := time.Unix(0, lineEvent.Timestamp*1000000).UTC()

		urn, err := urns.NewURNFromParts(urns.LineScheme, lineEvent.Source.UserID, "", "")
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}

		msg := h.Backend().NewIncomingMsg(channel, urn, lineEvent.Message.Text).WithExternalID(lineEvent.ReplyToken).WithReceivedOn(date)
		msgs = append(msgs, msg)
	}

	if len(msgs) == 0 {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring request, no message")
	}

	return handlers.WriteMsgsAndResponse(ctx, h, msgs, w, r)

}

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
	body, err := ioutil.ReadAll(r.Body)
	r.Body = ioutil.NopCloser(bytes.NewBuffer(body))
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
	Type string `json:"type"`
	Text string `json:"text"`
}

type mtImageMsg struct {
	Type       string `json:"type"`
	URL        string `json:"originalContentUrl"`
	PreviewURL string `json:"previewImageUrl"`
}

type mtPayload struct {
	To         string        `json:"to,omitempty"`
	ReplyToken string        `json:"replyToken,omitempty"`
	Messages json.RawMessage `json:"messages"`
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	authToken := msg.Channel().StringConfigForKey(courier.ConfigAuthToken, "")
	if authToken == "" {
		return nil, fmt.Errorf("no auth token set for LN channel: %s", msg.Channel().UUID())
	}
	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)

	// all msg parts in JSON
	var jsonMsgs []string
	parts := handlers.SplitMsgByChannel(msg.Channel(), msg.Text(), maxMsgLength)
	// fill all msg parts with text parts
	for _, part := range parts {
		if jsonMsg, err := json.Marshal(mtTextMsg{Type: "text", Text: part}); err == nil {
			jsonMsgs = append(jsonMsgs, string(jsonMsg))
		}
	}
	// fill all msg parts with attachment parts
	for _, attachment := range msg.Attachments() {
		var jsonMsg []byte
		var err error

		prefix, url := handlers.SplitAttachment(attachment)

		switch mediaType := strings.Split(prefix, "/")[0]; mediaType {
		case "image":
			jsonMsg, err = json.Marshal(mtImageMsg{Type: "image", URL: url, PreviewURL: url})
		default:
			jsonMsg, err = json.Marshal(mtTextMsg{Type: "text", Text: url})
		}
		if err == nil {
			jsonMsgs = append(jsonMsgs, string(jsonMsg))
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
			rr, err := utils.MakeHTTPRequest(req)
			log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
			status.AddLog(log)

			if err == nil {
				batch = []string{}
				batchCount = 0
				continue
			}
			// retry without the reply token if it's invalid
			errMsg, err := jsonparser.GetString(rr.Body, "message")
			if err == nil && errMsg == "Invalid reply token" {
				req, err = buildSendMsgRequest(authToken, msg.URN().Path(), "", batch)
				if err != nil {
					return status, err
				}
				rr, err = utils.MakeHTTPRequest(req)
				log = courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
				status.AddLog(log)
				if err != nil {
					return status, err
				}
			} else {
				return status, err
			}
		}
	}
	status.SetStatus(courier.MsgWired)
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
