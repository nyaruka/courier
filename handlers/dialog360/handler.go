package dialog360

import (
	"bytes"
	"context"
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

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(r *channels.Routes) error {
	r.Add(h, http.MethodPost, "receive", models.ChannelLogTypeMultiReceive, handlers.JSONPayload(h, h.receiveEvent))
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

// receiveEvent is our HTTP handler function for incoming messages and status updates
func (h *handler) receiveEvent(ctx context.Context, channel *models.Channel, w http.ResponseWriter, r *http.Request, payload *Notifications, clog *models.ChannelLog) ([]channels.Event, error) {

	// is not a 'whatsapp_business_account' object? ignore it
	if payload.Object != "whatsapp_business_account" {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring request")
	}

	// no entries? ignore this request
	if len(payload.Entry) == 0 {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring request, no entries")
	}

	var events []channels.Event
	var data []any

	events, data, err := h.processWhatsAppPayload(ctx, channel, payload, w, r, clog)
	if err != nil {
		return nil, err
	}

	return events, channels.WriteDataResponse(w, http.StatusOK, "Events Handled", data)
}

func (h *handler) processWhatsAppPayload(ctx context.Context, channel *models.Channel, payload *Notifications, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, []any, error) {
	// the list of events we deal with
	events := make([]channels.Event, 0, 2)

	// the list of data we will return in our response
	data := make([]any, 0, 2)

	seenMsgIDs := make(map[string]bool)
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
					data = append(data, channels.NewInfoData("ignoring group message"))
					continue
				}

				date, urn, text, mediaURL, mediaID, err, finalErr := waMsg.ExtractData(clog)
				if finalErr != nil {
					return nil, nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, finalErr.Error())
				}

				if err != nil {
					channels.LogRequestError(r, channel, err)
					continue
				}

				if mediaID != "" && mediaURL == "" {
					mediaURL, err = h.resolveMediaURL(channel, mediaID, clog)
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

				err = models.WriteMsg(ctx, h.Runtime(), event, clog)
				if err != nil {
					return nil, nil, err
				}

				events = append(events, event)
				data = append(data, channels.NewMsgReceiveData(event))
				seenMsgIDs[waMsg.ID] = true
			}

			for _, status := range change.Value.Statuses {

				msgStatus, found := whatsapp.StatusMapping[status.Status]
				if !found {
					if whatsapp.IgnoreStatuses[status.Status] {
						data = append(data, channels.NewInfoData(fmt.Sprintf("ignoring status: %s", status.Status)))
					} else {
						handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, fmt.Sprintf("unknown status: %s", status.Status))
					}
					continue
				}

				for _, statusError := range status.Errors {
					clog.Error(models.ErrorExternal(strconv.Itoa(statusError.Code), statusError.Title))
				}

				event := models.NewStatusUpdateByExternalID(channel, status.ID, msgStatus, clog)
				err := models.WriteStatusUpdate(ctx, h.Runtime(), event)
				if err != nil {
					return nil, nil, err
				}

				events = append(events, event)
				data = append(data, channels.NewStatusData(event))

			}

			for _, chError := range change.Value.Errors {
				clog.Error(models.ErrorExternal(strconv.Itoa(chError.Code), chError.Title))
			}

		}

	}
	return events, data, nil
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

	// if we got a user_id in the response, set it as a new URN on the send result so the backend
	// can queue a contact_changed task to append it to the contact (unless it's the URN we sent to)
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
