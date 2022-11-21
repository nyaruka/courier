package facebookapp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
var (
	sendURL  = "https://graph.facebook.com/v12.0/me/messages"
	graphURL = "https://graph.facebook.com/v12.0/"

	signatureHeader = "X-Hub-Signature-256"

	maxRequestBodyBytes int64 = 1024 * 1024

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

var waStatusMapping = map[string]courier.MsgStatusValue{
	"sent":      courier.MsgSent,
	"delivered": courier.MsgDelivered,
	"read":      courier.MsgDelivered,
	"failed":    courier.MsgFailed,
}

var waIgnoreStatuses = map[string]bool{
	"deleted": true,
}

func newHandler(channelType courier.ChannelType, name string, useUUIDRoutes bool) courier.ChannelHandler {
	return &handler{handlers.NewBaseHandlerWithParams(channelType, name, useUUIDRoutes, []string{courier.ConfigAuthToken})}
}

func init() {
	courier.RegisterHandler(newHandler("IG", "Instagram", false))
	courier.RegisterHandler(newHandler("FBA", "Facebook", false))
	courier.RegisterHandler(newHandler("WAC", "WhatsApp Cloud", false))

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

type wacMedia struct {
	Caption  string `json:"caption"`
	Filename string `json:"filename"`
	ID       string `json:"id"`
	Mimetype string `json:"mime_type"`
	SHA256   string `json:"sha256"`
}
type moPayload struct {
	Object string `json:"object"`
	Entry  []struct {
		ID      string `json:"id"`
		Time    int64  `json:"time"`
		Changes []struct {
			Field string `json:"field"`
			Value struct {
				MessagingProduct string `json:"messaging_product"`
				Metadata         *struct {
					DisplayPhoneNumber string `json:"display_phone_number"`
					PhoneNumberID      string `json:"phone_number_id"`
				} `json:"metadata"`
				Contacts []struct {
					Profile struct {
						Name string `json:"name"`
					} `json:"profile"`
					WaID string `json:"wa_id"`
				} `json:"contacts"`
				Messages []struct {
					ID        string `json:"id"`
					From      string `json:"from"`
					Timestamp string `json:"timestamp"`
					Type      string `json:"type"`
					Context   *struct {
						Forwarded           bool   `json:"forwarded"`
						FrequentlyForwarded bool   `json:"frequently_forwarded"`
						From                string `json:"from"`
						ID                  string `json:"id"`
					} `json:"context"`
					Text struct {
						Body string `json:"body"`
					} `json:"text"`
					Image    *wacMedia `json:"image"`
					Audio    *wacMedia `json:"audio"`
					Video    *wacMedia `json:"video"`
					Document *wacMedia `json:"document"`
					Voice    *wacMedia `json:"voice"`
					Location *struct {
						Latitude  float64 `json:"latitude"`
						Longitude float64 `json:"longitude"`
						Name      string  `json:"name"`
						Address   string  `json:"address"`
					} `json:"location"`
					Button *struct {
						Text    string `json:"text"`
						Payload string `json:"payload"`
					} `json:"button"`
					Interactive struct {
						Type        string `json:"type"`
						ButtonReply struct {
							ID    string `json:"id"`
							Title string `json:"title"`
						} `json:"button_reply,omitempty"`
						ListReply struct {
							ID    string `json:"id"`
							Title string `json:"title"`
						} `json:"list_reply,omitempty"`
					} `json:"interactive,omitempty"`
					Errors []struct {
						Code  int    `json:"code"`
						Title string `json:"title"`
					} `json:"errors"`
				} `json:"messages"`
				Statuses []struct {
					ID           string `json:"id"`
					RecipientID  string `json:"recipient_id"`
					Status       string `json:"status"`
					Timestamp    string `json:"timestamp"`
					Type         string `json:"type"`
					Conversation *struct {
						ID     string `json:"id"`
						Origin *struct {
							Type string `json:"type"`
						} `json:"origin"`
					} `json:"conversation"`
					Pricing *struct {
						PricingModel string `json:"pricing_model"`
						Billable     bool   `json:"billable"`
						Category     string `json:"category"`
					} `json:"pricing"`
					Errors []struct {
						Code  int    `json:"code"`
						Title string `json:"title"`
					} `json:"errors"`
				} `json:"statuses"`
				Errors []struct {
					Code  int    `json:"code"`
					Title string `json:"title"`
				} `json:"errors"`
			} `json:"value"`
		} `json:"changes"`
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
				IsDeleted   bool   `json:"is_deleted"`
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

func (h *handler) RedactValues(ch courier.Channel) []string {
	vals := h.BaseHandler.RedactValues(ch)
	vals = append(vals, h.Server().Config().FacebookApplicationSecret, h.Server().Config().FacebookWebhookSecret, h.Server().Config().WhatsappAdminSystemUserToken)
	return vals
}

// WriteRequestError writes the passed in error to our response writer
func (h *handler) WriteRequestError(ctx context.Context, w http.ResponseWriter, err error) error {
	return courier.WriteError(w, http.StatusOK, err)
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
	if payload.Object != "page" && payload.Object != "instagram" && payload.Object != "whatsapp_business_account" {
		return nil, fmt.Errorf("object expected 'page', 'instagram' or 'whatsapp_business_account', found %s", payload.Object)
	}

	// no entries? ignore this request
	if len(payload.Entry) == 0 {
		return nil, fmt.Errorf("no entries found")
	}

	var channelAddress string

	//if object is 'page' returns type FBA, if object is 'instagram' returns type IG
	if payload.Object == "page" {
		channelAddress = payload.Entry[0].ID
		return h.Backend().GetChannelByAddress(ctx, courier.ChannelType("FBA"), courier.ChannelAddress(channelAddress))
	} else if payload.Object == "instagram" {
		channelAddress = payload.Entry[0].ID
		return h.Backend().GetChannelByAddress(ctx, courier.ChannelType("IG"), courier.ChannelAddress(channelAddress))
	} else {
		if len(payload.Entry[0].Changes) == 0 {
			return nil, fmt.Errorf("no changes found")
		}

		channelAddress = payload.Entry[0].Changes[0].Value.Metadata.PhoneNumberID
		if channelAddress == "" {
			return nil, fmt.Errorf("no channel address found")
		}
		return h.Backend().GetChannelByAddress(ctx, courier.ChannelType("WAC"), courier.ChannelAddress(channelAddress))
	}
}

// receiveVerify handles Facebook's webhook verification callback
func (h *handler) receiveVerify(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
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

func resolveMediaURL(mediaID string, token string, clog *courier.ChannelLog) (string, error) {
	if token == "" {
		return "", fmt.Errorf("missing token for WA channel")
	}

	base, _ := url.Parse(graphURL)
	path, _ := url.Parse(fmt.Sprintf("/%s", mediaID))
	retrieveURL := base.ResolveReference(path)

	// set the access token as the authorization header
	req, _ := http.NewRequest(http.MethodGet, retrieveURL.String(), nil)
	//req.Header.Set("User-Agent", utils.HTTPUserAgent)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, respBody, err := handlers.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return "", errors.New("error resolving media URL")
	}

	mediaURL, err := jsonparser.GetString(respBody, "url")
	return mediaURL, err
}

// receiveEvent is our HTTP handler function for incoming messages and status updates
func (h *handler) receiveEvent(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	err := h.validateSignature(r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	payload := &moPayload{}
	err = handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// is not a 'page' and 'instagram' object? ignore it
	if payload.Object != "page" && payload.Object != "instagram" && payload.Object != "whatsapp_business_account" {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring request")
	}

	// no entries? ignore this request
	if len(payload.Entry) == 0 {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring request, no entries")
	}

	var events []courier.Event
	var data []interface{}

	if channel.ChannelType() == "FBA" || channel.ChannelType() == "IG" {
		events, data, err = h.processFacebookInstagramPayload(ctx, channel, payload, w, r, clog)
	} else {
		events, data, err = h.processCloudWhatsAppPayload(ctx, channel, payload, w, r, clog)

	}

	if err != nil {
		return nil, err
	}

	return events, courier.WriteDataResponse(w, http.StatusOK, "Events Handled", data)
}

func (h *handler) processCloudWhatsAppPayload(ctx context.Context, channel courier.Channel, payload *moPayload, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, []interface{}, error) {
	// the list of events we deal with
	events := make([]courier.Event, 0, 2)

	// the list of data we will return in our response
	data := make([]interface{}, 0, 2)

	token := h.Server().Config().WhatsappAdminSystemUserToken

	var contactNames = make(map[string]string)

	// for each entry
	for _, entry := range payload.Entry {
		if len(entry.Changes) == 0 {
			continue
		}

		for _, change := range entry.Changes {

			for _, contact := range change.Value.Contacts {
				contactNames[contact.WaID] = contact.Profile.Name
			}

			for _, msg := range change.Value.Messages {
				// create our date from the timestamp
				ts, err := strconv.ParseInt(msg.Timestamp, 10, 64)
				if err != nil {
					return nil, nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("invalid timestamp: %s", msg.Timestamp))
				}
				date := time.Unix(ts, 0).UTC()

				urn, err := urns.NewWhatsAppURN(msg.From)
				if err != nil {
					return nil, nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
				}

				for _, msgError := range msg.Errors {
					clog.Error(courier.ErrorExternal(strconv.Itoa(msgError.Code), msgError.Title))
				}

				text := ""
				mediaURL := ""

				if msg.Type == "text" {
					text = msg.Text.Body
				} else if msg.Type == "audio" && msg.Audio != nil {
					text = msg.Audio.Caption
					mediaURL, err = resolveMediaURL(msg.Audio.ID, token, clog)
				} else if msg.Type == "voice" && msg.Voice != nil {
					text = msg.Voice.Caption
					mediaURL, err = resolveMediaURL(msg.Voice.ID, token, clog)
				} else if msg.Type == "button" && msg.Button != nil {
					text = msg.Button.Text
				} else if msg.Type == "document" && msg.Document != nil {
					text = msg.Document.Caption
					mediaURL, err = resolveMediaURL(msg.Document.ID, token, clog)
				} else if msg.Type == "image" && msg.Image != nil {
					text = msg.Image.Caption
					mediaURL, err = resolveMediaURL(msg.Image.ID, token, clog)
				} else if msg.Type == "video" && msg.Video != nil {
					text = msg.Video.Caption
					mediaURL, err = resolveMediaURL(msg.Video.ID, token, clog)
				} else if msg.Type == "location" && msg.Location != nil {
					mediaURL = fmt.Sprintf("geo:%f,%f", msg.Location.Latitude, msg.Location.Longitude)
				} else if msg.Type == "interactive" && msg.Interactive.Type == "button_reply" {
					text = msg.Interactive.ButtonReply.Title
				} else if msg.Type == "interactive" && msg.Interactive.Type == "list_reply" {
					text = msg.Interactive.ListReply.Title
				} else {
					// we received a message type we do not support.
					courier.LogRequestError(r, channel, fmt.Errorf("unsupported message type %s", msg.Type))
					continue
				}

				// create our message
				ev := h.Backend().NewIncomingMsg(channel, urn, text, clog).WithReceivedOn(date).WithExternalID(msg.ID).WithContactName(contactNames[msg.From])
				event := h.Backend().CheckExternalIDSeen(ev)

				// we had an error downloading media
				if err != nil {
					courier.LogRequestError(r, channel, err)
				}

				if mediaURL != "" {
					event.WithAttachment(mediaURL)
				}

				err = h.Backend().WriteMsg(ctx, event, clog)
				if err != nil {
					return nil, nil, err
				}

				h.Backend().WriteExternalIDSeen(event)

				events = append(events, event)
				data = append(data, courier.NewMsgReceiveData(event))

			}

			for _, status := range change.Value.Statuses {

				msgStatus, found := waStatusMapping[status.Status]
				if !found {
					if waIgnoreStatuses[status.Status] {
						data = append(data, courier.NewInfoData(fmt.Sprintf("ignoring status: %s", status.Status)))
					} else {
						handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unknown status: %s", status.Status))
					}
					continue
				}

				for _, statusError := range status.Errors {
					clog.Error(courier.ErrorExternal(strconv.Itoa(statusError.Code), statusError.Title))
				}

				event := h.Backend().NewMsgStatusForExternalID(channel, status.ID, msgStatus, clog)
				err := h.Backend().WriteMsgStatus(ctx, event)

				// we don't know about this message, just tell them we ignored it
				if err == courier.ErrMsgNotFound {
					data = append(data, courier.NewInfoData(fmt.Sprintf("message id: %s not found, ignored", status.ID)))
					continue
				}

				if err != nil {
					return nil, nil, err
				}

				events = append(events, event)
				data = append(data, courier.NewStatusData(event))

			}

			for _, chError := range change.Value.Errors {
				clog.Error(courier.ErrorExternal(strconv.Itoa(chError.Code), chError.Title))
			}

		}

	}
	return events, data, nil
}

func (h *handler) processFacebookInstagramPayload(ctx context.Context, channel courier.Channel, payload *moPayload, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, []interface{}, error) {
	var err error

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

		var urn urns.URN

		// create our URN
		if payload.Object == "instagram" {
			urn, err = urns.NewInstagramURN(sender)
			if err != nil {
				return nil, nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
			}
		} else {
			urn, err = urns.NewFacebookURN(sender)
			if err != nil {
				return nil, nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
			}
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
					return nil, nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
				}
			}

			event := h.Backend().NewChannelEvent(channel, courier.Referral, urn, clog).WithOccurredOn(date)

			// build our extra
			extra := map[string]interface{}{
				referrerIDKey: msg.OptIn.Ref,
			}
			event = event.WithExtra(extra)

			err := h.Backend().WriteChannelEvent(ctx, event, clog)
			if err != nil {
				return nil, nil, err
			}

			events = append(events, event)
			data = append(data, courier.NewEventReceiveData(event))

		} else if msg.Postback != nil {
			// by default postbacks are treated as new conversations, unless we have referral information
			eventType := courier.NewConversation
			if msg.Postback.Referral.Ref != "" {
				eventType = courier.Referral
			}
			event := h.Backend().NewChannelEvent(channel, eventType, urn, clog).WithOccurredOn(date)

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

			err := h.Backend().WriteChannelEvent(ctx, event, clog)
			if err != nil {
				return nil, nil, err
			}

			events = append(events, event)
			data = append(data, courier.NewEventReceiveData(event))

		} else if msg.Referral != nil {
			// this is an incoming referral
			event := h.Backend().NewChannelEvent(channel, courier.Referral, urn, clog).WithOccurredOn(date)

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

			err := h.Backend().WriteChannelEvent(ctx, event, clog)
			if err != nil {
				return nil, nil, err
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

			if msg.Message.IsDeleted {
				h.Backend().DeleteMsgWithExternalID(ctx, channel, msg.Message.MID)
				data = append(data, courier.NewInfoData("msg deleted"))
				continue
			}

			has_story_mentions := false

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

				if att.Type == "story_mention" {
					data = append(data, courier.NewInfoData("ignoring story_mention"))
					has_story_mentions = true
					continue
				}

				if att.Payload != nil && att.Payload.URL != "" {
					attachmentURLs = append(attachmentURLs, att.Payload.URL)
				}

			}

			// if we have a story mention, skip and do not save any message
			if has_story_mentions {
				continue
			}

			// create our message
			ev := h.Backend().NewIncomingMsg(channel, urn, text, clog).WithExternalID(msg.Message.MID).WithReceivedOn(date)
			event := h.Backend().CheckExternalIDSeen(ev)

			// add any attachment URL found
			for _, attURL := range attachmentURLs {
				event.WithAttachment(attURL)
			}

			err := h.Backend().WriteMsg(ctx, event, clog)
			if err != nil {
				return nil, nil, err
			}

			h.Backend().WriteExternalIDSeen(event)

			events = append(events, event)
			data = append(data, courier.NewMsgReceiveData(event))

		} else if msg.Delivery != nil {
			// this is a delivery report
			for _, mid := range msg.Delivery.MIDs {
				event := h.Backend().NewMsgStatusForExternalID(channel, mid, courier.MsgDelivered, clog)
				err := h.Backend().WriteMsgStatus(ctx, event)

				// we don't know about this message, just tell them we ignored it
				if err == courier.ErrMsgNotFound {
					data = append(data, courier.NewInfoData("message not found, ignored"))
					continue
				}

				if err != nil {
					return nil, nil, err
				}

				events = append(events, event)
				data = append(data, courier.NewStatusData(event))
			}

		} else {
			data = append(data, courier.NewInfoData("ignoring unknown entry type"))
		}
	}

	return events, data, nil
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

func (h *handler) Send(ctx context.Context, msg courier.Msg, clog *courier.ChannelLog) (courier.MsgStatus, error) {
	if msg.Channel().ChannelType() == "FBA" || msg.Channel().ChannelType() == "IG" {
		return h.sendFacebookInstagramMsg(ctx, msg, clog)
	} else if msg.Channel().ChannelType() == "WAC" {
		return h.sendCloudAPIWhatsappMsg(ctx, msg, clog)
	}

	return nil, fmt.Errorf("unssuported channel type")
}

func (h *handler) sendFacebookInstagramMsg(ctx context.Context, msg courier.Msg, clog *courier.ChannelLog) (courier.MsgStatus, error) {
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

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored, clog)

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

		resp, respBody, err := handlers.RequestHTTP(req, clog)
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

				contact, err := h.Backend().GetContact(ctx, msg.Channel(), msg.URN(), "", "", clog)
				if err != nil {
					clog.RawError(errors.Errorf("unable to get contact for %s", msg.URN().String()))
				}
				realURN, err := h.Backend().AddURNtoContact(ctx, msg.Channel(), contact, realIDURN)
				if err != nil {
					clog.RawError(errors.Errorf("unable to add real facebook URN %s to contact with uuid %s", realURN.String(), contact.UUID()))
				}
				referralIDExtURN, err := urns.NewURNFromParts(urns.ExternalScheme, referralID, "", "")
				if err != nil {
					clog.RawError(errors.Errorf("unable to make ext urn from %s", referralID))
				}
				extURN, err := h.Backend().AddURNtoContact(ctx, msg.Channel(), contact, referralIDExtURN)
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
		status.SetStatus(courier.MsgWired)
	}

	return status, nil
}

