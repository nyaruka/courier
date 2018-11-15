package twitter

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/dghubble/oauth1"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
	"github.com/pkg/errors"
)

var (
	apiURL       = "https://api.twitter.com/1.1"
	maxMsgLength = 10000

	// Labels in quick replies can't be more than 36 characters
	maxOptionLength = 36
)

const (
	configHandleID          = "handle_id"
	configAPIKey            = "api_key"
	configAPISecret         = "api_secret"
	configAccessToken       = "access_token"
	configAccessTokenSecret = "access_token_secret"
)

func init() {
	courier.RegisterHandler(newHandler("TWT", "Twitter Activity"))
	courier.RegisterHandler(newHandler("TT", "Twitter"))
}

type handler struct {
	handlers.BaseHandler
}

func newHandler(channelType string, name string) courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType(channelType), name)}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveEvent)
	s.AddHandlerRoute(h, http.MethodGet, "receive", h.receiveVerify)
	return nil
}

// receiveVerify handles Twitter's webhook verification callback
func (h *handler) receiveVerify(ctx context.Context, c courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	crcToken := r.URL.Query().Get("crc_token")
	if crcToken == "" {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, fmt.Errorf(`missing required 'crc_token' query parameter`))
	}

	secret := c.StringConfigForKey("api_secret", "")
	if secret == "" {
		return nil, fmt.Errorf("TWT channel missing required secret in channel config")
	}

	signature := generateSignature(secret, crcToken)
	w.Header().Set("Content-Type", "application/json")
	_, err := w.Write([]byte(fmt.Sprintf(`{"response_token": "sha256=%s"}`, signature)))

	return nil, err
}

// struct for each user
type moUser struct {
	ID         string `json:"id"          validate:"required"`
	Name       string `json:"name"        validate:"required"`
	ScreenName string `json:"screen_name" validate:"required"`
}

// {
//    "direct_message_events": [
//      {
//	      "created_timestamp": "1494877823220",
//        "message_create": {
//          "message_data": {
//            "text": "hello world!",
//          },
//          "sender_id": "twitterid1",
//          "target": {"recipient_id": "twitterid2" }
//        },
//        "type": "message_create",
//        "id": "twitterMsgId"
//      }
//    ],
//    "users": {
//       "twitterid1": { "id": "twitterid1", "name": "joe", "screen_name": "joe" },
//       "twitterid2": { "id": "twitterid2", "name": "jane", "screen_name": "jane" },
//    }
// }
type moPayload struct {
	DirectMessageEvents []struct {
		CreatedTimestamp string `json:"created_timestamp" validate:"required"`
		MessageCreate    struct {
			MessageData struct {
				Text       string `json:"text"`
				Attachment *struct {
					Media struct {
						MediaURLHTTPS string `json:"media_url_https"`
					} `json:"media"`
				} `json:"attachment,omitempty"`
			} `json:"message_data"`
			SenderID string `json:"sender_id" validate:"required"`
			Target   struct {
				RecipientID string `json:"recipient_id" validate:"required"`
			} `json:"target"`
		} `json:"message_create"`
		Type string `json:"type" validate:"required"`
		ID   string `json:"id"   validate:"required"`
	} `json:"direct_message_events"`
	Users map[string]moUser `json:"users"`
}

// receiveEvent is our HTTP handler function for incoming events
func (h *handler) receiveEvent(ctx context.Context, c courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	// read our handle id
	handleID := c.StringConfigForKey(configHandleID, "")
	if handleID == "" {
		return nil, fmt.Errorf("Missing handle id config for TWT channel")
	}

	payload := &moPayload{}
	err := handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
	}

	// no direct message events? ignore
	if len(payload.DirectMessageEvents) == 0 {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, c, w, r, "ignoring, no direct messages")
	}

	// the list of messages we read
	msgs := make([]courier.Msg, 0, 2)

	// for each entry
	for _, entry := range payload.DirectMessageEvents {
		// not a message create, ignore
		if entry.Type != "message_create" {
			continue
		}

		senderID := entry.MessageCreate.SenderID

		// ignore this entry if we sent it
		if senderID == handleID {
			continue
		}

		// look up the user for this sender
		user, found := payload.Users[senderID]
		if !found {
			return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, fmt.Errorf("unable to find user for id: %s", senderID))
		}

		urn, err := urns.NewURNFromParts(urns.TwitterIDScheme, user.ID, "", strings.ToLower(user.ScreenName))
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
		}

		// create our date from the timestamp (they give us millis, arg is nanos)
		ts, err := strconv.ParseInt(entry.CreatedTimestamp, 10, 64)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, fmt.Errorf("invalid timestamp: %s", entry.CreatedTimestamp))
		}
		date := time.Unix(0, ts*1000000).UTC()

		// Twitter escapes & in HTML format, so replace &amp; with &
		text := strings.Replace(entry.MessageCreate.MessageData.Text, "&amp;", "&", -1)

		// create our message
		msg := h.Backend().NewIncomingMsg(c, urn, text).WithExternalID(entry.ID).WithReceivedOn(date).WithContactName(user.Name)

		// if we have an attachment, add that as well
		if entry.MessageCreate.MessageData.Attachment != nil {
			msg.WithAttachment(entry.MessageCreate.MessageData.Attachment.Media.MediaURLHTTPS)
		}

		msgs = append(msgs, msg)
	}

	return handlers.WriteMsgsAndResponse(ctx, h, msgs, w, r)
}

