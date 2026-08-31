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
	"strconv"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/courier/v26/handlers/meta/messenger"
	"github.com/nyaruka/courier/v26/handlers/meta/whatsapp"
	"github.com/nyaruka/courier/v26/utils"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/core/events"
)

// Endpoints we hit
var (
	sendURL  = "https://graph.facebook.com/v25.0/me/messages"
	graphURL = "https://graph.facebook.com/v25.0/"

	signatureHeader = "X-Hub-Signature-256"

	maxRequestBodyBytes int64 = 1024 * 1024

	// max for the body
	maxMsgLength = 1000

	// max for the body of WhatsApp messages
	maxMsgLengthWAC = 4096

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

func newHandler(channelType models.ChannelType, name string) channels.Handler {
	return &handler{handlers.NewBaseHandler(channelType, name, handlers.DisableUUIDRouting(), handlers.WithRedactConfigKeys(models.ConfigAuthToken))}
}

func init() {
	channels.RegisterHandler(newHandler("IG", "Instagram"))
	channels.RegisterHandler(newHandler("FBA", "Facebook"))
	channels.RegisterHandler(newHandler("WAC", "WhatsApp Cloud"))

}

type handler struct {
	handlers.BaseHandler
}

// Initialize registers the routes this handler serves
func (h *handler) Initialize(r *channels.Routes) error {
	r.Add(h, http.MethodGet, "receive", models.ChannelLogTypeWebhookVerify, h.receiveVerify)
	r.AddReceive(h, http.MethodPost, "receive", models.ChannelLogTypeMultiReceive, handlers.JSONPayload(h.receiveEvents))
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

func (h *handler) RedactValues(ch *models.Channel) []string {
	vals := h.BaseHandler.RedactValues(ch)
	vals = append(vals, h.Runtime().Config.FacebookApplicationSecret, h.Runtime().Config.FacebookWebhookSecret, h.Runtime().Config.WhatsappAdminSystemUserToken)
	return vals
}

// WriteRequestError writes the passed in error to our response writer
func (h *handler) WriteRequestError(ctx context.Context, w http.ResponseWriter, err error) error {
	return channels.WriteError(w, http.StatusOK, err)
}

// GetChannel returns the channel
func (h *handler) GetChannel(ctx context.Context, r *http.Request) (*models.Channel, error) {
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
		return models.GetChannelByAddress(ctx, models.ChannelType("FBA"), models.ChannelAddress(channelAddress))
	} else if payload.Object == "instagram" {
		channelAddress = payload.Entry[0].ID
		return models.GetChannelByAddress(ctx, models.ChannelType("IG"), models.ChannelAddress(channelAddress))
	} else {
		if len(payload.Entry[0].Changes) == 0 {
			return nil, fmt.Errorf("no changes found")
		}

		channelAddress = payload.Entry[0].Changes[0].Value.Metadata.PhoneNumberID
		if channelAddress == "" {
			return nil, fmt.Errorf("no channel address found")
		}
		return models.GetChannelByAddress(ctx, models.ChannelType("WAC"), models.ChannelAddress(channelAddress))
	}
}

// receiveVerify handles Facebook's webhook verification callback
func (h *handler) receiveVerify(ctx context.Context, channel *models.Channel, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
	mode := r.URL.Query().Get("hub.mode")

	// this isn't a subscribe verification, that's an error
	if mode != "subscribe" {
		return nil, channels.WriteErrorResponse(ctx, h, w, r, channel, fmt.Errorf("unknown request"))
	}

	// verify the token against our server facebook webhook secret, if the same return the challenge FB sent us
	secret := r.URL.Query().Get("hub.verify_token")
	if !utils.SecretEqual(secret, h.Runtime().Config.FacebookWebhookSecret) {
		return nil, channels.WriteErrorResponse(ctx, h, w, r, channel, fmt.Errorf("token does not match secret"))
	}
	// and respond with the challenge token
	_, err := fmt.Fprint(w, r.URL.Query().Get("hub.challenge"))
	return nil, err
}

func (h *handler) resolveMediaURL(mediaID string, token string, clog *models.ChannelLog) (string, error) {
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

// receiveEvents is our receive function for incoming messages and status updates
func (h *handler) receiveEvents(ctx context.Context, channel *models.Channel, r *http.Request, payload *Notifications, in *channels.Received, clog *models.ChannelLog) error {
	if err := h.validateSignature(r); err != nil {
		return err
	}

	// a payload with an unexpected object or no entries never gets here - GetChannel decodes the same body
	// and fails the request first

	// a failure here is in the payload itself, so asking for it again wouldn't get any further. The seam writes
	// whatever was parsed ahead of it rather than dropping it.
	if channel.ChannelType() == "FBA" || channel.ChannelType() == "IG" {
		return h.parseFacebookInstagramPayload(channel, payload, r, in, clog)
	}
	return h.parseWhatsAppPayload(channel, payload, r, in, clog)
}

// parseWhatsAppPayload turns a notification into the set of things it contained, without writing any of them -
// which is what lets the whole batch be written together, and keeps a failure part way through it from leaving
// a response that describes more than we actually did. It still does I/O, since resolving a message's media
// means asking the provider for its URL, but it makes no changes.
func (h *handler) parseWhatsAppPayload(channel *models.Channel, payload *Notifications, r *http.Request, in *channels.Received, clog *models.ChannelLog) error {

	token := h.Runtime().Config.WhatsappAdminSystemUserToken

	seenMsgIDs := make(map[string]bool, 2)
	contactNames := make(map[string]string)

	// for each entry
	for _, entry := range payload.Entry {
		if len(entry.Changes) == 0 {
			continue
		}

		for _, change := range entry.Changes {

			// contacts are keyed by both identifiers they can carry, as a message from a user with a username may
			// only reference them by their user_id
			for _, contact := range change.Value.Contacts {
				if contact.WaID != "" {
					contactNames[contact.WaID] = contact.Profile.Name
				}
				if contact.UserID != "" {
					contactNames[contact.UserID] = contact.Profile.Name
				}
			}

			for _, waMsg := range change.Value.Messages {
				if seenMsgIDs[waMsg.ID] {
					continue
				}

				if waMsg.GroupID != "" {
					in.Ignored("ignoring group message")
					continue
				}

				date, urn, text, mediaURL, mediaID, err, finalErr := waMsg.ExtractData(clog)
				if finalErr != nil {
					return finalErr
				}

				if err != nil {
					channels.LogRequestError(r, channel, err)
					continue
				}

				if mediaID != "" && mediaURL == "" {
					mediaURL, err = h.resolveMediaURL(mediaID, token, clog)
					// we had an error downloading media
					if err != nil {
						channels.LogRequestError(r, channel, err)
					}
				}

				// create our message
				event := models.NewIncomingMsg(channel, urn, text, waMsg.ID, clog).WithReceivedOn(date).WithContactName(contactNames[waMsg.Identifier()])

				if mediaURL != "" {
					event.WithAttachment(mediaURL)
				}

				if payload := waMsg.ExtractPayload(); payload != nil {
					event.WithPayload(payload)
				} else if waMsg.Interactive.Type == "nfm_reply" && waMsg.Interactive.NFMReply.ResponseJSON != "" {
					channels.LogRequestError(r, channel, errors.New("nfm_reply response_json is not a valid JSON object"))
				}

				// if we have a user_id, add it as a secondary whatsapp URN (unless it's already the primary URN)
				if waMsg.FromUserID != "" {
					userIDURN, urnErr := urns.New(urns.WhatsApp, waMsg.FromUserID)
					if urnErr == nil {
						if userIDURN != urn {
							event.WithNewURN(userIDURN, models.NewURNAppend)
						}
					} else {
						channels.LogRequestError(r, channel, fmt.Errorf("invalid user_id for whatsapp URN: %w", urnErr))
					}
				}

				in.Msg(event)
				seenMsgIDs[waMsg.ID] = true
			}

			for _, status := range change.Value.Statuses {

				msgStatus, found := whatsapp.StatusMapping[status.Status]
				if !found {
					if whatsapp.IgnoreStatuses[status.Status] {
						in.Ignored(fmt.Sprintf("ignoring status: %s", status.Status))
					} else {
						channels.LogRequestError(r, channel, fmt.Errorf("unknown status: %s", status.Status))
						in.Ignored(fmt.Sprintf("unknown status: %s", status.Status))
					}
					continue
				}

				for _, statusError := range status.Errors {
					statusError.ErrorChannelLog(clog)
				}

				in.Status(models.NewStatusUpdateByExternalID(channel, status.ID, msgStatus, clog))
			}

			for _, chError := range change.Value.Errors {
				chError.ErrorChannelLog(clog)
			}

		}

	}

	return nil
}

// parseFacebookInstagramPayload turns a notification into the set of things it contained, without writing any
// of them. It matters most for this payload shape, which carries channel events - those have no duplicate
// detection of their own, so a parse failure part way through used to leave the events before it written and
// then ask the provider to resend the whole batch, writing them a second time.
func (h *handler) parseFacebookInstagramPayload(channel *models.Channel, payload *Notifications, r *http.Request, in *channels.Received, clog *models.ChannelLog) error {

	var err error

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
				return errors.New("invalid instagram id")
			}
		} else {
			urn, err = urns.New(urns.Facebook, sender)
			if err != nil {
				return errors.New("invalid facebook id")
			}
		}

		if msg.OptIn != nil {
			if msg.OptIn.Type == "notification_messages" {
				in.Ignored("ignoring optin")
			} else {
				// this is an optin from the checkbox plugin, treat it as a referral
				event := models.NewChannelEvent(channel, models.EventTypeReferral, urn, clog).
					WithOccurredOn(date).
					WithExtra(map[string]string{referrerIDKey: msg.OptIn.Ref})

				in.Event(event)
			}

		} else if msg.Postback != nil {
			// by default postbacks are treated as new conversations, unless we have referral information
			eventType := models.EventTypeNewConversation
			if msg.Postback.Referral.Ref != "" {
				eventType = models.EventTypeReferral
			}
			event := models.NewChannelEvent(channel, eventType, urn, clog).WithOccurredOn(date)

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

			in.Event(event)

		} else if msg.Referral != nil {
			// this is an incoming referral
			event := models.NewChannelEvent(channel, models.EventTypeReferral, urn, clog).WithOccurredOn(date)

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

			in.Event(event)

		} else if msg.Message != nil {
			// this is an incoming message
			if seenMsgIDs[msg.Message.MID] {
				continue
			}

			// ignore echos
			if msg.Message.IsEcho {
				in.Ignored("ignoring echo")
				continue
			}

			if msg.Message.IsDeleted {
				in.DeletedMsg(msg.Message.MID)
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
					in.Ignored("ignoring story_mention")
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
			event := models.NewIncomingMsg(channel, urn, text, msg.Message.MID, clog).WithReceivedOn(date)

			// add any attachment URL found
			for _, attURL := range attachmentURLs {
				event.WithAttachment(attURL)
			}

			in.Msg(event)
			seenMsgIDs[msg.Message.MID] = true

		} else if msg.Delivery != nil {
			// this is a delivery report
			for _, mid := range msg.Delivery.MIDs {
				in.Status(models.NewStatusUpdateByExternalID(channel, mid, models.MsgStatusDelivered, clog))
			}

		} else {
			in.Ignored("ignoring unknown entry type")
		}
	}

	return nil
}

