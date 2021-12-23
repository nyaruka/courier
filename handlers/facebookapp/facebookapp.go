package facebookapp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
var (
	sendURL  = "https://graph.facebook.com/v12.0/me/messages"
	graphURL = "https://graph.facebook.com/v12.0/"

	signatureHeader = "X-Hub-Signature"

	// max for the body
	maxMsgLength = 1000

	// Sticker ID substitutions
	stickerIDToEmoji = map[int64]string{
		369239263222822: "👍", // small
		369239343222814: "👍", // medium
		369239383222810: "👍", // big
	}

	tagByTopic = map[string]string{
		"event":    "CONFIRMED_EVENT_UPDATE",
		"purchase": "POST_PURCHASE_UPDATE",
		"account":  "ACCOUNT_UPDATE",
		"agent":    "HUMAN_AGENT",
	}
)

// keys for extra in channel events
const (
	referrerIDKey = "referrer_id"
	sourceKey     = "source"
	adIDKey       = "ad_id"
	typeKey       = "type"
	titleKey      = "title"
	payloadKey    = "payload"
)

func newHandler(channelType courier.ChannelType, name string, validateSignatures bool) courier.ChannelHandler {
	return &handler{handlers.NewBaseHandlerWithParams(channelType, name, validateSignatures)}
}

func init() {
	courier.RegisterHandler(newHandler("IG", "Instagram", false))
	courier.RegisterHandler(newHandler("FBA", "Facebook", false))

}

type handler struct {
	handlers.BaseHandler
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodGet, "receive", h.receiveVerify)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveEvent)
	return nil
}

type Sender struct {
	ID      string `json:"id"`
	UserRef string `json:"user_ref,omitempty"`
}

type User struct {
	ID string `json:"id"`
}

// {
//   "object":"page",
//   "entry":[{
//     "id":"180005062406476",
//     "time":1514924367082,
//     "messaging":[{
//       "sender":  {"id":"1630934236957797"},
//       "recipient":{"id":"180005062406476"},
//       "timestamp":1514924366807,
//       "message":{
//         "mid":"mid.$cAAD5QiNHkz1m6cyj11guxokwkhi2",
//         "seq":33116,
//         "text":"65863634"
//       }
//     }]
//   }]
// }
type moPayload struct {
	Object string `json:"object"`
	Entry  []struct {
		ID        string `json:"id"`
		Time      int64  `json:"time"`
		Messaging []struct {
			Sender    Sender `json:"sender"`
			Recipient User   `json:"recipient"`
			Timestamp int64  `json:"timestamp"`

			OptIn *struct {
				Ref     string `json:"ref"`
				UserRef string `json:"user_ref"`
			} `json:"optin"`

			Referral *struct {
				Ref    string `json:"ref"`
				Source string `json:"source"`
				Type   string `json:"type"`
				AdID   string `json:"ad_id"`
			} `json:"referral"`

			Postback *struct {
				MID      string `json:"mid"`
				Title    string `json:"title"`
				Payload  string `json:"payload"`
				Referral struct {
					Ref    string `json:"ref"`
					Source string `json:"source"`
					Type   string `json:"type"`
					AdID   string `json:"ad_id"`
				} `json:"referral"`
			} `json:"postback"`

			Message *struct {
				IsEcho      bool   `json:"is_echo"`
				MID         string `json:"mid"`
				Text        string `json:"text"`
				Attachments []struct {
					Type    string `json:"type"`
					Payload *struct {
						URL         string `json:"url"`
						StickerID   int64  `json:"sticker_id"`
						Coordinates *struct {
							Lat  float64 `json:"lat"`
							Long float64 `json:"long"`
						} `json:"coordinates"`
					}
				} `json:"attachments"`
			} `json:"message"`

			Delivery *struct {
				MIDs      []string `json:"mids"`
				Watermark int64    `json:"watermark"`
			} `json:"delivery"`
		} `json:"messaging"`
	} `json:"entry"`
}

