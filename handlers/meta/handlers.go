package meta

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/handlers/meta/messenger"
	"github.com/nyaruka/courier/handlers/meta/whatsapp"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
)

// Endpoints we hit
var (
	sendURL  = "https://graph.facebook.com/v22.0/me/messages"
	graphURL = "https://graph.facebook.com/v22.0/"

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

func newHandler(channelType models.ChannelType, name string) courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(channelType, name, handlers.DisableUUIDRouting(), handlers.WithRedactConfigKeys(models.ConfigAuthToken))}
}

func init() {
	courier.RegisterHandler(newHandler("IG", "Instagram"))
	courier.RegisterHandler(newHandler("FBA", "Facebook"))
	courier.RegisterHandler(newHandler("WAC", "WhatsApp Cloud"))

}

type handler struct {
	handlers.BaseHandler
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodGet, "receive", courier.ChannelLogTypeWebhookVerify, h.receiveVerify)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMultiReceive, handlers.JSONPayload(h, h.receiveEvents))
	return nil
}

// https://developers.facebook.com/docs/whatsapp/cloud-api/webhooks/components#notification-payload-object
//
//	{
//	  "object":"page",
//	  "entry":[{
//	    "id":"180005062406476",
//	    "time":1514924367082,
//	    "messaging":[{
//	      "sender":  {"id":"1630934236957797"},
//	      "recipient":{"id":"180005062406476"},
//	      "timestamp":1514924366807,
//	      "message":{
//	        "mid":"mid.$cAAD5QiNHkz1m6cyj11guxokwkhi2",
//	        "seq":33116,
//	        "text":"65863634"
//	      }
//	    }]
//	  }]
//	}
type Notifications struct {
	Object string `json:"object"`
	Entry  []struct {
		ID        string                `json:"id"`
		Time      int64                 `json:"time"`
		Changes   []whatsapp.Change     `json:"changes"`   // used by WhatsApp
		Messaging []messenger.Messaging `json:"messaging"` // used by Facebook and Instgram
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

	payload := &Notifications{}
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
		return h.Backend().GetChannelByAddress(ctx, models.ChannelType("FBA"), models.ChannelAddress(channelAddress))
	} else if payload.Object == "instagram" {
		channelAddress = payload.Entry[0].ID
		return h.Backend().GetChannelByAddress(ctx, models.ChannelType("IG"), models.ChannelAddress(channelAddress))
	} else {
		if len(payload.Entry[0].Changes) == 0 {
			return nil, fmt.Errorf("no changes found")
		}

		channelAddress = payload.Entry[0].Changes[0].Value.Metadata.PhoneNumberID
		if channelAddress == "" {
			return nil, fmt.Errorf("no channel address found")
		}
		return h.Backend().GetChannelByAddress(ctx, models.ChannelType("WAC"), models.ChannelAddress(channelAddress))
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
	if !utils.SecretEqual(secret, h.Server().Config().FacebookWebhookSecret) {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("token does not match secret"))
	}
	// and respond with the challenge token
	_, err := fmt.Fprint(w, r.URL.Query().Get("hub.challenge"))
	return nil, err
}

func (h *handler) resolveMediaURL(mediaID string, token string, clog *courier.ChannelLog) (string, error) {
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

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return "", errors.New("error resolving media URL")
	}

	mediaURL, err := jsonparser.GetString(respBody, "url")
	return mediaURL, err
}

// receiveEvents is our HTTP handler function for incoming messages and status updates
func (h *handler) receiveEvents(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *Notifications, clog *courier.ChannelLog) ([]courier.Event, error) {
	err := h.validateSignature(r)
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
	var data []any

	if channel.ChannelType() == "FBA" || channel.ChannelType() == "IG" {
		events, data, err = h.processFacebookInstagramPayload(ctx, channel, payload, w, r, clog)
	} else {
		events, data, err = h.processWhatsAppPayload(ctx, channel, payload, w, r, clog)

	}

	if err != nil {
		return nil, err
	}

	return events, courier.WriteDataResponse(w, http.StatusOK, "Events Handled", data)
}

