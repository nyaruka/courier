package facebook

import (
	"context"
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
	"github.com/sirupsen/logrus"
)

var facebookMessageURL = "https://graph.facebook.com/v2.6/me/messages"
var facebookSubscribeURL = "https://graph.facebook.com/v2.6/me/subscribed_apps"
var facebookGraphURL = "https://graph.facebook.com/v2.6/"
var facebookSubscribeTimeout = time.Second * 5

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
	courier.RegisterHandler(NewHandler())
}

type handler struct {
	handlers.BaseHandler
}

const (
	maxLength = 640 // Facebook API says 640 is max for the body
)

// NewHandler returns a new FacebookHandler ready to be registered
func NewHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("FB"), "Facebook")}
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

// ReceiveVerify handles Facebook's webhook verification callback
func (h *handler) ReceiveVerify(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	mode := r.URL.Query().Get("hub.mode")

	// this isn't a subscribe verification, that's an error
	if mode != "subscribe" {
		return nil, courier.WriteError(ctx, w, r, fmt.Errorf("unknown request"))
	}

	// verify the token against our secret, if the same return the challenge FB sent us
	secret := r.URL.Query().Get("hub.verify_token")
	if secret != channel.StringConfigForKey(courier.ConfigSecret, "") {
		return nil, courier.WriteError(ctx, w, r, fmt.Errorf("token does not match secret"))
	}

	// make sure we have an auth token
	authToken := channel.StringConfigForKey(courier.ConfigAuthToken, "")
	if authToken == "" {
		return nil, courier.WriteError(ctx, w, r, fmt.Errorf("missing auth token for FB channel"))
	}

	// everything looks good, we will subscribe to this page's messages asynchronously
	go func() {
		// wait a bit for Facebook to handle this response
		time.Sleep(facebookSubscribeTimeout)

		// subscribe to messaging events for this page
		form := url.Values{}
		form.Set("access_token", authToken)
		req, _ := http.NewRequest(http.MethodPost, facebookSubscribeURL, strings.NewReader(form.Encode()))
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
type moEnvelope struct {
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

// Receive is our HTTP handler function for incoming messages and status updates
func (h *handler) Receive(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	mo := &moEnvelope{}
	err := handlers.DecodeAndValidateJSON(mo, r)
	if err != nil {
		return nil, courier.WriteError(ctx, w, r, err)
	}

	// not a page object? ignore
	if mo.Object != "page" {
		return nil, courier.WriteRequestIgnored(ctx, w, r, channel, "ignoring non-page request")
	}

	// no entries? ignore this request
	if len(mo.Entry) == 0 {
		return nil, courier.WriteRequestIgnored(ctx, w, r, channel, "ignoring request, no entries")
	}

	// the list of events we deal with
	events := make([]courier.Event, 0, 2)

	// the list of data we will return in our response
	data := make([]interface{}, 0, 2)

	// for each entry
	for _, entry := range mo.Entry {
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
		urn := urns.NewURNFromParts(urns.FacebookScheme, msg.Sender.ID, "")

		if msg.OptIn != nil {
			// this is an opt in, if we have a user_ref, use that as our URN
			if msg.OptIn.UserRef != "" {
				urn = urns.NewURNFromParts(urns.FacebookScheme, urns.FacebookRefPrefix+msg.OptIn.UserRef, "")
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

func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	return nil, fmt.Errorf("FB sending via Courier not yet implemented")
}

// ReceiveVerify handles Facebook's webhook verification callback
func (h *handler) DescribeURN(ctx context.Context, channel courier.Channel, urn urns.URN) (map[string]string, error) {
	// can't do anything with facebook refs, ignore them
	if urn.IsFacebookRef() {
		return nil, nil
	}

	accessToken := channel.StringConfigForKey(courier.ConfigAuthToken, "")
	if accessToken == "" {
		return nil, fmt.Errorf("missing access token")
	}

	// build a request to lookup the stats for this contact
	u, _ := url.Parse(fmt.Sprintf("%s%s", facebookGraphURL, urn.Path()))
	u.Query().Set("fields", "first_name,last_name")
	u.Query().Set("access_token", accessToken)
	req, _ := http.NewRequest(http.MethodGet, u.String(), nil)
	rr, err := utils.MakeHTTPRequest(req)
	if err != nil {
		return nil, err
	}

	// read our first and last name
	firstName, _ := jsonparser.GetString(rr.Body, "first_name")
	lastName, _ := jsonparser.GetString(rr.Body, "last_name")

	return map[string]string{"name": utils.JoinNonEmpty(firstName, lastName)}, nil
}