// GetChannel returns the channel
func (h *handler) GetChannel(ctx context.Context, r *http.Request) (courier.Channel, error) {
	if r.Method == http.MethodGet {
		return nil, nil
	}

	payload := &moPayload{}
	err := handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, err
	}

	// is not a 'page' and 'instagram' object? ignore it
	if payload.Object != "page" && payload.Object != "instagram" {
		return nil, fmt.Errorf("object expected 'page' or 'instagram', found %s", payload.Object)
	}

	// no entries? ignore this request
	if len(payload.Entry) == 0 {
		return nil, fmt.Errorf("no entries found")
	}

	EntryID := payload.Entry[0].ID

	//if object is 'page' returns type FBA, if object is 'instagram' returns type IG
	if payload.Object == "page" {
		return h.Backend().GetChannelByAddress(ctx, courier.ChannelType("FBA"), courier.ChannelAddress(EntryID))
	} else {
		return h.Backend().GetChannelByAddress(ctx, courier.ChannelType("IG"), courier.ChannelAddress(EntryID))
	}
}

// receiveVerify handles Facebook's webhook verification callback
func (h *handler) receiveVerify(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	mode := r.URL.Query().Get("hub.mode")

	// this isn't a subscribe verification, that's an error
	if mode != "subscribe" {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unknown request"))
	}

	// verify the token against our server facebook webhook secret, if the same return the challenge FB sent us
	secret := r.URL.Query().Get("hub.verify_token")
	if secret != h.Server().Config().FacebookWebhookSecret {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("token does not match secret"))
	}
	// and respond with the challenge token
	_, err := fmt.Fprint(w, r.URL.Query().Get("hub.challenge"))
	return nil, err
}

