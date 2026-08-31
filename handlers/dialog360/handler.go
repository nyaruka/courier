package dialog360

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
	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/courier/v26/handlers/meta/whatsapp"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/core/events"
)

const (
	d3AuthorizationKey = "D360-API-KEY"
)

var (
	// max for the body
	maxMsgLength = 4096
)

func init() {
	channels.RegisterHandler(newWAHandler(models.ChannelType("D3C"), "360Dialog"))
}

type handler struct {
	handlers.BaseHandler
}

func newWAHandler(channelType models.ChannelType, name string) channels.Handler {
	return &handler{handlers.NewBaseHandler(channelType, name)}
}

// Initialize registers the routes this handler serves
func (h *handler) Initialize(r *channels.Routes) error {
	r.AddReceive(h, http.MethodPost, "receive", channels.ReceiveKindAny, handlers.JSONPayload(h.receiveEvent))
	return nil
}

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
		ID      string            `json:"id"`
		Time    int64             `json:"time"`
		Changes []whatsapp.Change `json:"changes"` // used by WhatsApp
	} `json:"entry"`
}

// receiveEvent is our receive function for incoming messages and status updates
func (h *handler) receiveEvent(ctx context.Context, channel *models.Channel, r *http.Request, payload *Notifications, in *channels.Received, clog *models.ChannelLog) error {
	// is not a 'whatsapp_business_account' object? ignore it
	if payload.Object != "whatsapp_business_account" {
		return channels.Ignore("ignoring request")
	}

	// no entries? ignore this request
	if len(payload.Entry) == 0 {
		return channels.Ignore("ignoring request, no entries")
	}

	// a failure here is in the payload itself, so asking for it again wouldn't get any further. The seam writes
	// whatever was parsed ahead of it rather than dropping it.
	return h.parseWhatsAppPayload(channel, payload, r, in, clog)
}

// parseWhatsAppPayload hands the notification's changes to the shared WhatsApp parser, which fills in the batch.
// Media is resolved with the channel's own token against the provider's base URL.
//
// A malformed payload is answered as ignored rather than as an error: unlike the Cloud API handler, this one
// doesn't force a 200 on error responses, and 360dialog would retry a payload that can't parse any better the
// second time.
func (h *handler) parseWhatsAppPayload(channel *models.Channel, payload *Notifications, r *http.Request, in *channels.Received, clog *models.ChannelLog) error {
	resolveMedia := func(mediaID string) (string, error) { return h.resolveMediaURL(channel, mediaID, clog) }

	var changes []whatsapp.Change
	for _, entry := range payload.Entry {
		changes = append(changes, entry.Changes...)
	}

	if err := whatsapp.ParseChanges(channel, changes, resolveMedia, r, in, clog); err != nil {
		return channels.Ignore("%s", err.Error())
	}
	return nil
}

// BuildAttachmentRequest to download media for message attachment with Bearer token set
func (h *handler) BuildAttachmentRequest(ctx context.Context, channel *models.Channel, attachmentURL string, clog *models.ChannelLog) (*http.Request, error) {
	token := channel.StringConfigForKey(models.ConfigAuthToken, "")
	if token == "" {
		return nil, fmt.Errorf("missing token for D3C channel")
	}

	// set the access token as the authorization header
	req, _ := http.NewRequest(http.MethodGet, attachmentURL, nil)
	req.Header.Set(d3AuthorizationKey, token)
	return req, nil
}

var _ channels.AttachmentRequestBuilder = (*handler)(nil)

func (h *handler) resolveMediaURL(channel *models.Channel, mediaID string, clog *models.ChannelLog) (string, error) {
	// sometimes WA will send an attachment with status=undownloaded and no ID
	if mediaID == "" {
		return "", nil
	}

	token := channel.StringConfigForKey(models.ConfigAuthToken, "")
	if token == "" {
		return "", fmt.Errorf("missing token for D3C channel")
	}

	urlStr := channel.StringConfigForKey(models.ConfigBaseURL, "")
	url, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("invalid base url set for D3C channel: %s", err)
	}

	mediaPath, _ := url.Parse(mediaID)
	mediaURL := url.ResolveReference(mediaPath).String()

	req, _ := http.NewRequest(http.MethodGet, mediaURL, nil)
	req.Header.Set(d3AuthorizationKey, token)

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("failed to request media URL for D3C channel: %s", err)
	}

	fbFileURL, err := jsonparser.GetString(respBody, "url")
	if err != nil {
		return "", fmt.Errorf("missing url field in response for D3C media: %s", err)
	}

	fileURL := strings.ReplaceAll(fbFileURL, "https://lookaside.fbsbx.com", urlStr)

	return fileURL, nil
}

func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	accessToken := msg.Channel().StringConfigForKey(models.ConfigAuthToken, "")
	urlStr := msg.Channel().StringConfigForKey(models.ConfigBaseURL, "")
	url, err := url.Parse(urlStr)
	if accessToken == "" || err != nil {
		return channels.ErrChannelConfig
	}
	sendURL, _ := url.Parse("/messages")

	requestPayloads, err := whatsapp.GetMsgPayloads(msg, maxMsgLength, clog)
	if err != nil {
		return err
	}

	var userID string
	for _, payload := range requestPayloads {
		respUserID, err := h.requestD3C(payload, accessToken, res, sendURL, clog)
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

// WhatsApp displays typing indicators for up to 25 seconds or until a reply is sent
var sendableEvents = map[string]time.Duration{events.TypeTypingStarted: 20 * time.Second}

// SendableEvents declares support for typing indicators
func (h *handler) SendableEvents(*models.Channel) map[string]time.Duration {
	return sendableEvents
}

// SendEvent sends a typing started event as a typing indicator, which WhatsApp implements as marking
// the referenced incoming message as read with a typing_indicator field - so it also marks messages as
// read, which is acceptable because we only send one when a reply is being composed.
// See https://developers.facebook.com/docs/whatsapp/cloud-api/typing-indicators
func (h *handler) SendEvent(ctx context.Context, ch *models.Channel, event events.Event, clog *models.ChannelLog) error {
	typing, ok := event.(*events.TypingStarted)
	if !ok {
		return fmt.Errorf("unsupported event type: %s", event.Type())
	}
	if typing.MsgExternalID == "" {
		return fmt.Errorf("%s event requires msg_external_id", event.Type())
	}

	accessToken := ch.StringConfigForKey(models.ConfigAuthToken, "")
	baseURL, err := url.Parse(ch.StringConfigForKey(models.ConfigBaseURL, ""))
	if accessToken == "" || err != nil {
		return channels.ErrChannelConfig
	}
	sendURL, _ := baseURL.Parse("/messages")

	payload := whatsapp.NewTypingRequest(typing.MsgExternalID)

	req, err := http.NewRequest(http.MethodPost, sendURL.String(), bytes.NewReader(jsonx.MustMarshal(payload)))
	if err != nil {
		return err
	}
	req.Header.Set(d3AuthorizationKey, accessToken)
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

func (h *handler) requestD3C(payload whatsapp.SendRequest, accessToken string, res *channels.SendResult, wacPhoneURL *url.URL, clog *models.ChannelLog) (string, error) {
	jsonBody := jsonx.MustMarshal(payload)

	req, err := http.NewRequest(http.MethodPost, wacPhoneURL.String(), bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}

	req.Header.Set(d3AuthorizationKey, accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 == 5 {
		return "", channels.ErrConnectionFailed
	}
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