func (h *handler) processWhatsAppPayload(ctx context.Context, channel courier.Channel, payload *Notifications, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, []any, error) {
	// the list of events we deal with
	events := make([]courier.Event, 0, 2)

	// the list of data we will return in our response
	data := make([]any, 0, 2)

	token := h.Server().Config().WhatsappAdminSystemUserToken

	seenMsgIDs := make(map[string]bool, 2)
	contactNames := make(map[string]string)

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
				if seenMsgIDs[msg.ID] {
					continue
				}

				// create our date from the timestamp
				ts, err := strconv.ParseInt(msg.Timestamp, 10, 64)
				if err != nil {
					return nil, nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("invalid timestamp: %s", msg.Timestamp))
				}
				date := parseTimestamp(ts)

				urn, err := urns.New(urns.WhatsApp, msg.From)
				if err != nil {
					return nil, nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, errors.New("invalid whatsapp id"))
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
					mediaURL, err = h.resolveMediaURL(msg.Audio.ID, token, clog)
				} else if msg.Type == "voice" && msg.Voice != nil {
					text = msg.Voice.Caption
					mediaURL, err = h.resolveMediaURL(msg.Voice.ID, token, clog)
				} else if msg.Type == "button" && msg.Button != nil {
					text = msg.Button.Text
				} else if msg.Type == "document" && msg.Document != nil {
					text = msg.Document.Caption
					mediaURL, err = h.resolveMediaURL(msg.Document.ID, token, clog)
				} else if msg.Type == "image" && msg.Image != nil {
					text = msg.Image.Caption
					mediaURL, err = h.resolveMediaURL(msg.Image.ID, token, clog)
				} else if msg.Type == "video" && msg.Video != nil {
					text = msg.Video.Caption
					mediaURL, err = h.resolveMediaURL(msg.Video.ID, token, clog)
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
				event := h.Backend().NewIncomingMsg(ctx, channel, urn, text, msg.ID, clog).WithReceivedOn(date).WithContactName(contactNames[msg.From])

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

				events = append(events, event)
				data = append(data, courier.NewMsgReceiveData(event))
				seenMsgIDs[msg.ID] = true
			}

			for _, status := range change.Value.Statuses {

				msgStatus, found := whatsapp.StatusMapping[status.Status]
				if !found {
					if whatsapp.IgnoreStatuses[status.Status] {
						data = append(data, courier.NewInfoData(fmt.Sprintf("ignoring status: %s", status.Status)))
					} else {
						handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unknown status: %s", status.Status))
					}
					continue
				}

				for _, statusError := range status.Errors {
					clog.Error(courier.ErrorExternal(strconv.Itoa(statusError.Code), statusError.Title))
				}

				event := h.Backend().NewStatusUpdateByExternalID(channel, status.ID, msgStatus, clog)
				err := h.Backend().WriteStatusUpdate(ctx, event)
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

func (h *handler) processFacebookInstagramPayload(ctx context.Context, channel courier.Channel, payload *Notifications, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, []any, error) {
	var err error

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

		date := parseTimestamp(msg.Timestamp)

		sender := msg.Sender.UserRef
		if sender == "" {
			sender = msg.Sender.ID
		}

		var urn urns.URN

		// create our URN
		if payload.Object == "instagram" {
			urn, err = urns.New(urns.Instagram, sender)
			if err != nil {
				return nil, nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, errors.New("invalid instagram id"))
			}
		} else {
			urn, err = urns.New(urns.Facebook, sender)
			if err != nil {
				return nil, nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, errors.New("invalid facebook id"))
			}
		}

		if msg.OptIn != nil {
			var event courier.ChannelEvent

			if msg.OptIn.Type == "notification_messages" {
				eventType := models.EventTypeOptIn
				authToken := msg.OptIn.NotificationMessagesToken

				if msg.OptIn.NotificationMessagesStatus == "STOP_NOTIFICATIONS" {
					eventType = models.EventTypeOptOut
					authToken = "" // so that we remove it
				}

				event = h.Backend().NewChannelEvent(channel, eventType, urn, clog).
					WithOccurredOn(date).
					WithExtra(map[string]string{titleKey: msg.OptIn.Title, payloadKey: msg.OptIn.Payload}).
					WithURNAuthTokens(map[string]string{fmt.Sprintf("optin:%s", msg.OptIn.Payload): authToken})
			} else {

				// this is an opt in, if we have a user_ref, use that as our URN (this is a checkbox plugin)
				// TODO:
				//    We need to deal with the case of them responding and remapping the user_ref in that case:
				//    https://developers.facebook.com/docs/messenger-platform/discovery/checkbox-plugin
				//    Right now that we even support this isn't documented and I don't think anybody uses it, so leaving that out.
				//    (things will still work, we just will have dupe contacts, one with user_ref for the first contact, then with the real id when they reply)
				if msg.OptIn.UserRef != "" {
					urn, err = urns.New(urns.Facebook, urns.FacebookRefPrefix+msg.OptIn.UserRef)
					if err != nil {
						return nil, nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
					}
				}

				event = h.Backend().NewChannelEvent(channel, models.EventTypeReferral, urn, clog).
					WithOccurredOn(date).
					WithExtra(map[string]string{referrerIDKey: msg.OptIn.Ref})
			}

			err := h.Backend().WriteChannelEvent(ctx, event, clog)
			if err != nil {
				return nil, nil, err
			}

			events = append(events, event)
			data = append(data, courier.NewEventReceiveData(event))

		} else if msg.Postback != nil {
			// by default postbacks are treated as new conversations, unless we have referral information
			eventType := models.EventTypeNewConversation
			if msg.Postback.Referral.Ref != "" {
				eventType = models.EventTypeReferral
			}
			event := h.Backend().NewChannelEvent(channel, eventType, urn, clog).WithOccurredOn(date)

			// build our extra
			extra := map[string]string{titleKey: msg.Postback.Title, payloadKey: msg.Postback.Payload}

			// add in referral information if we have it
			if eventType == models.EventTypeReferral {
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
			event := h.Backend().NewChannelEvent(channel, models.EventTypeReferral, urn, clog).WithOccurredOn(date)

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
				return nil, nil, err
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

			if msg.Message.IsDeleted {
				h.Backend().DeleteMsgByExternalID(ctx, channel, msg.Message.MID)
				data = append(data, courier.NewInfoData("msg deleted"))
				continue
			}

			text := msg.Message.Text
			attachmentURLs := make([]string, 0, 2)

			for _, att := range msg.Message.Attachments {
				// if we have a sticker ID, use that as our text
				if att.Type == "image" && att.Payload != nil && att.Payload.StickerID != 0 {
					text = stickerIDToEmoji[att.Payload.StickerID]
				}
				if att.Type == "like_heart" {
					text = "❤️"
				}

				if att.Type == "location" {
					attachmentURLs = append(attachmentURLs, fmt.Sprintf("geo:%f,%f", att.Payload.Coordinates.Lat, att.Payload.Coordinates.Long))
				}

				if att.Type == "story_mention" {
					data = append(data, courier.NewInfoData("ignoring story_mention"))
					continue
				}

				if att.Payload != nil && att.Payload.URL != "" && att.Type != "fallback" && strings.HasPrefix(att.Payload.URL, "http") {
					attachmentURLs = append(attachmentURLs, att.Payload.URL)
				}
			}

			// if we have no text or accepted attachments, don't create a message
			if text == "" && len(attachmentURLs) == 0 {
				continue
			}

			// create our message
			event := h.Backend().NewIncomingMsg(ctx, channel, urn, text, msg.Message.MID, clog).WithReceivedOn(date)

			// add any attachment URL found
			for _, attURL := range attachmentURLs {
				event.WithAttachment(attURL)
			}

			err := h.Backend().WriteMsg(ctx, event, clog)
			if err != nil {
				return nil, nil, err
			}

			events = append(events, event)
			data = append(data, courier.NewMsgReceiveData(event))
			seenMsgIDs[msg.Message.MID] = true

		} else if msg.Delivery != nil {
			// this is a delivery report
			for _, mid := range msg.Delivery.MIDs {
				event := h.Backend().NewStatusUpdateByExternalID(channel, mid, models.MsgStatusDelivered, clog)
				err := h.Backend().WriteStatusUpdate(ctx, event)
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

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	if msg.Channel().ChannelType() == "FBA" || msg.Channel().ChannelType() == "IG" {
		return h.sendFacebookInstagramMsg(ctx, msg, res, clog)
	} else if msg.Channel().ChannelType() == "WAC" {
		return h.sendWhatsAppMsg(ctx, msg, res, clog)
	}

	return fmt.Errorf("unssuported channel type")
}

func (h *handler) sendFacebookInstagramMsg(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	// can't do anything without an access token
	accessToken := msg.Channel().StringConfigForKey(models.ConfigAuthToken, "")
	if accessToken == "" {
		return courier.ErrChannelConfig
	}

	isHuman := msg.Origin() == models.MsgOriginChat || msg.Origin() == models.MsgOriginTicket
	payload := &messenger.SendRequest{}

	// build our recipient
	if IsFacebookRef(msg.URN()) {
		payload.Recipient.UserRef = FacebookRef(msg.URN())
	} else if msg.URNAuth() != "" {
		payload.Recipient.NotificationMessagesToken = msg.URNAuth()
	} else {
		payload.Recipient.ID = msg.URN().Path()
	}

	if isHuman {
		payload.MessagingType = "MESSAGE_TAG"
		payload.Tag = "HUMAN_AGENT"
	} else {
		if msg.ResponseToExternalID() != "" {
			payload.MessagingType = "RESPONSE"
		} else {
			payload.MessagingType = "UPDATE"
		}
	}

	msgURL, _ := url.Parse(sendURL)
	query := url.Values{}
	query.Set("access_token", accessToken)
	msgURL.RawQuery = query.Encode()

	// Send each text segment and attachment separately. We send attachments first as otherwise quick replies get
	// attached to attachment segments and are hidden when images load.
	for _, part := range handlers.SplitMsg(msg, handlers.SplitOptions{MaxTextLen: maxMsgLength}) {
		if part.Type == handlers.MsgPartTypeOptIn {
			payload.Message.Attachment = &messenger.Attachment{}
			payload.Message.Attachment.Type = "template"
			payload.Message.Attachment.Payload.TemplateType = "notification_messages"
			payload.Message.Attachment.Payload.Title = part.OptIn.Name
			payload.Message.Attachment.Payload.Payload = fmt.Sprint(part.OptIn.ID)
			payload.Message.Text = ""

		} else if part.Type == handlers.MsgPartTypeAttachment {
			payload.Message.Attachment = &messenger.Attachment{}
			attType, attURL := handlers.SplitAttachment(part.Attachment)
			attType = strings.Split(attType, "/")[0]
			if attType == "application" || attType == "document" {
				attType = "file"
			}
			payload.Message.Attachment.Type = attType
			payload.Message.Attachment.Payload.URL = attURL
			payload.Message.Attachment.Payload.IsReusable = true
			payload.Message.Text = ""

		} else {
			payload.Message.Text = part.Text
			payload.Message.Attachment = nil
		}

		// include any quick replies on the last piece we send
		if part.IsLast {
			for _, qr := range msg.QuickReplies() {
				payload.Message.QuickReplies = append(payload.Message.QuickReplies, messenger.QuickReply{Title: qr.Text, Payload: qr.Text, ContentType: "text"})
			}
		} else {
			payload.Message.QuickReplies = nil
		}

		jsonBody := jsonx.MustMarshal(payload)

		req, err := http.NewRequest(http.MethodPost, msgURL.String(), bytes.NewReader(jsonBody))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 == 5 {
			return courier.ErrConnectionFailed
		} else if resp.StatusCode/100 != 2 {
			return courier.ErrResponseStatus
		}

		respPayload := &messenger.SendResponse{}
		err = json.Unmarshal(respBody, respPayload)
		if err != nil {
			return courier.ErrResponseUnparseable
		}

		if respPayload.Error.Code != 0 {
			return courier.ErrFailedWithReason(strconv.Itoa(respPayload.Error.Code), respPayload.Error.Message)
		}

		if respPayload.ExternalID == "" {
			return courier.ErrResponseUnexpected
		}

		res.AddExternalID(respPayload.ExternalID)
		if IsFacebookRef(msg.URN()) {
			recipientID := respPayload.RecipientID
			if recipientID == "" {
				return courier.ErrResponseUnexpected
			}

			referralID := FacebookRef(msg.URN())

			realIDURN, err := urns.New(urns.Facebook, recipientID)
			if err != nil {
				clog.RawError(fmt.Errorf("unable to make facebook urn from %s", recipientID))
			}

			contact, err := h.Backend().GetContact(ctx, msg.Channel(), msg.URN(), nil, "", true, clog)
			if err != nil {
				clog.RawError(fmt.Errorf("unable to get contact for %s", msg.URN().String()))
			}
			realURN, err := h.Backend().AddURNtoContact(ctx, msg.Channel(), contact, realIDURN, nil)
			if err != nil {
				clog.RawError(fmt.Errorf("unable to add real facebook URN %s to contact with uuid %s", realURN.String(), contact.UUID()))
			}
			referralIDExtURN, err := urns.New(urns.External, referralID)
			if err != nil {
				clog.RawError(fmt.Errorf("unable to make ext urn from %s", referralID))
			}
			extURN, err := h.Backend().AddURNtoContact(ctx, msg.Channel(), contact, referralIDExtURN, nil)
			if err != nil {
				clog.RawError(fmt.Errorf("unable to add URN %s to contact with uuid %s", extURN.String(), contact.UUID()))
			}

			referralFacebookURN, err := h.Backend().RemoveURNfromContact(ctx, msg.Channel(), contact, msg.URN())
			if err != nil {
				clog.RawError(fmt.Errorf("unable to remove referral facebook URN %s from contact with uuid %s", referralFacebookURN.String(), contact.UUID()))
			}

		}
	}

	return nil
}

func (h *handler) sendWhatsAppMsg(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	// can't do anything without an access token
	accessToken := h.Server().Config().WhatsappAdminSystemUserToken

	base, _ := url.Parse(graphURL)
	path, _ := url.Parse(fmt.Sprintf("/%s/messages", msg.Channel().Address()))
	wacPhoneURL := base.ResolveReference(path)

	requestPayloads, err := whatsapp.GetMsgPayloads(ctx, msg, maxMsgLength, clog)
	if err != nil {
		return err
	}

	for _, payload := range requestPayloads {
		err := h.requestWAC(payload, accessToken, res, wacPhoneURL, clog)
		if err != nil {
			return err
		}
	}

	return nil
}

func (h *handler) requestWAC(payload whatsapp.SendRequest, accessToken string, res *courier.SendResult, wacPhoneURL *url.URL, clog *courier.ChannelLog) error {
	jsonBody := jsonx.MustMarshal(payload)

	req, err := http.NewRequest(http.MethodPost, wacPhoneURL.String(), bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 == 5 {
		return courier.ErrConnectionFailed
	}

	respPayload := &whatsapp.SendResponse{}
	err = json.Unmarshal(respBody, respPayload)
	if err != nil {
		return courier.ErrResponseUnparseable
	}

	if slices.Contains(whatsapp.WACThrottlingErrorCodes, respPayload.Error.Code) {
		return courier.ErrConnectionThrottled
	}

	if respPayload.Error.Code != 0 {
		return courier.ErrFailedWithReason(strconv.Itoa(respPayload.Error.Code), respPayload.Error.Message)
	}

	externalID := respPayload.Messages[0].ID
	if externalID != "" {
		res.AddExternalID(externalID)
	}
	return nil
}

// DescribeURN looks up URN metadata for new contacts
func (h *handler) DescribeURN(ctx context.Context, channel courier.Channel, urn urns.URN, clog *courier.ChannelLog) (map[string]string, error) {
	if channel.ChannelType() == "WAC" {
		return map[string]string{}, nil
	}

	// can't do anything with facebook refs, ignore them
	if IsFacebookRef(urn) {
		return map[string]string{}, nil
	}

	accessToken := channel.StringConfigForKey(models.ConfigAuthToken, "")
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

	resp, respBody, err := h.RequestHTTP(req, clog)
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

var _ courier.AttachmentRequestBuilder = (*handler)(nil)

func parseTimestamp(ts int64) time.Time {
	// sometimes Facebook sends timestamps in seconds rather than milliseconds
	if ts >= 1_000_000_000_000 {
		return time.Unix(0, ts*1000000).UTC()
	}
	return time.Unix(ts, 0).UTC()
}
