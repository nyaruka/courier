package facebook

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
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
	"github.com/pkg/errors"
)

// Endpoints we hit
var twitterSendURL = "https://api.twitter.com/1.1/direct_messages/events/new.json"

// DMs can be up to 10,000 characters long
var maxMsgLength = 10000

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("TWT"), "Twitter")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	err := s.AddHandlerRoute(h, http.MethodPost, "receive", h.Receive)
	if err != nil {
		return err
	}
	return s.AddHandlerRoute(h, http.MethodGet, "receive", h.ReceiveVerify)
}

// ReceiveVerify handles Twitter's webhook verification callback
func (h *handler) ReceiveVerify(ctx context.Context, c courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	crcToken := r.URL.Query().Get("crc_token")
	if crcToken == "" {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, c, fmt.Errorf(`missing required 'crc_token' query parameter`))
	}

	secret := c.StringConfigForKey(courier.ConfigSecret, "")
	if secret == "" {
		return nil, fmt.Errorf("TWT channel missing required secret in channel config")
	}

	signature := generateSignature(secret, crcToken)
	w.Header().Set("Content-Type", "application/json")
	_, err := w.Write([]byte(fmt.Sprintf(`{"response_token": "%s"}`, signature)))

	return nil, err
}

// struct for each user
type twtUser struct {
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
type moEnvelope struct {
	DirectMessageEvents []struct {
		CreatedTimestamp string `json:"created_timestamp" validate:"required"`
		MessageCreate    struct {
			MessageData struct {
				Text string `json:"text"`
			} `json:"message_data"`
			SenderID string `json:"sender_id" validate:"required"`
			Target   struct {
				RecipientID string `json:"recipient_id" validate:"required"`
			} `json:"target"`
		} `json:"message_create"`
		Type string `json:"type" validate:"required"`
		ID   string `json:"id"   validate:"required"`
	} `json:"direct_message_events"`
	Users map[string]twtUser `json:"users"`
}

// Receive is our HTTP handler function for incoming messages
func (h *handler) Receive(ctx context.Context, c courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	// read our handle id
	handleID := c.StringConfigForKey("handle_id", "")
	if handleID == "" {
		return nil, fmt.Errorf("Missing handle id config for TWT channel")
	}

	mo := &moEnvelope{}
	err := handlers.DecodeAndValidateJSON(mo, r)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, c, err)
	}

	// no direct message events? ignore
	if len(mo.DirectMessageEvents) == 0 {
		return nil, courier.WriteAndLogRequestIgnored(ctx, w, r, c, "ignoring, no direct messages")
	}

	// the list of messages we read
	msgs := make([]courier.Msg, 0, 2)
	events := make([]courier.Event, 0, 2)

	// for each entry
	for _, entry := range mo.DirectMessageEvents {
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
		user, found := mo.Users[senderID]
		if !found {
			return nil, courier.WriteAndLogRequestError(ctx, w, r, c, fmt.Errorf("unable to find user for id: %s", senderID))
		}

		urn := urns.NewURNFromParts(urns.TwitterIDScheme, user.ID, user.ScreenName)

		// create our date from the timestamp (they give us millis, arg is nanos)
		ts, err := strconv.ParseInt(entry.CreatedTimestamp, 10, 64)
		if err != nil {
			return nil, courier.WriteAndLogRequestError(ctx, w, r, c, fmt.Errorf("invalid timestamp: %s", entry.CreatedTimestamp))
		}
		date := time.Unix(0, ts*1000000).UTC()

		// create our message
		msg := h.Backend().NewIncomingMsg(c, urn, entry.MessageCreate.MessageData.Text).WithExternalID(entry.ID).WithReceivedOn(date).WithContactName(user.Name)
		err = h.Backend().WriteMsg(ctx, msg)
		if err != nil {
			return nil, err
		}

		msgs = append(msgs, msg)
		events = append(events, msg)
	}

	return events, courier.WriteMsgSuccess(ctx, w, r, msgs)
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
type mtEnvelope struct {
	Event struct {
		Type          string `json:"type"`
		MessageCreate struct {
			Target struct {
				RecipientID string `json:"recipient_id"`
			} `json:"target"`
			MessageData struct {
				Text       string `json:"text"`
				QuickReply *mtQR  `json:"quick_reply,omitempty"`
			} `json:"message_data"`
		} `json:"message_create"`
	} `json:"event"`
}