type wacMTMedia struct {
	ID       string `json:"id,omitempty"`
	Link     string `json:"link,omitempty"`
	Caption  string `json:"caption,omitempty"`
	Filename string `json:"filename,omitempty"`
}

type wacMTSection struct {
	Title string            `json:"title,omitempty"`
	Rows  []wacMTSectionRow `json:"rows" validate:"required"`
}

type wacMTSectionRow struct {
	ID          string `json:"id" validate:"required"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

type wacMTButton struct {
	Type  string `json:"type" validate:"required"`
	Reply struct {
		ID    string `json:"id" validate:"required"`
		Title string `json:"title" validate:"required"`
	} `json:"reply" validate:"required"`
}

type wacParam struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type wacComponent struct {
	Type    string      `json:"type"`
	SubType string      `json:"sub_type"`
	Index   string      `json:"index"`
	Params  []*wacParam `json:"parameters"`
}

type wacText struct {
	Body       string `json:"body"`
	PreviewURL bool   `json:"preview_url"`
}

type wacLanguage struct {
	Policy string `json:"policy"`
	Code   string `json:"code"`
}

type wacTemplate struct {
	Name       string          `json:"name"`
	Language   *wacLanguage    `json:"language"`
	Components []*wacComponent `json:"components"`
}

type wacInteractive struct {
	Type   string `json:"type"`
	Header *struct {
		Type     string      `json:"type"`
		Text     string      `json:"text,omitempty"`
		Video    *wacMTMedia `json:"video,omitempty"`
		Image    *wacMTMedia `json:"image,omitempty"`
		Document *wacMTMedia `json:"document,omitempty"`
	} `json:"header,omitempty"`
	Body struct {
		Text string `json:"text"`
	} `json:"body" validate:"required"`
	Footer *struct {
		Text string `json:"text"`
	} `json:"footer,omitempty"`
	Action *struct {
		Button   string         `json:"button,omitempty"`
		Sections []wacMTSection `json:"sections,omitempty"`
		Buttons  []wacMTButton  `json:"buttons,omitempty"`
	} `json:"action,omitempty"`
}

type wacMTPayload struct {
	MessagingProduct string `json:"messaging_product"`
	RecipientType    string `json:"recipient_type"`
	To               string `json:"to"`
	Type             string `json:"type"`

	Text *wacText `json:"text,omitempty"`

	Document *wacMTMedia `json:"document,omitempty"`
	Image    *wacMTMedia `json:"image,omitempty"`
	Audio    *wacMTMedia `json:"audio,omitempty"`
	Video    *wacMTMedia `json:"video,omitempty"`

	Interactive *wacInteractive `json:"interactive,omitempty"`

	Template *wacTemplate `json:"template,omitempty"`
}

type wacMTResponse struct {
	Messages []*struct {
		ID string `json:"id"`
	} `json:"messages"`
	Error struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}

func (h *handler) sendCloudAPIWhatsappMsg(ctx context.Context, msg courier.Msg, clog *courier.ChannelLog) (courier.MsgStatus, error) {
	// can't do anything without an access token
	accessToken := h.Server().Config().WhatsappAdminSystemUserToken

	base, _ := url.Parse(graphURL)
	path, _ := url.Parse(fmt.Sprintf("/%s/messages", msg.Channel().Address()))
	wacPhoneURL := base.ResolveReference(path)

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored, clog)

	hasCaption := false

	msgParts := make([]string, 0)
	if msg.Text() != "" {
		msgParts = handlers.SplitMsgByChannel(msg.Channel(), msg.Text(), maxMsgLength)
	}
	qrs := msg.QuickReplies()

	var payloadAudio wacMTPayload

	for i := 0; i < len(msgParts)+len(msg.Attachments()); i++ {
		payload := wacMTPayload{MessagingProduct: "whatsapp", RecipientType: "individual", To: msg.URN().Path()}

		if len(msg.Attachments()) == 0 {
			// do we have a template?
			var templating *MsgTemplating
			templating, err := h.getTemplate(msg)
			if err != nil {
				return nil, errors.Wrapf(err, "unable to decode template: %s for channel: %s", string(msg.Metadata()), msg.Channel().UUID())
			}
			if templating != nil {

				payload.Type = "template"

				template := wacTemplate{Name: templating.Template.Name, Language: &wacLanguage{Policy: "deterministic", Code: templating.Language}}
				payload.Template = &template

				component := &wacComponent{Type: "body"}

				for _, v := range templating.Variables {
					component.Params = append(component.Params, &wacParam{Type: "text", Text: v})
				}
				template.Components = append(payload.Template.Components, component)

			} else {
				if i < (len(msgParts) + len(msg.Attachments()) - 1) {
					// this is still a msg part
					text := &wacText{PreviewURL: false}
					payload.Type = "text"
					if strings.Contains(msgParts[i-len(msg.Attachments())], "https://") || strings.Contains(msgParts[i-len(msg.Attachments())], "http://") {
						text.PreviewURL = true
					}
					text.Body = msgParts[i-len(msg.Attachments())]
					payload.Text = text
				} else {
					if len(qrs) > 0 {
						payload.Type = "interactive"
						// We can use buttons
						if len(qrs) <= 3 {
							interactive := wacInteractive{Type: "button", Body: struct {
								Text string "json:\"text\""
							}{Text: msgParts[i-len(msg.Attachments())]}}

							btns := make([]wacMTButton, len(qrs))
							for i, qr := range qrs {
								btns[i] = wacMTButton{
									Type: "reply",
								}
								btns[i].Reply.ID = fmt.Sprint(i)
								btns[i].Reply.Title = qr
							}
							interactive.Action = &struct {
								Button   string         "json:\"button,omitempty\""
								Sections []wacMTSection "json:\"sections,omitempty\""
								Buttons  []wacMTButton  "json:\"buttons,omitempty\""
							}{Buttons: btns}
							payload.Interactive = &interactive
						} else if len(qrs) <= 10 {
							interactive := wacInteractive{Type: "list", Body: struct {
								Text string "json:\"text\""
							}{Text: msgParts[i-len(msg.Attachments())]}}

							section := wacMTSection{
								Rows: make([]wacMTSectionRow, len(qrs)),
							}
							for i, qr := range qrs {
								section.Rows[i] = wacMTSectionRow{
									ID:    fmt.Sprint(i),
									Title: qr,
								}
							}

							interactive.Action = &struct {
								Button   string         "json:\"button,omitempty\""
								Sections []wacMTSection "json:\"sections,omitempty\""
								Buttons  []wacMTButton  "json:\"buttons,omitempty\""
							}{Button: "Menu", Sections: []wacMTSection{
								section,
							}}

							payload.Interactive = &interactive
						} else {
							return nil, fmt.Errorf("too many quick replies WAC supports only up to 10 quick replies")
						}
					} else {
						// this is still a msg part
						text := &wacText{PreviewURL: false}
						payload.Type = "text"
						if strings.Contains(msgParts[i-len(msg.Attachments())], "https://") || strings.Contains(msgParts[i-len(msg.Attachments())], "http://") {
							text.PreviewURL = true
						}
						text.Body = msgParts[i-len(msg.Attachments())]
						payload.Text = text
					}
				}
			}

		} else if i < len(msg.Attachments()) && (len(qrs) == 0 || len(qrs) > 3) {
			attType, attURL := handlers.SplitAttachment(msg.Attachments()[i])
			attType = strings.Split(attType, "/")[0]
			if attType == "application" {
				attType = "document"
			}
			payload.Type = attType
			media := wacMTMedia{Link: attURL}

			if len(msgParts) == 1 && attType != "audio" && len(msg.Attachments()) == 1 && len(msg.QuickReplies()) == 0 {
				media.Caption = msgParts[i]
				hasCaption = true
			}

			if attType == "image" {
				payload.Image = &media
			} else if attType == "audio" {
				payload.Audio = &media
			} else if attType == "video" {
				payload.Video = &media
			} else if attType == "document" {
				payload.Document = &media
			}
		} else {
			if len(qrs) > 0 {
				payload.Type = "interactive"
				// We can use buttons
				if len(qrs) <= 3 {
					interactive := wacInteractive{Type: "button", Body: struct {
						Text string "json:\"text\""
					}{Text: msgParts[i]}}

					if len(msg.Attachments()) > 0 {
						hasCaption = true
						attType, attURL := handlers.SplitAttachment(msg.Attachments()[i])
						attType = strings.Split(attType, "/")[0]
						if attType == "application" {
							attType = "document"
						}
						if attType == "image" {
							image := wacMTMedia{
								Link: attURL,
							}
							interactive.Header = &struct {
								Type     string      "json:\"type\""
								Text     string      "json:\"text,omitempty\""
								Video    *wacMTMedia "json:\"video,omitempty\""
								Image    *wacMTMedia "json:\"image,omitempty\""
								Document *wacMTMedia "json:\"document,omitempty\""
							}{Type: "image", Image: &image}
						} else if attType == "video" {
							video := wacMTMedia{
								Link: attURL,
							}
							interactive.Header = &struct {
								Type     string      "json:\"type\""
								Text     string      "json:\"text,omitempty\""
								Video    *wacMTMedia "json:\"video,omitempty\""
								Image    *wacMTMedia "json:\"image,omitempty\""
								Document *wacMTMedia "json:\"document,omitempty\""
							}{Type: "video", Video: &video}
						} else if attType == "document" {
							filename, err := utils.BasePathForURL(attURL)
							if err != nil {
								return nil, err
							}
							document := wacMTMedia{
								Link:     attURL,
								Filename: filename,
							}
							interactive.Header = &struct {
								Type     string      "json:\"type\""
								Text     string      "json:\"text,omitempty\""
								Video    *wacMTMedia "json:\"video,omitempty\""
								Image    *wacMTMedia "json:\"image,omitempty\""
								Document *wacMTMedia "json:\"document,omitempty\""
							}{Type: "document", Document: &document}
						} else if attType == "audio" {
							var zeroIndex bool
							if i == 0 {
								zeroIndex = true
							}
							payloadAudio = wacMTPayload{MessagingProduct: "whatsapp", RecipientType: "individual", To: msg.URN().Path(), Type: "audio", Audio: &wacMTMedia{Link: attURL}}
							status, err := requestWAC(payloadAudio, accessToken, status, wacPhoneURL, zeroIndex, clog)
							if err != nil {
								return status, nil
							}
						} else {
							interactive.Type = "button"
							interactive.Body.Text = msgParts[i]
						}
					}

					btns := make([]wacMTButton, len(qrs))
					for i, qr := range qrs {
						btns[i] = wacMTButton{
							Type: "reply",
						}
						btns[i].Reply.ID = fmt.Sprint(i)
						btns[i].Reply.Title = qr
					}
					interactive.Action = &struct {
						Button   string         "json:\"button,omitempty\""
						Sections []wacMTSection "json:\"sections,omitempty\""
						Buttons  []wacMTButton  "json:\"buttons,omitempty\""
					}{Buttons: btns}
					payload.Interactive = &interactive

				} else if len(qrs) <= 10 {
					interactive := wacInteractive{Type: "list", Body: struct {
						Text string "json:\"text\""
					}{Text: msgParts[i-len(msg.Attachments())]}}

					section := wacMTSection{
						Rows: make([]wacMTSectionRow, len(qrs)),
					}
					for i, qr := range qrs {
						section.Rows[i] = wacMTSectionRow{
							ID:    fmt.Sprint(i),
							Title: qr,
						}
					}

					interactive.Action = &struct {
						Button   string         "json:\"button,omitempty\""
						Sections []wacMTSection "json:\"sections,omitempty\""
						Buttons  []wacMTButton  "json:\"buttons,omitempty\""
					}{Button: "Menu", Sections: []wacMTSection{
						section,
					}}

					payload.Interactive = &interactive
				} else {
					return nil, fmt.Errorf("too many quick replies WAC supports only up to 10 quick replies")
				}
			} else {
				// this is still a msg part
				text := &wacText{PreviewURL: false}
				payload.Type = "text"
				if strings.Contains(msgParts[i-len(msg.Attachments())], "https://") || strings.Contains(msgParts[i-len(msg.Attachments())], "http://") {
					text.PreviewURL = true
				}
				text.Body = msgParts[i-len(msg.Attachments())]
				payload.Text = text
			}
		}

		var zeroIndex bool
		if i == 0 {
			zeroIndex = true
		}

		status, err := requestWAC(payload, accessToken, status, wacPhoneURL, zeroIndex, clog)
		if err != nil {
			return status, err
		}

		if hasCaption {
			break
		}
	}
	return status, nil
}

func requestWAC(payload wacMTPayload, accessToken string, status courier.MsgStatus, wacPhoneURL *url.URL, zeroIndex bool, clog *courier.ChannelLog) (courier.MsgStatus, error) {
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return status, err
	}

	req, err := http.NewRequest(http.MethodPost, wacPhoneURL.String(), bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	_, respBody, _ := handlers.RequestHTTP(req, clog)
	respPayload := &wacMTResponse{}
	err = json.Unmarshal(respBody, respPayload)
	if err != nil {
		clog.Error(courier.ErrorResponseUnparseable("JSON"))
		return status, nil
	}

	if respPayload.Error.Code != 0 {
		clog.Error(courier.ErrorExternal(strconv.Itoa(respPayload.Error.Code), respPayload.Error.Message))
		return status, nil
	}

	externalID := respPayload.Messages[0].ID
	if zeroIndex && externalID != "" {
		status.SetExternalID(externalID)
	}
	// this was wired successfully
	status.SetStatus(courier.MsgWired)
	return status, nil
}

// DescribeURN looks up URN metadata for new contacts
func (h *handler) DescribeURN(ctx context.Context, channel courier.Channel, urn urns.URN, clog *courier.ChannelLog) (map[string]string, error) {
	if channel.ChannelType() == "WAC" {
		return map[string]string{}, nil
	}

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
	var name string

	if fmt.Sprint(channel.ChannelType()) == "FBA" {
		query.Set("fields", "first_name,last_name")
	}

	query.Set("access_token", accessToken)
	u.RawQuery = query.Encode()
	req, _ := http.NewRequest(http.MethodGet, u.String(), nil)

	resp, respBody, err := handlers.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return nil, errors.New("unable to look up contact data")
	}

	// read our first and last name	or complete name
	if fmt.Sprint(channel.ChannelType()) == "FBA" {
		firstName, _ := jsonparser.GetString(respBody, "first_name")
		lastName, _ := jsonparser.GetString(respBody, "last_name")
		name = utils.JoinNonEmpty(" ", firstName, lastName)
	} else {
		name, _ = jsonparser.GetString(respBody, "name")
	}

	return map[string]string{"name": name}, nil

}

// see https://developers.facebook.com/docs/messenger-platform/webhook#security
func (h *handler) validateSignature(r *http.Request) error {
	headerSignature := r.Header.Get(signatureHeader)
	if headerSignature == "" {
		return fmt.Errorf("missing request signature")
	}
	appSecret := h.Server().Config().FacebookApplicationSecret

	body, err := handlers.ReadBody(r, maxRequestBodyBytes)
	if err != nil {
		return fmt.Errorf("unable to read request body: %s", err)
	}

	expectedSignature, err := fbCalculateSignature(appSecret, body)
	if err != nil {
		return err
	}

	signature := ""
	if len(headerSignature) == 71 && strings.HasPrefix(headerSignature, "sha256=") {
		signature = strings.TrimPrefix(headerSignature, "sha256=")
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
	mac := hmac.New(sha256.New, []byte(appSecret))
	mac.Write(buffer.Bytes())

	return hex.EncodeToString(mac.Sum(nil)), nil
}

func (h *handler) getTemplate(msg courier.Msg) (*MsgTemplating, error) {
	mdJSON := msg.Metadata()
	if len(mdJSON) == 0 {
		return nil, nil
	}
	metadata := &TemplateMetadata{}
	err := json.Unmarshal(mdJSON, metadata)
	if err != nil {
		return nil, err
	}
	templating := metadata.Templating
	if templating == nil {
		return nil, nil
	}

	// check our template is valid
	err = utils.Validate(templating)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid templating definition")
	}
	// check country
	if templating.Country != "" {
		templating.Language = fmt.Sprintf("%s_%s", templating.Language, templating.Country)
	}

	// map our language from iso639-3_iso3166-2 to the WA country / iso638-2 pair
	language, found := languageMap[templating.Language]
	if !found {
		return nil, fmt.Errorf("unable to find mapping for language: %s", templating.Language)
	}
	templating.Language = language

	return templating, err
}

// BuildAttachmentRequest to download media for message attachment with Bearer token set
func (h *handler) BuildAttachmentRequest(ctx context.Context, b courier.Backend, channel courier.Channel, attachmentURL string, clog *courier.ChannelLog) (*http.Request, error) {
	token := h.Server().Config().WhatsappAdminSystemUserToken
	if token == "" {
		return nil, fmt.Errorf("missing token for WAC channel")
	}

	req, _ := http.NewRequest(http.MethodGet, attachmentURL, nil)

	// set the access token as the authorization header for WAC
	if channel.ChannelType() == "WAC" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req, nil
}

func (*handler) AttachmentRequestClient(ch courier.Channel) *http.Client {
	return utils.GetHTTPClient()
}

var _ courier.AttachmentRequestBuilder = (*handler)(nil)

type TemplateMetadata struct {
	Templating *MsgTemplating `json:"templating"`
}

type MsgTemplating struct {
	Template struct {
		Name string `json:"name" validate:"required"`
		UUID string `json:"uuid" validate:"required"`
	} `json:"template" validate:"required,dive"`
	Language  string   `json:"language" validate:"required"`
	Country   string   `json:"country"`
	Namespace string   `json:"namespace"`
	Variables []string `json:"variables"`
}

// mapping from iso639-3_iso3166-2 to WA language code
var languageMap = map[string]string{
	"afr":    "af",    // Afrikaans
	"sqi":    "sq",    // Albanian
	"ara":    "ar",    // Arabic
	"aze":    "az",    // Azerbaijani
	"ben":    "bn",    // Bengali
	"bul":    "bg",    // Bulgarian
	"cat":    "ca",    // Catalan
	"zho":    "zh_CN", // Chinese
	"zho_CN": "zh_CN", // Chinese (CHN)
	"zho_HK": "zh_HK", // Chinese (HKG)
	"zho_TW": "zh_TW", // Chinese (TAI)
	"hrv":    "hr",    // Croatian
	"ces":    "cs",    // Czech
	"dah":    "da",    // Danish
	"nld":    "nl",    // Dutch
	"eng":    "en",    // English
	"eng_GB": "en_GB", // English (UK)
	"eng_US": "en_US", // English (US)
	"est":    "et",    // Estonian
	"fil":    "fil",   // Filipino
	"fin":    "fi",    // Finnish
	"fra":    "fr",    // French
	"kat":    "ka",    // Georgian
	"deu":    "de",    // German
	"ell":    "el",    // Greek
	"guj":    "gu",    // Gujarati
	"hau":    "ha",    // Hausa
	"enb":    "he",    // Hebrew
	"hin":    "hi",    // Hindi
	"hun":    "hu",    // Hungarian
	"ind":    "id",    // Indonesian
	"gle":    "ga",    // Irish
	"ita":    "it",    // Italian
	"jpn":    "ja",    // Japanese
	"kan":    "kn",    // Kannada
	"kaz":    "kk",    // Kazakh
	"kin":    "rw_RW", // Kinyarwanda
	"kor":    "ko",    // Korean
	"kir":    "ky_KG", // Kyrgyzstan
	"lao":    "lo",    // Lao
	"lav":    "lv",    // Latvian
	"lit":    "lt",    // Lithuanian
	"mal":    "ml",    // Malayalam
	"mkd":    "mk",    // Macedonian
	"msa":    "ms",    // Malay
	"mar":    "mr",    // Marathi
	"nob":    "nb",    // Norwegian
	"fas":    "fa",    // Persian
	"pol":    "pl",    // Polish
	"por":    "pt_PT", // Portuguese
	"por_BR": "pt_BR", // Portuguese (BR)
	"por_PT": "pt_PT", // Portuguese (POR)
	"pan":    "pa",    // Punjabi
	"ron":    "ro",    // Romanian
	"rus":    "ru",    // Russian
	"srp":    "sr",    // Serbian
	"slk":    "sk",    // Slovak
	"slv":    "sl",    // Slovenian
	"spa":    "es",    // Spanish
	"spa_AR": "es_AR", // Spanish (ARG)
	"spa_ES": "es_ES", // Spanish (SPA)
	"spa_MX": "es_MX", // Spanish (MEX)
	"swa":    "sw",    // Swahili
	"swe":    "sv",    // Swedish
	"tam":    "ta",    // Tamil
	"tel":    "te",    // Telugu
	"tha":    "th",    // Thai
	"tur":    "tr",    // Turkish
	"ukr":    "uk",    // Ukrainian
	"urd":    "ur",    // Urdu
	"uzb":    "uz",    // Uzbek
	"vie":    "vi",    // Vietnamese
	"zul":    "zu",    // Zulu
}
