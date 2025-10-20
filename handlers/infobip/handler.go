package infobip

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

var sendURL = "https://api.infobip.com/sms/3/messages"

const (
	configTransliteration = "transliteration"
	configAPIKey          = "api_key"
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(models.ChannelType("IB"), "Infobip")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, handlers.JSONPayload(h, h.receiveMessage))
	s.AddHandlerRoute(h, http.MethodPost, "delivered", courier.ChannelLogTypeMsgStatus, handlers.JSONPayload(h, h.statusMessage))
	return nil
}

var statusMapping = map[string]models.MsgStatus{
	"PENDING":       models.MsgStatusSent,
	"EXPIRED":       models.MsgStatusSent,
	"DELIVERED":     models.MsgStatusDelivered,
	"REJECTED":      models.MsgStatusFailed,
	"UNDELIVERABLE": models.MsgStatusFailed,
}

type statusPayload struct {
	Results []ibStatus `validate:"required" json:"results"`
}
type ibStatus struct {
	MessageID string `validate:"required" json:"messageId"`
	Status    struct {
		GroupName string `validate:"required" json:"groupName"`
	} `validate:"required" json:"status"`
}

// statusMessage is our HTTP handler function for status updates
func (h *handler) statusMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *statusPayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	data := make([]any, len(payload.Results))
	statuses := make([]courier.Event, len(payload.Results))
	for _, s := range payload.Results {
		msgStatus, found := statusMapping[s.Status.GroupName]
		if !found {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unknown status '%s', must be one of PENDING, DELIVERED, EXPIRED, REJECTED or UNDELIVERABLE", s.Status.GroupName))
		}

		// write our status
		status := h.Backend().NewStatusUpdateByExternalID(channel, s.MessageID, msgStatus, clog)
		err := h.Backend().WriteStatusUpdate(ctx, status)
		if err != nil {
			return nil, err
		}
		data = append(data, courier.NewStatusData(status))
		statuses = append(statuses, status)
	}

	return statuses, courier.WriteDataResponse(w, http.StatusOK, "statuses handled", data)
}

//	{
//		"results": [
//		  {
//			"messageId": "817790313235066447",
//			"from": "385916242493",
//			"to": "385921004026",
//			"text": "QUIZ Correct answer is Paris",
//			"cleanText": "Correct answer is Paris",
//			"keyword": "QUIZ",
//			"receivedAt": "2016-10-06T09:28:39.220+0000",
//			"smsCount": 1,
//			"price": {
//			  "pricePerMessage": 0,
//			  "currency": "EUR"
//			},
//			"callbackData": "callbackData"
//		  }
//		],
//		"messageCount": 1,
//		"pendingMessageCount": 0
//	}
type v3InboundPayload struct {
	Results             []v3InboundMessage `validate:"required" json:"results"`
	MessageCount        int                `json:"messageCount"`
	PendingMessageCount int                `json:"pendingMessageCount"`
}

type v3InboundMessage struct {
	ApplicationID       string         `json:"applicationId,omitempty"`
	MessageID           string         `json:"messageId"`
	From                string         `json:"from" validate:"required"`
	To                  string         `json:"to"`
	Text                string         `json:"text"`
	CleanText           string         `json:"cleanText,omitempty"`
	Keyword             string         `json:"keyword,omitempty"`
	ReceivedAt          string         `json:"receivedAt"`
	SmsCount            int            `json:"smsCount"`
	Price               v3InboundPrice `json:"price"`
	CallbackData        string         `json:"callbackData,omitempty"`
	EntityID            string         `json:"entityId,omitempty"`
	CampaignReferenceID string         `json:"campaignReferenceId,omitempty"`
}

