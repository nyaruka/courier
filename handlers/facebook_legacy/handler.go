package facebook_legacy

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/pkg/errors"
)

// Endpoints we hit
var (
	sendURL      = "https://graph.facebook.com/v3.3/me/messages"
	subscribeURL = "https://graph.facebook.com/v3.3/me/subscribed_apps"
	graphURL     = "https://graph.facebook.com/v3.3/"

	// How long we want after the subscribe callback to register the page for events
	subscribeTimeout = time.Second * 2

	// Facebook API says 640 is max for the body
	maxMsgLength = 640

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
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMultiReceive, handlers.JSONPayload(h, h.receiveEvents))
	s.AddHandlerRoute(h, http.MethodGet, "receive", courier.ChannelLogTypeWebhookVerify, h.receiveVerify)
	return nil
}

// receiveVerify handles Facebook's webhook verification callback
func (h *handler) receiveVerify(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	mode := r.URL.Query().Get("hub.mode")

	// this isn't a subscribe verification, that's an error
	if mode != "subscribe" {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unknown request"))
	}

	// verify the token against our secret, if the same return the challenge FB sent us
	secret := r.URL.Query().Get("hub.verify_token")
	if secret != channel.StringConfigForKey(courier.ConfigSecret, "") {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("token does not match secret"))
	}

	// make sure we have an auth token
	authToken := channel.StringConfigForKey(courier.ConfigAuthToken, "")
	if authToken == "" {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("missing auth token for FB channel"))
	}

	// everything looks good, we will subscribe to this page's messages asynchronously
	go func() {
		// wait a bit for Facebook to handle this response
		time.Sleep(subscribeTimeout)

		h.subscribeToEvents(ctx, channel, authToken)
	}()

	// and respond with the challenge token
	_, err := fmt.Fprint(w, r.URL.Query().Get("hub.challenge"))
	return nil, err
}

func (h *handler) subscribeToEvents(ctx context.Context, channel courier.Channel, authToken string) {
	clog := courier.NewChannelLog(courier.ChannelLogTypePageSubscribe, channel, h.RedactValues(channel))

	// subscribe to messaging events for this page
	form := url.Values{}
	form.Set("access_token", authToken)
	req, _ := http.NewRequest(http.MethodPost, subscribeURL, strings.NewReader(form.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, respBody, err := h.RequestHTTP(req, clog)

	// log if we get any kind of error
	success, _ := jsonparser.GetBoolean(respBody, "success")
	if err != nil || resp.StatusCode/100 != 2 || !success {
		slog.Error("error subscribing to Facebook page events", "channel_uuid", channel.UUID())
	}

	h.Backend().WriteChannelLog(ctx, clog)
}

type fbUser struct {
	ID string `json:"id"`
}

//	{
//	  "object":"page",
//	  "entry":[{
//	    "id":"180005062406476",
//	    "time":1514924367082,
//	    "messaging":[
//	      {
//	        "sender": {"id":"1630934236957797"},
//	        "recipient":{"id":"180005062406476"},
//	        "timestamp":1514924366807,
//	        "message":{
//	          "mid":"mid.$cAAD5QiNHkz1m6cyj11guxokwkhi2",
//	          "seq":33116,
//	          "text":"65863634"
//	        }
//	      }
//	    ]
//	  }]
//	}
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
					AdID   string `json:"ad_id"`
				} `json:"referral"`
			} `json:"postback"`

			Message *struct {
				IsEcho      bool   `json:"is_echo"`
				MID         string `json:"mid"`
				Text        string `json:"text"`
				StickerID   int64  `json:"sticker_id"`
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
				Seq       int      `json:"seq"`
			} `json:"delivery"`
		} `json:"messaging"`
	} `json:"entry"`
}