// {
//   "event": {
//     "type": "message_create",
//     "message_create": {
//       "target": {
//         "recipient_id": "844385345234"
//       },
//       "message_data": {
//         "text": "Hello World!",
//         "quick_reply": {
//	         "type": "options",
//           "options": [
//	           { "label": "Red"}, {"label": "Green"}
//           ]
//         }
//       }
//     }
//	 }
// }
type mtPayload struct {
	Event struct {
		Type          string `json:"type"`
		MessageCreate struct {
			Target struct {
				RecipientID string `json:"recipient_id"`
			} `json:"target"`
			MessageData struct {
				Text       string        `json:"text"`
				QuickReply *mtQR         `json:"quick_reply,omitempty"`
				Attachment *mtAttachment `json:"attachment,omitempty"`
			} `json:"message_data"`
		} `json:"message_create"`
	} `json:"event"`
}

type mtQR struct {
	Type    string       `json:"type"`
	Options []mtQROption `json:"options"`
}

type mtQROption struct {
	Label string `json:"label"`
}

type mtAttachment struct {
	Type  string `json:"type"`
	Media struct {
		ID string `json:"id"`
	} `json:"media"`
}

func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	apiKey := msg.Channel().StringConfigForKey(configAPIKey, "")
	apiSecret := msg.Channel().StringConfigForKey(configAPISecret, "")
	accessToken := msg.Channel().StringConfigForKey(configAccessToken, "")
	accessSecret := msg.Channel().StringConfigForKey(configAccessTokenSecret, "")
	if apiKey == "" || apiSecret == "" || accessToken == "" || accessSecret == "" {
		return nil, fmt.Errorf("missing api or tokens for TWT channel")
	}

	// create our OAuth client that will take care of signing
	config := oauth1.NewConfig(apiKey, apiSecret)
	token := oauth1.NewToken(accessToken, accessSecret)
	client := config.Client(ctx, token)

	url, err := url.Parse(apiURL)
	sendPath, _ := url.Parse("direct_messages/events/new.json")
	sendURL := url.ResolveReference(sendPath).String()

	mediaPath, _ := url.Parse("media/upload.json")
	mediaURL := url.ResolveReference(mediaPath).String()

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	var log *courier.ChannelLog

	msgParts := make([]string, 0)
	if msg.Text() != "" {
		msgParts = handlers.SplitMsg(msg.Text(), maxMsgLength)
	}

	for i := 0; i < len(msgParts)+len(msg.Attachments()); i++ {
		payload := mtPayload{}
		payload.Event.Type = "message_create"
		payload.Event.MessageCreate.Target.RecipientID = msg.URN().Path()

		if i < len(msgParts) {
			// this is still a msg part
			payload.Event.MessageCreate.MessageData.Text = msgParts[i]
		} else {
			attachment := msg.Attachments()[i-len(msgParts)]
			mimeType, s3url := handlers.SplitAttachment(attachment)
			mediaID := ""
			if strings.HasPrefix(mimeType, "image") || strings.HasPrefix(mimeType, "video") {
				mediaID, log, err = uploadMediaToTwitter(msg, mediaURL, mimeType, s3url, client)
				status.AddLog(log)
				if err != nil {
					log.WithError("Unable to upload media to Twitter server", err)
				}
			} else {
				log.WithError("Unable to upload media, Unsupported Twitter attachment", err)
			}

			if mediaID != "" {
				mtAttachment := &mtAttachment{}
				mtAttachment.Type = "media"
				mtAttachment.Media.ID = mediaID
				payload.Event.MessageCreate.MessageData.Attachment = mtAttachment
				payload.Event.MessageCreate.MessageData.Text = ""
			}
		}

		// attach quick replies if we have them
		if i == 0 && len(msg.QuickReplies()) > 0 {
			qrs := &mtQR{}
			qrs.Type = "options"
			for _, option := range msg.QuickReplies() {
				if len(option) > maxOptionLength {
					option = option[:maxOptionLength]
				}
				qrs.Options = append(qrs.Options, mtQROption{option})
			}
			payload.Event.MessageCreate.MessageData.QuickReply = qrs
		}

		jsonBody, err := json.Marshal(payload)
		if err != nil {
			return status, err
		}

		req, _ := http.NewRequest(http.MethodPost, sendURL, bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		rr, err := utils.MakeHTTPRequestWithClient(req, client)

		// record our status and log
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
		status.AddLog(log)
		if err != nil {
			return status, nil
		}

		externalID, err := jsonparser.GetString(rr.Body, "event", "id")
		if err != nil {
			log.WithError("Message Send Error", errors.Errorf("unable to get message_id from body"))
			return status, nil
		}

		// if this is our first message, record the external id
		if i == 0 {
			status.SetExternalID(externalID)
		}

		if err == nil {
			// this was wired successfully
			status.SetStatus(courier.MsgWired)
		}
	}

	return status, nil
}

