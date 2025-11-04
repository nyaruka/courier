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

const configTransliteration = "transliteration"

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

type v3InboundPayload struct {
	Results             []v3InboundMessage `validate:"required" json:"results"`
	MessageCount        int                `json:"messageCount"`
	PendingMessageCount int                `json:"pendingMessageCount"`
}

type v3InboundMessage struct {
	ApplicationID       string                `json:"applicationId,omitempty"`
	MessageID           string                `json:"messageId"`
	From                string                `json:"from" validate:"required"`
	To                  string                `json:"to"`
	Text                string                `json:"text"` // This will be present for SMS
	CleanText           string                `json:"cleanText,omitempty"`
	Keyword             string                `json:"keyword,omitempty"`
	ReceivedAt          string                `json:"receivedAt"`
	SmsCount            int                   `json:"smsCount"`
	Price               v3InboundPrice        `json:"price"`
	CallbackData        string                `json:"callbackData,omitempty"`
	EntityID            string                `json:"entityId,omitempty"`
	CampaignReferenceID string                `json:"campaignReferenceId,omitempty"`
	Message             []v2MMSInboundSegment `json:"message"` // This will be present for MMS
}

type v2MMSInboundSegment struct {
	ContentType string `json:"contentType"`
	URL         string `json:"url,omitempty"`
	Value       string `json:"value,omitempty"`
}

type v3InboundPrice struct {
	PricePerMessage float64 `json:"pricePerMessage"`
	Currency        string  `json:"currency"`
}

// receiveMessage is our HTTP handler function for incoming messages (both SMS and MMS)
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *v3InboundPayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	if payload.MessageCount == 0 {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring request, no message")
	}

	msgs := []courier.MsgIn{}
	for _, infobipMessage := range payload.Results {
		messageID := infobipMessage.MessageID
		dateString := infobipMessage.ReceivedAt

		// create our URN
		urn, err := urns.ParsePhone(infobipMessage.From, channel.Country(), true, false)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}

		// Check if this is an MMS message by looking at the Message field
		isMMS := len(infobipMessage.Message) > 0

		var text string
		var attachments []string

		if isMMS {
			// Handle MMS message
			var textParts []string
			for _, segment := range infobipMessage.Message {
				if segment.ContentType == "text/plain" {
					textParts = append(textParts, segment.Value)
				} else if segment.URL != "" {
					attachment := fmt.Sprintf("%s:%s", segment.ContentType, segment.URL)
					attachments = append(attachments, attachment)
				}
			}
			text = strings.Join(textParts, "\n")
		} else {
			// Handle SMS message
			text = infobipMessage.Text
		}

		// Skip if no text and no attachments
		if text == "" && len(attachments) == 0 {
			continue
		}

		// Parse date if provided
		date := time.Now()
		if dateString != "" {
			// The format for ReceivedAt is "yyyy-MM-dd'T'HH:mm:ss.SSS+0000"
			// It is not RFC3339 Compliant
			// In Go 1.26 revisit changing this with json/v2 with custom datetime support

			date, err = time.Parse("2006-01-02T15:04:05.000-0700", dateString)
			if err != nil {
				return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
			}
		}

		// build our message
		msg := h.Backend().NewIncomingMsg(ctx, channel, urn, text, messageID, clog).WithReceivedOn(date)
		for _, attachment := range attachments {
			msg = msg.WithAttachment(attachment)
		}
		msgs = append(msgs, msg)
	}

	if len(msgs) == 0 {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring request, no message")
	}

	return handlers.WriteMsgsAndResponse(ctx, h, msgs, w, r, clog)
}

type v3OutboundPayload struct {
	Messages []v3OutboundMessage `json:"messages"`
}

type v3OutboundMessage struct {
	From         string                  `json:"from"`
	Destinations []v3OutboundDestination `json:"destinations"`
	Content      v3OutboundContent       `json:"content"`
	Webhooks     *v3OutboundWebhooks     `json:"webhooks,omitempty"`
}

type v3OutboundDestination struct {
	To        string `json:"to"`
	MessageID string `json:"messageId,omitempty"`
}

type v3OutboundContent struct {
	Text            string `json:"text,omitempty"`
	Transliteration string `json:"transliteration,omitempty"`
}

type v3OutboundWebhooks struct {
	Delivery v3OutboundDelivery `json:"delivery"`
}

type v3OutboundDelivery struct {
	URL                string `json:"url"`
	IntermediateReport bool   `json:"intermediateReport"`
	ContentType        string `json:"contentType"`
}

type v2MMSOutboundPayload struct {
	Messages []v2MMSOutboundMessage `json:"messages"`
}