type mtQR {
				QuickReply struct {
					Type    string `json:"type"`
					Options []mtQROption {
						Label string `json:"label"`
					} `json:"options"`
}


type mtQROption struct {
	Title       string `json:"title"`
	Payload     string `json:"payload"`
	ContentType string `json:"content_type"`
}

func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	// can't do anything without an access token
	accessToken := msg.Channel().StringConfigForKey(courier.ConfigAuthToken, "")
	if accessToken == "" {
		return nil, fmt.Errorf("missing access token")
	}

	payload := mtEnvelope{}

	// set our message type
	if msg.ResponseToID().IsZero() {
		payload.MessagingType = "NON_PROMOTIONAL_SUBSCRIPTION"
	} else {
		payload.MessagingType = "RESPONSE"
	}

	// build our recipient
	if msg.URN().IsFacebookRef() {
		payload.Recipient.UserRef = msg.URN().FacebookRef()
	} else {
		payload.Recipient.ID = msg.URN().Path()
	}

	sendURL, _ := url.Parse(facebookSendURL)
	query := url.Values{}
	query.Set("access_token", accessToken)
	sendURL.RawQuery = query.Encode()

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)

	msgParts := make([]string, 0)
	if msg.Text() != "" {
		msgParts = handlers.SplitMsg(msg.Text(), maxMsgLength)
	}

	// send each part and each attachment separately
	for i := 0; i < len(msgParts)+len(msg.Attachments()); i++ {
		if i < len(msgParts) {
			// this is still a msg part
			payload.Message.Text = msgParts[i]
			payload.Message.Attachment = nil
		} else {
			// this is an attachment
			payload.Message.Attachment = &mtAttachment{}
			attType, attURL := courier.SplitAttachment(msg.Attachments()[i-len(msgParts)])
			attType = strings.Split(attType, "/")[0]
			payload.Message.Attachment.Type = attType
			payload.Message.Attachment.Payload.URL = attURL
			payload.Message.Attachment.Payload.IsReusable = true
			payload.Message.Text = ""
		}

		// include any quick replies on the first piece we send
		if i == 0 {
			for _, qr := range msg.QuickReplies() {
				payload.Message.QuickReplies = append(payload.Message.QuickReplies, mtQuickReply{qr, qr, "text"})
			}
		} else {
			payload.Message.QuickReplies = nil
		}

		jsonBody, err := json.Marshal(payload)
		if err != nil {
			return status, err
		}

		req, err := http.NewRequest(http.MethodPost, sendURL.String(), bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		rr, err := utils.MakeHTTPRequest(req)

		// record our status and log
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
		status.AddLog(log)
		if err != nil {
			return status, nil
		}

		externalID, err := jsonparser.GetString(rr.Body, "message_id")
		if err != nil {
			log.WithError("Message Send Error", errors.Errorf("unable to get message_id from body"))
			return status, nil
		}

		// if this is our first message, record the external id
		if i == 0 {
			status.SetExternalID(externalID)
		}

		// this was wired successfully
		status.SetStatus(courier.MsgWired)
	}

	return status, nil
}

// ReceiveVerify handles Facebook's webhook verification callback
func (h *handler) DescribeURN(ctx context.Context, channel courier.Channel, urn urns.URN) (map[string]string, error) {
	// can't do anything with facebook refs, ignore them
	if urn.IsFacebookRef() {
		return map[string]string{}, nil
	}

	accessToken := channel.StringConfigForKey(courier.ConfigAuthToken, "")
	if accessToken == "" {
		return nil, fmt.Errorf("missing access token")
	}

	// build a request to lookup the stats for this contact
	base, _ := url.Parse(facebookGraphURL)
	path, _ := url.Parse(fmt.Sprintf("/%s", urn.Path()))
	u := base.ResolveReference(path)

	query := url.Values{}
	query.Set("fields", "first_name,last_name")
	query.Set("access_token", accessToken)
	u.RawQuery = query.Encode()
	req, _ := http.NewRequest(http.MethodGet, u.String(), nil)
	rr, err := utils.MakeHTTPRequest(req)
	if err != nil {
		return nil, fmt.Errorf("unable to look up contact data:%s\n%s", err, rr.Response)
	}

	// read our first and last name
	firstName, _ := jsonparser.GetString(rr.Body, "first_name")
	lastName, _ := jsonparser.GetString(rr.Body, "last_name")

	return map[string]string{"name": utils.JoinNonEmpty(" ", firstName, lastName)}, nil
}

// hashes the passed in content in sha256 using the passed in secret
func generateSignature(secret string, content string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(content))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