func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	if msg.Channel().ChannelType() == "FBA" || msg.Channel().ChannelType() == "IG" {
		return h.sendFacebookInstagramMsg(ctx, msg, res, clog)
	} else if msg.Channel().ChannelType() == "WAC" {
		return h.sendWhatsAppMsg(ctx, msg, res, clog)
	}

	return fmt.Errorf("unssuported channel type")
}

func (h *handler) sendFacebookInstagramMsg(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	// can't do anything without an access token
	accessToken := msg.Channel().StringConfigForKey(models.ConfigAuthToken, "")
	if accessToken == "" {
		return channels.ErrChannelConfig
	}

	isHuman := msg.Origin() == models.MsgOriginChat || msg.Origin() == models.MsgOriginTicket
	payload := &messenger.SendRequest{}

	// build our recipient
	if msg.URNAuth() != "" {
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
		if part.Type == handlers.MsgPartTypeAttachment {
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
			qrs := handlers.FilterQuickRepliesByType(msg.QuickReplies(), models.QuickReplyTypeText)
			for _, qr := range qrs {
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
		if err := handlers.ErrorFromResponse(resp, err); err != nil {
			return err
		}

		respPayload := &messenger.SendResponse{}
		err = json.Unmarshal(respBody, respPayload)
		if err != nil {
			return channels.ErrResponseUnparseable
		}

		if respPayload.Error.Code != 0 {
			return channels.ErrFailedWithReason(strconv.Itoa(respPayload.Error.Code), respPayload.Error.Message)
		}

		if respPayload.ExternalID == "" {
			return channels.ErrResponseUnexpected
		}

		res.AddExternalID(respPayload.ExternalID)
	}

	return nil
}

func (h *handler) sendWhatsAppMsg(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	// can't do anything without an access token
	accessToken := h.Runtime().Config.WhatsappAdminSystemUserToken

	base, _ := url.Parse(graphURL)
	path, _ := url.Parse(fmt.Sprintf("/%s/messages", msg.Channel().Address()))
	wacPhoneURL := base.ResolveReference(path)

	requestPayloads, err := whatsapp.GetMsgPayloads(msg, maxMsgLengthWAC, clog)
	if err != nil {
		return err
	}

	var userID string
	for _, payload := range requestPayloads {
		respUserID, err := h.requestWAC(payload, accessToken, res, wacPhoneURL, clog)
		if err != nil {
			return err
		}
		if respUserID != "" {
			userID = respUserID
		}
	}

	// if we got a user_id in the response, set it as a new URN on the send result so that send completion
	// can queue a contact_changed task to append it to the contact (unless it is the URN we sent to)
	if userID != "" {
		userIDURN, err := urns.New(urns.WhatsApp, userID)
		if err != nil {
			clog.RawError(fmt.Errorf("unable to make whatsapp URN from user_id %s: %w", userID, err))
		} else if userIDURN != msg.URN() {
			res.SetNewURN(userIDURN)
		}
	}

	return nil
}

// WhatsApp displays typing indicators for up to 25 seconds or until a reply is sent. Messenger and
// Instagram display them for up to 20 seconds and are the rare platforms with an explicit off action.
var sendableEvents = map[models.ChannelType]map[string]time.Duration{
	"FBA": {events.TypeTypingStarted: 15 * time.Second, events.TypeTypingStopped: 0},
	"IG":  {events.TypeTypingStarted: 15 * time.Second, events.TypeTypingStopped: 0},
	"WAC": {events.TypeTypingStarted: 20 * time.Second},
}

// SendableEvents declares support for typing indicators
func (h *handler) SendableEvents(*models.Channel) map[string]time.Duration {
	return sendableEvents[h.ChannelType()]
}

func (h *handler) SendEvent(ctx context.Context, ch *models.Channel, event events.Event, clog *models.ChannelLog) error {
	if h.ChannelType() == "FBA" || h.ChannelType() == "IG" {
		return h.sendFacebookInstagramEvent(ctx, ch, event, clog)
	} else if h.ChannelType() == "WAC" {
		return h.sendWhatsAppEvent(ctx, ch, event, clog)
	}

	return fmt.Errorf("unsupported channel type")
}

// Sends typing started/stopped events as typing_on/typing_off sender actions.
// See https://developers.facebook.com/docs/messenger-platform/send-messages/sender-actions
func (h *handler) sendFacebookInstagramEvent(ctx context.Context, ch *models.Channel, event events.Event, clog *models.ChannelLog) error {
	var urn urns.URN
	var action string
	switch typed := event.(type) {
	case *events.TypingStarted:
		urn, action = typed.URN, "typing_on"
	case *events.TypingStopped:
		urn, action = typed.URN, "typing_off"
	default:
		return fmt.Errorf("unsupported event type: %s", event.Type())
	}

	accessToken := ch.StringConfigForKey(models.ConfigAuthToken, "")
	if accessToken == "" {
		return channels.ErrChannelConfig
	}

	// sender action requests can only contain the recipient and the action
	payload := &struct {
		Recipient struct {
			ID string `json:"id"`
		} `json:"recipient"`
		SenderAction string `json:"sender_action"`
	}{SenderAction: action}
	payload.Recipient.ID = urn.Path()

	msgURL, _ := url.Parse(sendURL)
	query := url.Values{}
	query.Set("access_token", accessToken)
	msgURL.RawQuery = query.Encode()

	req, err := http.NewRequest(http.MethodPost, msgURL.String(), bytes.NewReader(jsonx.MustMarshal(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 == 5 {
		return channels.ErrConnectionFailed
	}

	if handlers.IsThrottled(resp) {
		return channels.ErrConnectionThrottled
	}

	respPayload := &messenger.SendResponse{}
	if err := json.Unmarshal(respBody, respPayload); err != nil || resp.StatusCode/100 != 2 || respPayload.Error.Code != 0 {
		return channels.ErrResponseStatus
	}

	return nil
}

// Sends a typing started event as a typing indicator, which WhatsApp implements as marking
// the referenced incoming message as read with a typing_indicator field - so it also marks messages as
// read, which is acceptable because we only send one when a reply is being composed.
// See https://developers.facebook.com/docs/whatsapp/cloud-api/typing-indicators
func (h *handler) sendWhatsAppEvent(ctx context.Context, ch *models.Channel, event events.Event, clog *models.ChannelLog) error {
	typing, ok := event.(*events.TypingStarted)
	if !ok {
		return fmt.Errorf("unsupported event type: %s", event.Type())
	}
	if typing.MsgExternalID == "" {
		return fmt.Errorf("%s event requires msg_external_id", event.Type())
	}

	payload := whatsapp.NewTypingRequest(typing.MsgExternalID)

	base, _ := url.Parse(graphURL)
	path, _ := url.Parse(fmt.Sprintf("/%s/messages", ch.Address()))

	req, err := http.NewRequest(http.MethodPost, base.ResolveReference(path).String(), bytes.NewReader(jsonx.MustMarshal(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", h.Runtime().Config.WhatsappAdminSystemUserToken))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 == 5 {
		return channels.ErrConnectionFailed
	}

	if handlers.IsThrottled(resp) {
		return channels.ErrConnectionThrottled
	}

	response := &struct {
		Success bool `json:"success"`
	}{}
	if err := json.Unmarshal(respBody, response); err != nil || resp.StatusCode/100 != 2 || !response.Success {
		return channels.ErrResponseStatus
	}

	return nil
}

func (h *handler) requestWAC(payload whatsapp.SendRequest, accessToken string, res *channels.SendResult, wacPhoneURL *url.URL, clog *models.ChannelLog) (string, error) {
	jsonBody := jsonx.MustMarshal(payload)

	req, err := http.NewRequest(http.MethodPost, wacPhoneURL.String(), bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 == 5 {
		return "", channels.ErrConnectionFailed
	}
	// Graph API rate limiting is reported as a 429, sometimes with an error code we don't know about
	if handlers.IsThrottled(resp) {
		return "", channels.ErrConnectionThrottled
	}

	respPayload, err := whatsapp.ParseSendResponse(respBody)
	if err != nil {
		return "", err
	}

	if id := respPayload.ExternalID(); id != "" {
		res.AddExternalID(id)
	}
	return respPayload.UserID(), nil
}

// DescribeURN looks up URN metadata for new contacts
func (h *handler) DescribeURN(ctx context.Context, channel *models.Channel, urn urns.URN, clog *models.ChannelLog) (map[string]string, error) {
	if channel.ChannelType() == "WAC" {
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
	appSecret := h.Runtime().Config.FacebookApplicationSecret

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
func (h *handler) BuildAttachmentRequest(ctx context.Context, channel *models.Channel, attachmentURL string, clog *models.ChannelLog) (*http.Request, error) {
	token := h.Runtime().Config.WhatsappAdminSystemUserToken
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

var _ channels.AttachmentRequestBuilder = (*handler)(nil)

func parseTimestamp(ts int64) time.Time {

	if ts >= 1_000_000_000_000 {
		return time.Unix(0, ts*1000000).UTC()
	}

	// sometimes Facebook sends timestamps in seconds rather than milliseconds
	return time.Unix(ts, 0).UTC()
}