// receiveEvent is our HTTP handler function for incoming messages and status updates
func (h *handler) receiveEvent(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	err := h.validateSignature(r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	payload := &moPayload{}
	err = handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// // is not a 'page' and 'instagram' object? ignore it
	if payload.Object != "page" && payload.Object != "instagram" {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring request")
	}

	// no entries? ignore this request
	if len(payload.Entry) == 0 {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring request, no entries")
	}

	// the list of events we deal with
	events := make([]courier.Event, 0, 2)

	// the list of data we will return in our response
	data := make([]interface{}, 0, 2)

	// for each entry
	for _, entry := range payload.Entry {
		// no entry, ignore
		if len(entry.Messaging) == 0 {
			continue
		}

		// grab our message, there is always a single one
		msg := entry.Messaging[0]

		// ignore this entry if it is to another page
		if channel.Address() != msg.Recipient.ID {
			continue
		}

		// create our date from the timestamp (they give us millis, arg is nanos)
		date := time.Unix(0, msg.Timestamp*1000000).UTC()

		sender := msg.Sender.UserRef
		if sender == "" {
			sender = msg.Sender.ID
		}

		// create our URN
		urn, err := urns.NewFacebookURN(sender)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}
		if msg.OptIn != nil {
			// this is an opt in, if we have a user_ref, use that as our URN (this is a checkbox plugin)
			// TODO:
			//    We need to deal with the case of them responding and remapping the user_ref in that case:
			//    https://developers.facebook.com/docs/messenger-platform/discovery/checkbox-plugin
			//    Right now that we even support this isn't documented and I don't think anybody uses it, so leaving that out.
			//    (things will still work, we just will have dupe contacts, one with user_ref for the first contact, then with the real id when they reply)
			if msg.OptIn.UserRef != "" {
				urn, err = urns.NewFacebookURN(urns.FacebookRefPrefix + msg.OptIn.UserRef)
				if err != nil {
					return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
				}
			}

			event := h.Backend().NewChannelEvent(channel, courier.Referral, urn).WithOccurredOn(date)

			// build our extra
			extra := map[string]interface{}{
				referrerIDKey: msg.OptIn.Ref,
			}
			event = event.WithExtra(extra)

			err := h.Backend().WriteChannelEvent(ctx, event)
			if err != nil {
				return nil, err
			}

			events = append(events, event)
			data = append(data, courier.NewEventReceiveData(event))

		} else if msg.Postback != nil {
			// by default postbacks are treated as new conversations, unless we have referral information
			eventType := courier.NewConversation
			if msg.Postback.Referral.Ref != "" {
				eventType = courier.Referral
			}
			event := h.Backend().NewChannelEvent(channel, eventType, urn).WithOccurredOn(date)

			// build our extra
			extra := map[string]interface{}{
				titleKey:   msg.Postback.Title,
				payloadKey: msg.Postback.Payload,
			}

			// add in referral information if we have it
			if eventType == courier.Referral {
				extra[referrerIDKey] = msg.Postback.Referral.Ref
				extra[sourceKey] = msg.Postback.Referral.Source
				extra[typeKey] = msg.Postback.Referral.Type

				if msg.Postback.Referral.AdID != "" {
					extra[adIDKey] = msg.Postback.Referral.AdID
				}
			}

			event = event.WithExtra(extra)

			err := h.Backend().WriteChannelEvent(ctx, event)
			if err != nil {
				return nil, err
			}

			events = append(events, event)
			data = append(data, courier.NewEventReceiveData(event))

		} else if msg.Referral != nil {
			// this is an incoming referral
			event := h.Backend().NewChannelEvent(channel, courier.Referral, urn).WithOccurredOn(date)

			// build our extra
			extra := map[string]interface{}{
				sourceKey: msg.Referral.Source,
				typeKey:   msg.Referral.Type,
			}

			// add referrer id if present
			if msg.Referral.Ref != "" {
				extra[referrerIDKey] = msg.Referral.Ref
			}

			// add ad id if present
			if msg.Referral.AdID != "" {
				extra[adIDKey] = msg.Referral.AdID
			}
			event = event.WithExtra(extra)

			err := h.Backend().WriteChannelEvent(ctx, event)
			if err != nil {
				return nil, err
			}

			events = append(events, event)
			data = append(data, courier.NewEventReceiveData(event))

		} else if msg.Message != nil {
			// this is an incoming message

			// ignore echos
			if msg.Message.IsEcho {
				data = append(data, courier.NewInfoData("ignoring echo"))
				continue
			}

			text := msg.Message.Text

			attachmentURLs := make([]string, 0, 2)

			// if we have a sticker ID, use that as our text
			for _, att := range msg.Message.Attachments {
				if att.Type == "image" && att.Payload != nil && att.Payload.StickerID != 0 {
					text = stickerIDToEmoji[att.Payload.StickerID]
				}

				if att.Type == "location" {
					attachmentURLs = append(attachmentURLs, fmt.Sprintf("geo:%f,%f", att.Payload.Coordinates.Lat, att.Payload.Coordinates.Long))
				}

				if att.Payload != nil && att.Payload.URL != "" {
					attachmentURLs = append(attachmentURLs, att.Payload.URL)
				}

			}

			// create our message
			ev := h.Backend().NewIncomingMsg(channel, urn, text).WithExternalID(msg.Message.MID).WithReceivedOn(date)
			event := h.Backend().CheckExternalIDSeen(ev)

			// add any attachment URL found
			for _, attURL := range attachmentURLs {
				event.WithAttachment(attURL)
			}

			err := h.Backend().WriteMsg(ctx, event)
			if err != nil {
				return nil, err
			}

			h.Backend().WriteExternalIDSeen(event)

			events = append(events, event)
			data = append(data, courier.NewMsgReceiveData(event))

		} else if msg.Delivery != nil {
			// this is a delivery report
			for _, mid := range msg.Delivery.MIDs {
				event := h.Backend().NewMsgStatusForExternalID(channel, mid, courier.MsgDelivered)
				err := h.Backend().WriteMsgStatus(ctx, event)

				// we don't know about this message, just tell them we ignored it
				if err == courier.ErrMsgNotFound {
					data = append(data, courier.NewInfoData("message not found, ignored"))
					continue
				}

				if err != nil {
					return nil, err
				}

				events = append(events, event)
				data = append(data, courier.NewStatusData(event))
			}

		} else {
			data = append(data, courier.NewInfoData("ignoring unknown entry type"))
		}
	}

	return events, courier.WriteDataResponse(ctx, w, http.StatusOK, "Events Handled", data)
}