type v2MMSOutboundMessage struct {
	Sender       string                     `json:"sender"`
	Destinations []v2MMSOutboundDestination `json:"destinations"`
	Content      v2MMSOutboundContent       `json:"content"`
	Webhooks     *v3OutboundWebhooks        `json:"webhooks,omitempty"` // Reusing SMS webhooks struct
}

type v2MMSOutboundDestination struct {
	To        string `json:"to"`
	MessageID string `json:"messageId,omitempty"`
}

type v2MMSOutboundContent struct {
	Title           string                `json:"title"`
	MessageSegments []v2MMSMessageSegment `json:"messageSegments"`
}

type v2MMSMessageSegment struct {
	Type        string `json:"type"` // "TEXT" or "LINK" for media
	ContentID   string `json:"contentId,omitempty"`
	Text        string `json:"text,omitempty"`
	URL         string `json:"url,omitempty"`         // Deprecated: use ContentURL for media
	ContentURL  string `json:"contentUrl,omitempty"`  // Required for LINK type
	ContentType string `json:"contentType,omitempty"` // Required for LINK type (e.g., image/jpeg)
}

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	apiKey := msg.Channel().StringConfigForKey(models.ConfigAPIKey, "")
	username := msg.Channel().StringConfigForKey(models.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(models.ConfigPassword, "")

	if apiKey == "" && (username == "" || password == "") {
		return courier.ErrChannelConfig
	}

	baseURL := msg.Channel().StringConfigForKey(models.ConfigBaseURL, "https://api.infobip.com")
	callbackDomain := msg.Channel().CallbackDomain(h.Server().Config().Domain)
	statusURL := fmt.Sprintf("https://%s%s%s/delivered", callbackDomain, "/c/ib/", msg.Channel().UUID())

	var requestBody *bytes.Buffer
	var req *http.Request
	var err error

	if len(msg.Attachments()) > 0 {
		// Handle MMS message
		mmsPayload := v2MMSOutboundPayload{
			Messages: []v2MMSOutboundMessage{
				{
					Sender: msg.Channel().Address(),
					Destinations: []v2MMSOutboundDestination{
						{
							To:        strings.TrimLeft(msg.URN().Path(), "+"),
							MessageID: string(msg.UUID()),
						},
					},
					Content: v2MMSOutboundContent{
						Title: "",
						MessageSegments: []v2MMSMessageSegment{
							{
								Type: "TEXT",
								Text: msg.Text(),
							},
						},
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

		for _, attachmentStr := range msg.Attachments() {
			mimeType, url := handlers.SplitAttachment(attachmentStr)
			if mimeType == "" || url == "" {
				handlers.WriteAndLogRequestError(ctx, h, msg.Channel(), nil, nil, fmt.Errorf("ignoring invalid attachment: %s", attachmentStr))
				continue
			}

			mmsPayload.Messages[0].Content.MessageSegments = append(mmsPayload.Messages[0].Content.MessageSegments, v2MMSMessageSegment{
				Type:        "LINK", // Media content must use LINK type per Infobip API
				ContentURL:  url,
				ContentType: mimeType,
			})
		}

		requestBody = &bytes.Buffer{}
		err = json.NewEncoder(requestBody).Encode(mmsPayload)
		if err != nil {
			return err
		}

		mmsSendURL := fmt.Sprintf("%s/mms/2/messages", baseURL)
		req, err = http.NewRequest(http.MethodPost, mmsSendURL, requestBody)
		if err != nil {
			return err
		}

	} else {
		// Handle SMS message

		transliteration := msg.Channel().StringConfigForKey(configTransliteration, "")

		smsPayload := v3OutboundPayload{
			Messages: []v3OutboundMessage{
				{
					From: msg.Channel().Address(),
					Destinations: []v3OutboundDestination{
						{
							To:        strings.TrimLeft(msg.URN().Path(), "+"),
							MessageID: string(msg.UUID()),
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

		requestBody = &bytes.Buffer{}
		err = json.NewEncoder(requestBody).Encode(smsPayload)
		if err != nil {
			return err
		}

		smsSendURL := fmt.Sprintf("%s/sms/3/messages", baseURL)
		req, err = http.NewRequest(http.MethodPost, smsSendURL, requestBody)
		if err != nil {
			return err
		}
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
	if apiKey := ch.StringConfigForKey(models.ConfigAPIKey, ""); apiKey != "" {
		redacted = append(redacted, "App "+apiKey)
	}
	if username := ch.StringConfigForKey(models.ConfigUsername, ""); username != "" {
		redacted = append(redacted, httpx.BasicAuth(username, ch.StringConfigForKey(models.ConfigPassword, "")))
	}
	return redacted
}