type v3InboundPrice struct {
	PricePerMessage float64 `json:"pricePerMessage"`
	Currency        string  `json:"currency"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *v3InboundPayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	if payload.MessageCount == 0 {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring request, no message")
	}

	msgs := []courier.MsgIn{}
	for _, infobipMessage := range payload.Results {
		messageID := infobipMessage.MessageID
		text := infobipMessage.Text
		dateString := infobipMessage.ReceivedAt

		if text == "" {
			continue
		}

		date := time.Now()
		var err error
		if dateString != "" {
			// The format for ReceivedAt is "yyyy-MM-dd'T'HH:mm:ss.SSSZ"
			date, err = time.Parse("2006-01-02T15:04:05.000Z0700", dateString)
			if err != nil {
				return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
			}
		}

		// create our URN
		urn, err := urns.ParsePhone(infobipMessage.From, channel.Country(), true, false)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}

		// build our infobipMessage
		msg := h.Backend().NewIncomingMsg(ctx, channel, urn, text, messageID, clog).WithReceivedOn(date)
		msgs = append(msgs, msg)

	}

	if len(msgs) == 0 {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring request, no message")
	}

	return handlers.WriteMsgsAndResponse(ctx, h, msgs, w, r, clog)
}

// v3OutboundPayload represents the request body for Infobip SMS API v3 outbound messages.
type v3OutboundPayload struct {
	Messages []v3OutboundMessage `json:"messages"`
}

// v3OutboundMessage represents a single message in the v3OutboundPayload.
type v3OutboundMessage struct {
	From         string                `json:"from"`
	Destinations []v3OutboundDestination `json:"destinations"`
	Content      v3OutboundContent     `json:"content"`
	Webhooks     *v3OutboundWebhooks   `json:"webhooks,omitempty"`
}

// v3OutboundDestination represents a single destination in a v3OutboundMessage.
type v3OutboundDestination struct {
	To        string `json:"to"`
	MessageID string `json:"messageId,omitempty"`
}

// v3OutboundContent represents the content of a v3OutboundMessage.
type v3OutboundContent struct {
	Text            string `json:"text,omitempty"`
	Transliteration string `json:"transliteration,omitempty"`
}

// v3OutboundWebhooks represents webhook settings for a v3OutboundMessage.
type v3OutboundWebhooks struct {
	Delivery v3OutboundDelivery `json:"delivery"`
}

// v3OutboundDelivery represents delivery report settings for a v3OutboundMessage.
type v3OutboundDelivery struct {
	URL                string `json:"url"`
	IntermediateReport bool   `json:"intermediateReport"`
	ContentType        string `json:"contentType"`
}

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	apiKey := msg.Channel().StringConfigForKey(configAPIKey, "")
	username := msg.Channel().StringConfigForKey(models.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(models.ConfigPassword, "")

	if apiKey == "" && (username == "" || password == "") {
		return courier.ErrChannelConfig
	}

	transliteration := msg.Channel().StringConfigForKey(configTransliteration, "")

	callbackDomain := msg.Channel().CallbackDomain(h.Server().Config().Domain)
	statusURL := fmt.Sprintf("https://%s%s%s/delivered", callbackDomain, "/c/ib/", msg.Channel().UUID())

	ibMsg := v3OutboundPayload{
		Messages: []v3OutboundMessage{
			{
				From: msg.Channel().Address(),
				Destinations: []v3OutboundDestination{
					{
						To:        strings.TrimLeft(msg.URN().Path(), "+"),
						MessageID: msg.ID().String(),
					},
				},
				Content: v3OutboundContent{
					Text:            handlers.GetTextAndAttachments(msg),
					Transliteration: transliteration,
				},
				Webhooks: &v3OutboundWebhooks{
					Delivery: v3OutboundDelivery{
						URL:                statusURL,
						IntermediateReport: true,
						ContentType:        "application/json",
					},
				},
			},
		},
	}

	requestBody := &bytes.Buffer{}
	err := json.NewEncoder(requestBody).Encode(ibMsg)
	if err != nil {
		return err
	}

	// build our request
	req, err := http.NewRequest(http.MethodPost, sendURL, requestBody)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if apiKey != "" {
		req.Header.Set("Authorization", "App "+apiKey)
	} else {
		req.SetBasicAuth(username, password)
	}

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 == 5 {
		return courier.ErrConnectionFailed
	} else if resp.StatusCode/100 != 2 {
		return courier.ErrResponseStatus
	}

	// Infobip v3 API response for sending messages
	// {
	//   "bulkId": "2034072219640523072",
	//   "messages": [
	//     {
	//       "messageId": "2250be2d4219-3af1-78856-aabe-1362af1edfd2",
	//       "status": {
	//         "groupId": 1,
	//         "groupName": "PENDING",
	//         "id": 26,
	//         "name": "PENDING_ACCEPTED",
	//         "description": "Message sent to next instance"
	//       },
	//       "destination": "41793026727",
	//       "details": {
	//         "messageCount": 1
	//       }
	//     }
	//   ]
	// }
	groupID, err := jsonparser.GetInt(respBody, "messages", "[0]", "status", "groupId")
	if err != nil || (groupID != 1 && groupID != 3) {
		return courier.ErrResponseContent
	}

	externalID, err := jsonparser.GetString(respBody, "messages", "[0]", "messageId")
	if err != nil {
		clog.Error(courier.ErrorResponseValueMissing("messageId"))
	} else {
		res.AddExternalID(externalID)
	}

	return nil
}

func (h *handler) RedactValues(ch courier.Channel) []string {
	redacted := []string{}
	if apiKey := ch.StringConfigForKey(configAPIKey, ""); apiKey != "" {
		redacted = append(redacted, "App "+apiKey)
	}
	if username := ch.StringConfigForKey(models.ConfigUsername, ""); username != "" {
		redacted = append(redacted, httpx.BasicAuth(username, ch.StringConfigForKey(models.ConfigPassword, "")))
	}
	return redacted
}