// receiveEvents is our HTTP handler function for incoming messages and status updates
func (h *handler) receiveEvents(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *moPayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	// not a page object? ignore
	if payload.Object != "page" {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring non-page request")
	}

	// no entries? ignore this request
	if len(payload.Entry) == 0 {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring request, no entries")
	}

	// the list of events we deal with
	events := make([]courier.Event, 0, 2)

	// the list of data we will return in our response
	data := make([]any, 0, 2)

	seenMsgIDs := make(map[string]bool, 2)

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
		urn, err := urns.NewFacebookURN(msg.Sender.ID)
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

			event := h.Backend().NewChannelEvent(channel, courier.EventTypeReferral, urn, clog).WithOccurredOn(date)

			// build our extra
			extra := map[string]string{referrerIDKey: msg.OptIn.Ref}
			event = event.WithExtra(extra)

			err := h.Backend().WriteChannelEvent(ctx, event, clog)
			if err != nil {
				return nil, err
			}

			events = append(events, event)
			data = append(data, courier.NewEventReceiveData(event))

		} else if msg.Postback != nil {
			// by default postbacks are treated as new conversations, unless we have referral information
			eventType := courier.EventTypeNewConversation
			if msg.Postback.Referral.Ref != "" {
				eventType = courier.EventTypeReferral
			}
			event := h.Backend().NewChannelEvent(channel, eventType, urn, clog).WithOccurredOn(date)

			// build our extra
			extra := map[string]string{
				titleKey:   msg.Postback.Title,
				payloadKey: msg.Postback.Payload,
			}

			// add in referral information if we have it
			if eventType == courier.EventTypeReferral {
				extra[referrerIDKey] = msg.Postback.Referral.Ref
				extra[sourceKey] = msg.Postback.Referral.Source
				extra[typeKey] = msg.Postback.Referral.Type

				if msg.Postback.Referral.AdID != "" {
					extra[adIDKey] = msg.Postback.Referral.AdID
				}
			}

			event = event.WithExtra(extra)

			err := h.Backend().WriteChannelEvent(ctx, event, clog)
			if err != nil {
				return nil, err
			}

			events = append(events, event)
			data = append(data, courier.NewEventReceiveData(event))

		} else if msg.Referral != nil {
			// this is an incoming referral
			event := h.Backend().NewChannelEvent(channel, courier.EventTypeReferral, urn, clog).WithOccurredOn(date)

			// build our extra
			extra := map[string]string{sourceKey: msg.Referral.Source, typeKey: msg.Referral.Type}

			// add referrer id if present
			if msg.Referral.Ref != "" {
				extra[referrerIDKey] = msg.Referral.Ref
			}

			// add ad id if present
			if msg.Referral.AdID != "" {
				extra[adIDKey] = msg.Referral.AdID
			}
			event = event.WithExtra(extra)

			err := h.Backend().WriteChannelEvent(ctx, event, clog)
			if err != nil {
				return nil, err
			}

			events = append(events, event)
			data = append(data, courier.NewEventReceiveData(event))

		} else if msg.Message != nil {
			// this is an incoming message
			if seenMsgIDs[msg.Message.MID] {
				continue
			}

			// ignore echos
			if msg.Message.IsEcho {
				data = append(data, courier.NewInfoData("ignoring echo"))
				continue
			}

			text := msg.Message.Text

			attachmentURLs := make([]string, 0, 2)

			// loop on our attachments
			for _, att := range msg.Message.Attachments {
				// if we have a sticker ID, use that as our text
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

			// if we have a sticker ID, use that as our text
			stickerText := stickerIDToEmoji[msg.Message.StickerID]
			if stickerText != "" {
				text = stickerText
			}

			// create our message
			event := h.Backend().NewIncomingMsg(channel, urn, text, msg.Message.MID, clog).WithReceivedOn(date)

			// add any attachment URL found
			for _, attURL := range attachmentURLs {
				event.WithAttachment(attURL)
			}

			err := h.Backend().WriteMsg(ctx, event, clog)
			if err != nil {
				return nil, err
			}

			events = append(events, event)
			data = append(data, courier.NewMsgReceiveData(event))
			seenMsgIDs[msg.Message.MID] = true

		} else if msg.Delivery != nil {
			// this is a delivery report
			for _, mid := range msg.Delivery.MIDs {
				event := h.Backend().NewStatusUpdateByExternalID(channel, mid, courier.MsgStatusDelivered, clog)
				err := h.Backend().WriteStatusUpdate(ctx, event)
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

	return events, courier.WriteDataResponse(w, http.StatusOK, "Events Handled", data)
}

//	{
//	  "messaging_type": "<MESSAGING_TYPE>"
//	  "recipient": {
//	    "id":"<PSID>"
//	  },
//	  "message": {
//	    "text":"hello, world!"
//	    "attachment":{
//	      "type":"image",
//	      "payload":{
//	        "url":"http://www.messenger-rocks.com/image.jpg",
//	        "is_reusable":true
//	      }
//	    }
//	  }
//	}
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

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
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
		payload.MessagingType = "NON_PROMOTIONAL_SUBSCRIPTION" // only allowed until Jan 15, 2020
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

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)

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

		jsonBody := jsonx.MustMarshal(payload)

		req, err := http.NewRequest(http.MethodPost, msgURL.String(), bytes.NewReader(jsonBody))
		if err != nil {
			return nil, err
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 != 2 {
			return status, nil
		}

		externalID, err := jsonparser.GetString(respBody, "message_id")
		if err != nil {
			clog.Error(courier.ErrorResponseValueMissing("message_id"))
			return status, nil
		}

		// if this is our first message, record the external id
		if i == 0 {
			status.SetExternalID(externalID)
			if msg.URN().IsFacebookRef() {
				recipientID, err := jsonparser.GetString(respBody, "recipient_id")
				if err != nil {
					clog.Error(courier.ErrorResponseValueMissing("recipient_id"))
					return status, nil
				}

				referralID := msg.URN().FacebookRef()

				realIDURN, err := urns.NewFacebookURN(recipientID)
				if err != nil {
					clog.RawError(errors.Errorf("unable to make facebook urn from %s", recipientID))
				}

				contact, err := h.Backend().GetContact(ctx, msg.Channel(), msg.URN(), nil, "", clog)
				if err != nil {
					clog.RawError(errors.Errorf("unable to get contact for %s", msg.URN().String()))
				}
				realURN, err := h.Backend().AddURNtoContact(ctx, msg.Channel(), contact, realIDURN, nil)
				if err != nil {
					clog.RawError(errors.Errorf("unable to add real facebook URN %s to contact with uuid %s", realURN.String(), contact.UUID()))
				}
				referralIDExtURN, err := urns.NewURNFromParts(urns.ExternalScheme, referralID, "", "")
				if err != nil {
					clog.RawError(errors.Errorf("unable to make ext urn from %s", referralID))
				}
				extURN, err := h.Backend().AddURNtoContact(ctx, msg.Channel(), contact, referralIDExtURN, nil)
				if err != nil {
					clog.RawError(errors.Errorf("unable to add URN %s to contact with uuid %s", extURN.String(), contact.UUID()))
				}

				referralFacebookURN, err := h.Backend().RemoveURNfromContact(ctx, msg.Channel(), contact, msg.URN())
				if err != nil {
					clog.RawError(errors.Errorf("unable to remove referral facebook URN %s from contact with uuid %s", referralFacebookURN.String(), contact.UUID()))
				}
			}
		}

		// this was wired successfully
		status.SetStatus(courier.MsgStatusWired)
	}

	return status, nil
}

// DescribeURN looks up URN metadata for new contacts
func (h *handler) DescribeURN(ctx context.Context, channel courier.Channel, urn urns.URN, clog *courier.ChannelLog) (map[string]string, error) {
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

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return nil, errors.New("unable to look up contact data")
	}

	// read our first and last name
	firstName, _ := jsonparser.GetString(respBody, "first_name")
	lastName, _ := jsonparser.GetString(respBody, "last_name")

	return map[string]string{"name": utils.JoinNonEmpty(" ", firstName, lastName)}, nil
}
