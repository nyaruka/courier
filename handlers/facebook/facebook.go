package facebook

import (
	"bytes"
	"context"
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
	"github.com/sirupsen/logrus"
)

// Endpoints we hit
var (
	sendURL      = "https://graph.facebook.com/v2.6/me/messages"
	subscribeURL = "https://graph.facebook.com/v2.6/me/subscribed_apps"
	graphURL     = "https://graph.facebook.com/v2.6/"

	// How long we want after the subscribe callback to register the page for events
	subscribeTimeout = time.Second * 2

	// Facebook API says 640 is max for the body
	maxMsgLength = 640
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

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("FB"), "Facebook")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	err := s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveEvent)
	if err != nil {
		return err
	}
	return s.AddHandlerRoute(h, http.MethodGet, "receive", h.receiveVerify)
}

// receiveVerify handles Facebook's webhook verification callback
func (h *handler) receiveVerify(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	mode := r.URL.Query().Get("hub.mode")

	// this isn't a subscribe verification, that's an error
	if mode != "subscribe" {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf("unknown request"))
	}

	// verify the token against our secret, if the same return the challenge FB sent us
	secret := r.URL.Query().Get("hub.verify_token")
	if secret != channel.StringConfigForKey(courier.ConfigSecret, "") {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf("token does not match secret"))
	}

	// make sure we have an auth token
	authToken := channel.StringConfigForKey(courier.ConfigAuthToken, "")
	if authToken == "" {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf("missing auth token for FB channel"))
	}

	// everything looks good, we will subscribe to this page's messages asynchronously
	go func() {
		// wait a bit for Facebook to handle this response
		time.Sleep(subscribeTimeout)

		// subscribe to messaging events for this page
		form := url.Values{}
		form.Set("access_token", authToken)
		req, _ := http.NewRequest(http.MethodPost, subscribeURL, strings.NewReader(form.Encode()))
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		rr, err := utils.MakeHTTPRequest(req)

		// log if we get any kind of error
		success, _ := jsonparser.GetBoolean([]byte(rr.Body), "success")
		if err != nil || !success {
			logrus.WithField("channel_uuid", channel.UUID()).WithField("response", rr.Response).Error("error subscribing to Facebook page events")
		}
	}()

	// and respond with the challenge token
	_, err := fmt.Fprint(w, r.URL.Query().Get("hub.challenge"))
	return nil, err
}

type fbUser struct {
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
			Sender    fbUser `json:"sender"`
			Recipient fbUser `json:"recipient"`
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
				Title    string `json:"title"`
				Payload  string `json:"payload"`
				Referral struct {
					Ref    string `json:"ref"`
					Source string `json:"source"`
					Type   string `json:"type"`
				} `json:"referral"`
			} `json:"postback"`

			Message *struct {
				IsEcho      bool   `json:"is_echo"`
				MID         string `json:"mid"`
				Text        string `json:"text"`
				Attachments []struct {
					Type    string `json:"type"`
					Payload *struct {
						URL string `json:"url"`
					}
				} `json:"attachments"`
			} `json:"message"`

			Delivery *struct {
				MIDs      []string `json:"mids"`
				Watermark int64    `json:"watermark"`
				Seq       int      `json:"seq"`
			} `json:"delivery"`
		} `json:"messaging"`
	} `json:"entry"`
}

// receiveEvent is our HTTP handler function for incoming messages and status updates
func (h *handler) receiveEvent(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	payload := &moPayload{}
	err := handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	// not a page object? ignore
	if payload.Object != "page" {
		return nil, courier.WriteAndLogRequestIgnored(ctx, w, r, channel, "ignoring non-page request")
	}

	// no entries? ignore this request
	if len(payload.Entry) == 0 {
		return nil, courier.WriteAndLogRequestIgnored(ctx, w, r, channel, "ignoring request, no entries")
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

		// create our URN
		urn, err := urns.NewURNFromParts(urns.FacebookScheme, msg.Sender.ID, "")
		if err != nil {
			return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
		}
		if msg.OptIn != nil {
			// this is an opt in, if we have a user_ref, use that as our URN (this is a checkbox plugin)
			// TODO:
			//    We need to deal with the case of them responding and remapping the user_ref in that case:
			//    https://developers.facebook.com/docs/messenger-platform/discovery/checkbox-plugin
			//    Right now that we even support this isn't documented and I don't think anybody uses it, so leaving that out.
			//    (things will still work, we just will have dupe contacts, one with user_ref for the first contact, then with the real id when they reply)
			if msg.OptIn.UserRef != "" {
				urn, err = urns.NewURNFromParts(urns.FacebookScheme, urns.FacebookRefPrefix+msg.OptIn.UserRef, "")
				if err != nil {
					return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
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
			// this is a postback
			eventType := courier.Referral
			if msg.Postback.Payload == "get_started" {
				eventType = courier.NewConversation
			}
			event := h.Backend().NewChannelEvent(channel, eventType, urn).WithOccurredOn(date)

			// build our extra
			extra := map[string]interface{}{
				titleKey:      msg.Postback.Title,
				payloadKey:    msg.Postback.Payload,
				referrerIDKey: msg.Postback.Referral.Ref,
				sourceKey:     msg.Postback.Referral.Source,
				typeKey:       msg.Postback.Referral.Type,
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

			// create our message
			event := h.Backend().NewIncomingMsg(channel, urn, msg.Message.Text).WithExternalID(msg.Message.MID).WithReceivedOn(date)

			// add any attachments
			for _, att := range msg.Message.Attachments {
				if att.Payload != nil && att.Payload.URL != "" {
					event.WithAttachment(att.Payload.URL)
				}
			}

			err := h.Backend().WriteMsg(ctx, event)
			if err != nil {
				return nil, err
			}

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

	payload := mtPayload{}

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

	msgURL, _ := url.Parse(sendURL)
	query := url.Values{}
	query.Set("access_token", accessToken)
	msgURL.RawQuery = query.Encode()

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
			attType, attURL := handlers.SplitAttachment(msg.Attachments()[i-len(msgParts)])
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

		req, _ := http.NewRequest(http.MethodPost, msgURL.String(), bytes.NewReader(jsonBody))
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
	base, _ := url.Parse(graphURL)
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