// {
//     "messaging_type": "<MESSAGING_TYPE>"
//     "recipient":{
//         "id":"<PSID>"
//     },
//     "message":{
//	       "text":"hello, world!"
//         "attachment":{
//             "type":"image",
//             "payload":{
//                 "url":"http://www.messenger-rocks.com/image.jpg",
//                 "is_reusable":true
//             }
//         }
//     }
// }
type mtPayload struct {
	MessagingType string `json:"messaging_type"`
	Tag           string `json:"tag,omitempty"`
	Recipient     struct {
		UserRef string `json:"user_ref,omitempty"`
		ID      string `json:"id,omitempty"`
	} `json:"recipient"`
	Message struct {
		Text         string         `json:"text,omitempty"`
		QuickReplies []mtQuickReply `json:"quick_replies,omitempty"`
		Attachment   *mtAttachment  `json:"attachment,omitempty"`
	} `json:"message"`
}

type mtAttachment struct {
	Type    string `json:"type"`
	Payload struct {
		URL        string `json:"url"`
		IsReusable bool   `json:"is_reusable"`
	} `json:"payload"`
}

type mtQuickReply struct {
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

	topic := msg.Topic()
	payload := mtPayload{}

	// set our message type
	if msg.ResponseToExternalID() != "" {
		payload.MessagingType = "RESPONSE"
	} else if topic != "" {
		payload.MessagingType = "MESSAGE_TAG"
		payload.Tag = tagByTopic[topic]
	} else {
		payload.MessagingType = "UPDATE"
	}

	// build our recipient
	if msg.URN().IsFacebookRef() {
		payload.Recipient.UserRef = msg.URN().FacebookRef()
	} else {
		payload.Recipient.ID = msg.URN().Path()
	}

	msgURL, _ := url.Parse(sendURL)
	query := url.Values{}
	query.Set("access_token", accessToken)
	msgURL.RawQuery = query.Encode()

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)

	msgParts := make([]string, 0)
	if msg.Text() != "" {
		msgParts = handlers.SplitMsgByChannel(msg.Channel(), msg.Text(), maxMsgLength)
	}

	// send each part and each attachment separately. we send attachments first as otherwise quick replies
	// attached to text messages get hidden when images get delivered
	for i := 0; i < len(msgParts)+len(msg.Attachments()); i++ {
		if i < len(msg.Attachments()) {
			// this is an attachment
			payload.Message.Attachment = &mtAttachment{}
			attType, attURL := handlers.SplitAttachment(msg.Attachments()[i])
			attType = strings.Split(attType, "/")[0]
			if attType == "application" {
				attType = "file"
			}
			payload.Message.Attachment.Type = attType
			payload.Message.Attachment.Payload.URL = attURL
			payload.Message.Attachment.Payload.IsReusable = true
			payload.Message.Text = ""
		} else {
			// this is still a msg part
			payload.Message.Text = msgParts[i-len(msg.Attachments())]
			payload.Message.Attachment = nil
		}

		// include any quick replies on the last piece we send
		if i == (len(msgParts)+len(msg.Attachments()))-1 {
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

		req, err := http.NewRequest(http.MethodPost, msgURL.String(), bytes.NewReader(jsonBody))
		if err != nil {
			return nil, err
		}
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
			if msg.URN().IsFacebookRef() {
				recipientID, err := jsonparser.GetString(rr.Body, "recipient_id")
				if err != nil {
					log.WithError("Message Send Error", errors.Errorf("unable to get recipient_id from body"))
					return status, nil
				}

				referralID := msg.URN().FacebookRef()

				realIDURN, err := urns.NewFacebookURN(recipientID)
				if err != nil {
					log.WithError("Message Send Error", errors.Errorf("unable to make facebook urn from %s", recipientID))
				}

				contact, err := h.Backend().GetContact(ctx, msg.Channel(), msg.URN(), "", "")
				if err != nil {
					log.WithError("Message Send Error", errors.Errorf("unable to get contact for %s", msg.URN().String()))
				}
				realURN, err := h.Backend().AddURNtoContact(ctx, msg.Channel(), contact, realIDURN)
				if err != nil {
					log.WithError("Message Send Error", errors.Errorf("unable to add real facebook URN %s to contact with uuid %s", realURN.String(), contact.UUID()))
				}
				referralIDExtURN, err := urns.NewURNFromParts(urns.ExternalScheme, referralID, "", "")
				if err != nil {
					log.WithError("Message Send Error", errors.Errorf("unable to make ext urn from %s", referralID))
				}
				extURN, err := h.Backend().AddURNtoContact(ctx, msg.Channel(), contact, referralIDExtURN)
				if err != nil {
					log.WithError("Message Send Error", errors.Errorf("unable to add URN %s to contact with uuid %s", extURN.String(), contact.UUID()))
				}

				referralFacebookURN, err := h.Backend().RemoveURNfromContact(ctx, msg.Channel(), contact, msg.URN())
				if err != nil {
					log.WithError("Message Send Error", errors.Errorf("unable to remove referral facebook URN %s from contact with uuid %s", referralFacebookURN.String(), contact.UUID()))
				}

			}

		}

		// this was wired successfully
		status.SetStatus(courier.MsgWired)
	}

	return status, nil
}