// hashes the passed in content in sha256 using the passed in secret
func generateSignature(secret string, content string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(content))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func uploadMediaToTwitter(msg courier.Msg, mediaUrl string, attachmentMimeType string, attachmentURL string, client *http.Client) (string, *courier.ChannelLog, error) {
	// retrieve the media to be sent from S3
	req, _ := http.NewRequest(http.MethodGet, attachmentURL, nil)
	s3rr, err := utils.MakeHTTPRequest(req)
	if err != nil {
		return "", courier.NewChannelLogFromRR("Media Fetch", msg.Channel(), msg.ID(), s3rr), err
	}

	mediaCategory := ""
	if strings.HasPrefix(attachmentMimeType, "image") {
		mediaCategory = "dm_image"
	} else if strings.HasPrefix(attachmentMimeType, "video") {
		mediaCategory = "dm_video"
	}

	// upload it to WhatsApp in exchange for a media id
	form := url.Values{
		"command":     []string{"INIT"},
		"media_type":  []string{attachmentMimeType},
		"total_bytes": []string{fmt.Sprintf("%d", len(s3rr.Body))},
	}

	if mediaCategory != "" {
		form["media_category"] = []string{mediaCategory}
	}

	twReq, _ := http.NewRequest(http.MethodPost, mediaUrl, strings.NewReader(form.Encode()))
	twReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	twReq.Header.Set("Accept", "application/json")
	twReq.Header.Set("User-Agent", utils.HTTPUserAgent)
	twrr, err := utils.MakeHTTPRequestWithClient(twReq, client)
	if err != nil {
		return "", courier.NewChannelLogFromRR("Media Upload Error INIT", msg.Channel(), msg.ID(), twrr), err
	}

	mediaID, err := jsonparser.GetString(twrr.Body, "media_id_string")
	if err != nil {
		return "", courier.NewChannelLogFromRR("Media Upload Error", msg.Channel(), msg.ID(), twrr), err
	}

	form = url.Values{
		"command":       []string{"APPEND"},
		"media_id":      []string{mediaID},
		"segment_index": []string{"0"},
	}

	twReq, _ = http.NewRequest(http.MethodPost, mediaUrl, strings.NewReader(form.Encode()))
	twReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	twReq.Header.Set("Accept", "application/json")
	twReq.Header.Set("User-Agent", utils.HTTPUserAgent)
	twrr, err = utils.MakeHTTPRequestWithClient(twReq, client)
	if err != nil {
		return "", courier.NewChannelLogFromRR("Media Upload Error APPEND", msg.Channel(), msg.ID(), twrr), err
	}

	form = url.Values{
		"command":  []string{"FINALIZE"},
		"media_id": []string{mediaID},
	}

	twReq, _ = http.NewRequest(http.MethodPost, mediaUrl, strings.NewReader(form.Encode()))
	twReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	twReq.Header.Set("Accept", "application/json")
	twReq.Header.Set("User-Agent", utils.HTTPUserAgent)
	twrr, err = utils.MakeHTTPRequestWithClient(twReq, client)
	if err != nil {
		return "", courier.NewChannelLogFromRR("Media Upload Error FINALIZE", msg.Channel(), msg.ID(), twrr), err
	}

	log := courier.NewChannelLogFromRR("Media Upload success", msg.Channel(), msg.ID(), twrr)

	if err != nil {
		return "", log, err
	}

	return mediaID, log, nil
}
