package facebook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/urns"
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

type mtQR struct {
	QuickReply struct {
		Type    string       `json:"type"`
		Options []mtQROption `json:"options"`
	}
}

type mtQROption struct {
	Title       string `json:"title"`
	Payload     string `json:"payload"`
	ContentType string `json:"content_type"`
}

func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	return nil, fmt.Errorf("sending not implemented for twitter")
}

// hashes the passed in content in sha256 using the passed in secret
func generateSignature(secret string, content string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(content))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