// DescribeURN looks up URN metadata for new contacts
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
	base, _ := url.Parse(graphURL)
	path, _ := url.Parse(fmt.Sprintf("/%s", urn.Path()))
	u := base.ResolveReference(path)

	query := url.Values{}
	query.Set("access_token", accessToken)
	u.RawQuery = query.Encode()
	req, _ := http.NewRequest(http.MethodGet, u.String(), nil)
	rr, err := utils.MakeHTTPRequest(req)
	if err != nil {
		return nil, fmt.Errorf("unable to look up contact data:%s\n%s", err, rr.Response)
	}

	// read our name
	name, _ := jsonparser.GetString(rr.Body, "name")

	return map[string]string{"name": name}, nil
}

// see https://developers.facebook.com/docs/messenger-platform/webhook#security
func (h *handler) validateSignature(r *http.Request) error {
	headerSignature := r.Header.Get(signatureHeader)
	if headerSignature == "" {
		return fmt.Errorf("missing request signature")
	}
	appSecret := h.Server().Config().FacebookApplicationSecret

	body, err := handlers.ReadBody(r, 100000)
	if err != nil {
		return fmt.Errorf("unable to read request body: %s", err)
	}

	expectedSignature, err := fbCalculateSignature(appSecret, body)
	if err != nil {
		return err
	}

	signature := ""
	if len(headerSignature) == 45 && strings.HasPrefix(headerSignature, "sha1=") {
		signature = strings.TrimPrefix(headerSignature, "sha1=")
	}

	// compare signatures in way that isn't sensitive to a timing attack
	if !hmac.Equal([]byte(expectedSignature), []byte(signature)) {
		return fmt.Errorf("invalid request signature, expected: %s got: %s for body: '%s'", expectedSignature, signature, string(body))
	}

	return nil
}

func fbCalculateSignature(appSecret string, body []byte) (string, error) {
	var buffer bytes.Buffer
	buffer.Write(body)

	// hash with SHA1
	mac := hmac.New(sha1.New, []byte(appSecret))
	mac.Write(buffer.Bytes())

	return hex.EncodeToString(mac.Sum(nil)), nil
}
