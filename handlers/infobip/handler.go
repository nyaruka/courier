// API docs: https://www.infobip.com/docs/api/platform/messages-api/sending-message/send-messages-api-message
package infobip

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/gsm7"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

const (
	configTransliteration = "transliteration"
	configApplicationID   = "application_id"
	configEntityID        = "entity_id"

	signatureHeader   = "X-Hub-Signature"
	signaturePrefix   = "SHA256="
	maxWebhookBodyLen = 1024 * 1024
)

func init() {
	channels.RegisterHandler(newHandler)
}

type handler struct {
	handlers.BaseHandler
}

func newHandler(rt *runtime.Runtime, r *channels.Routes) channels.Handler {
	h := &handler{handlers.NewBaseHandler(rt, models.ChannelType("IB"), "Infobip")}

	// the Messages API delivers inbound messages and delivery reports through the same envelope, so both go to receive
	r.AddReceive(h, http.MethodPost, "receive", channels.ReceiveKindAny, handlers.JSONPayload(h.receiveEvents))

	// delivery reports for messages sent before we pointed delivery webhooks at receive
	r.AddReceive(h, http.MethodPost, "delivered", channels.ReceiveKindStatus, handlers.JSONPayload(h.receiveStatus))
	return h
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
	Error json.RawMessage `json:"error"`
}

type ibError struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// logDeliveryError adds the error from a delivery report, if any, to the channel log
func logDeliveryError(raw json.RawMessage, clog *models.ChannelLog) {
	e := &ibError{}
	if len(raw) == 0 || json.Unmarshal(raw, e) != nil || e.ID == 0 || e.Name == "" {
		return
	}
	clog.Error(models.ErrorExternal(e.Name, e.Description))
}

// receiveStatus is our receive function for delivery reports sent to the legacy delivered URL
func (h *handler) receiveStatus(ctx context.Context, channel *models.Channel, r *http.Request, payload *statusPayload, in *channels.Received, clog *models.ChannelLog) error {
	if err := validateSignature(channel, r); err != nil {
		return channels.Unauthenticated(err)
	}

	for _, s := range payload.Results {
		msgStatus, found := statusMapping[s.Status.GroupName]
		if !found {
			return handlers.UnknownStatusError(statusMapping, s.Status.GroupName)
		}

		logDeliveryError(s.Error, clog)
		in.Status(models.NewStatusUpdateByExternalID(channel, s.MessageID, msgStatus, clog))
	}

	return nil
}

// eventsPayload is the envelope of every Messages API webhook, and of legacy SMS/MMS inbound webhooks. Results are
// decoded individually once we know what kind of event each one is.
type eventsPayload struct {
	Results []json.RawMessage `validate:"required" json:"results"`
}

type moEvent struct {
	Event      string      `json:"event"`
	Channel    string      `json:"channel"`
	Sender     string      `json:"sender"`
	MessageID  string      `json:"messageId"`
	ReceivedAt string      `json:"receivedAt"`
	Content    []moContent `json:"content"`
}

type moContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	URL  string `json:"url,omitempty"`
}

type deliveryEvent struct {
	Event     string `json:"event"`
	MessageID string `json:"messageId"`
	Status    struct {
		GroupName string `json:"groupName"`
	} `json:"status"`
	Error json.RawMessage `json:"error"`
}

// legacyInboundMessage is an inbound message in the SMS API (text) or MMS API (message segments) format
type legacyInboundMessage struct {
	MessageID  string                 `json:"messageId"`
	From       string                 `json:"from"`
	Text       string                 `json:"text"`
	ReceivedAt string                 `json:"receivedAt"`
	Message    []legacyInboundSegment `json:"message"`
}

type legacyInboundSegment struct {
	ContentType string `json:"contentType"`
	URL         string `json:"url,omitempty"`
	Value       string `json:"value,omitempty"`
}

// receiveEvents is our receive function for inbound messages and delivery reports
func (h *handler) receiveEvents(ctx context.Context, channel *models.Channel, r *http.Request, payload *eventsPayload, in *channels.Received, clog *models.ChannelLog) error {
	if err := validateSignature(channel, r); err != nil {
		return channels.Unauthenticated(err)
	}

	for _, raw := range payload.Results {
		var envelope struct {
			Event string `json:"event"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return fmt.Errorf("unable to parse result: %w", err)
		}

		switch envelope.Event {
		case "MO":
			in.As(channels.ReceiveKindMsg)

			mo := &moEvent{}
			if err := json.Unmarshal(raw, mo); err != nil {
				return fmt.Errorf("unable to parse MO event: %w", err)
			}
			if err := receiveMO(channel, mo, in, clog); err != nil {
				return err
			}

		case "DELIVERY":
			if in.Kind() != channels.ReceiveKindMsg {
				in.As(channels.ReceiveKindStatus)
			}

			dr := &deliveryEvent{}
			if err := json.Unmarshal(raw, dr); err != nil {
				return fmt.Errorf("unable to parse DELIVERY event: %w", err)
			}

			msgStatus, found := statusMapping[dr.Status.GroupName]
			if !found {
				in.Ignored(fmt.Sprintf("ignoring unknown status '%s'", dr.Status.GroupName))
				continue
			}
			logDeliveryError(dr.Error, clog)
			in.Status(models.NewStatusUpdateByExternalID(channel, dr.MessageID, msgStatus, clog))

		case "":
			in.As(channels.ReceiveKindMsg)

			msg := &legacyInboundMessage{}
			if err := json.Unmarshal(raw, msg); err != nil {
				return fmt.Errorf("unable to parse inbound message: %w", err)
			}
			if err := receiveLegacy(channel, msg, in, clog); err != nil {
				return err
			}

		default:
			in.Ignored(fmt.Sprintf("ignoring %s event", envelope.Event))
		}
	}

	return nil
}

// receiveMO adds a message from a Messages API MO event, which is a list of text and media content items
func receiveMO(channel *models.Channel, mo *moEvent, in *channels.Received, clog *models.ChannelLog) error {
	urn, err := urns.ParsePhone(mo.Sender, channel.Country(), true, false)
	if err != nil {
		return err
	}

	var subject string
	var texts, attachments []string
	for _, c := range mo.Content {
		switch c.Type {
		case "SUBJECT":
			subject = c.Text
		case "TEXT":
			if c.Text != "" {
				texts = append(texts, c.Text)
			}
		case "IMAGE", "AUDIO", "VIDEO", "DOCUMENT", "FILE", "URL":
			if c.URL != "" {
				attachments = append(attachments, c.URL)
			}
			if c.Text != "" {
				texts = append(texts, c.Text)
			}
		}
	}

	if subject != "" {
		texts = append([]string{subject}, texts...)
	}

	return addMsg(channel, urn, strings.Join(texts, "\n"), attachments, mo.MessageID, mo.ReceivedAt, in, clog)
}

// receiveLegacy adds a message from an SMS API or MMS API inbound message
func receiveLegacy(channel *models.Channel, m *legacyInboundMessage, in *channels.Received, clog *models.ChannelLog) error {
	urn, err := urns.ParsePhone(m.From, channel.Country(), true, false)
	if err != nil {
		return err
	}

	text := m.Text
	var attachments []string

	if len(m.Message) > 0 {
		var texts []string
		for _, segment := range m.Message {
			if segment.ContentType == "text/plain" {
				texts = append(texts, segment.Value)
			} else if segment.URL != "" {
				attachments = append(attachments, fmt.Sprintf("%s:%s", segment.ContentType, segment.URL))
			}
		}
		text = strings.Join(texts, "\n")
	}

	return addMsg(channel, urn, text, attachments, m.MessageID, m.ReceivedAt, in, clog)
}

func addMsg(channel *models.Channel, urn urns.URN, text string, attachments []string, externalID, receivedAt string, in *channels.Received, clog *models.ChannelLog) error {
	if text == "" && len(attachments) == 0 {
		return nil
	}

	date := dates.Now()
	if receivedAt != "" {
		// format is "yyyy-MM-dd'T'HH:mm:ss.SSS+0000" which isn't RFC3339
		var err error
		date, err = time.Parse("2006-01-02T15:04:05.000-0700", receivedAt)
		if err != nil {
			return err
		}
	}

	msg := models.NewIncomingMsg(channel, urn, text, externalID, clog).WithReceivedOn(date)
	for _, a := range attachments {
		msg.WithAttachment(a)
	}
	in.Msg(msg)
	return nil
}

// validateSignature checks the X-Hub-Signature header, which is a hex HMAC-SHA256 of the body using the channel
// secret. Channels without a secret aren't checked.
func validateSignature(c *models.Channel, r *http.Request) error {
	secret := c.StringConfigForKey(models.ConfigSecret, "")
	if secret == "" {
		return nil
	}

	header := strings.TrimSpace(r.Header.Get(signatureHeader))
	if header == "" {
		return fmt.Errorf("missing request signature")
	}
	if !strings.HasPrefix(strings.ToUpper(header), signaturePrefix) {
		return fmt.Errorf("invalid request signature format")
	}

	body, err := handlers.ReadBody(r, maxWebhookBodyLen)
	if err != nil {
		return fmt.Errorf("unable to read request body: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	actual := strings.ToLower(header[len(signaturePrefix):])

	if !hmac.Equal([]byte(expected), []byte(actual)) {
		return fmt.Errorf("invalid request signature")
	}
	return nil
}

type mtPayload struct {
	Messages []mtMessage `json:"messages"`
}

type mtMessage struct {
	Channel      string          `json:"channel"`
	Sender       string          `json:"sender"`
	Destinations []mtDestination `json:"destinations"`
	Content      mtContent       `json:"content"`
	Options      *mtOptions      `json:"options,omitempty"`
	Webhooks     *mtWebhooks     `json:"webhooks,omitempty"`
}

type mtDestination struct {
	To        string `json:"to"`
	MessageID string `json:"messageId,omitempty"`
}

type mtContent struct {
	Body mtBody `json:"body"`
}

type mtBody struct {
	Type  string        `json:"type"`
	Text  string        `json:"text,omitempty"`
	Items []mtMixedItem `json:"items,omitempty"`
}

type mtMixedItem struct {
	Text string `json:"text,omitempty"`
	URL  string `json:"url,omitempty"`
}

type mtOptions struct {
	Platform        *mtPlatform `json:"platform,omitempty"`
	Transliteration string      `json:"transliteration,omitempty"`
}

type mtPlatform struct {
	ApplicationID string `json:"applicationId,omitempty"`
	EntityID      string `json:"entityId,omitempty"`
}

type mtWebhooks struct {
	Delivery    mtDelivery `json:"delivery"`
	ContentType string     `json:"contentType"`
}

type mtDelivery struct {
	URL                string `json:"url"`
	IntermediateReport bool   `json:"intermediateReport"`
}

func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	apiKey := msg.Channel().StringConfigForKey(models.ConfigAPIKey, "")
	username := msg.Channel().StringConfigForKey(models.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(models.ConfigPassword, "")

	if apiKey == "" && (username == "" || password == "") {
		return channels.ErrChannelConfig
	}

	baseURL := msg.Channel().StringConfigForKey(models.ConfigBaseURL, "https://api.infobip.com")
	callbackDomain := msg.Channel().CallbackDomain(h.Runtime().Config.Domain)
	statusURL := fmt.Sprintf("https://%s/c/ib/%s/receive", callbackDomain, msg.Channel().UUID())

	mt := mtMessage{
		Channel: "SMS",
		Sender:  msg.Channel().Address(),
		Destinations: []mtDestination{
			{
				To:        strings.TrimLeft(msg.URN().Path(), "+"),
				MessageID: string(msg.UUID()),
			},
		},
		Content:  mtContent{Body: mtBody{Type: "TEXT", Text: msg.Text()}},
		Options:  messageOptions(msg.Channel()),
		Webhooks: &mtWebhooks{Delivery: mtDelivery{URL: statusURL, IntermediateReport: true}, ContentType: "application/json"},
	}
	cost := gsm7.Segments(msg.Text())

	if len(msg.Attachments()) > 0 {
		mt.Channel = "MMS"
		cost = 1

		items := make([]mtMixedItem, 0, len(msg.Attachments())+1)
		if msg.Text() != "" {
			items = append(items, mtMixedItem{Text: msg.Text()})
		}
		for _, a := range msg.Attachments() {
			_, url := handlers.SplitAttachment(a)
			if url == "" {
				clog.Error(models.ErrorAttachmentNotDecodable())
				continue
			}
			items = append(items, mtMixedItem{URL: url})
		}
		mt.Content = mtContent{Body: mtBody{Type: "MIXED", Items: items}}
	}

	requestBody := &bytes.Buffer{}
	if err := json.NewEncoder(requestBody).Encode(mtPayload{Messages: []mtMessage{mt}}); err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/messages-api/1/messages", requestBody)
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
	res.AddCost(cost)
	if err := handlers.ErrorFromResponse(resp, err); err != nil {
		return err
	}

	groupID, err := jsonparser.GetInt(respBody, "messages", "[0]", "status", "groupId")
	if err != nil || (groupID != 1 && groupID != 3) {
		name, _ := jsonparser.GetString(respBody, "messages", "[0]", "status", "name")
		if name != "" {
			description, _ := jsonparser.GetString(respBody, "messages", "[0]", "status", "description")
			clog.Error(models.ErrorExternal(name, description))
		}
		return channels.ErrResponseContent
	}

	externalID, err := jsonparser.GetString(respBody, "messages", "[0]", "messageId")
	if err != nil {
		clog.Error(models.ErrorResponseValueMissing("messageId"))
	} else {
		res.AddExternalID(externalID)
	}

	return nil
}

// messageOptions builds the message options from the channel config, or nil if there are none
func messageOptions(ch *models.Channel) *mtOptions {
	appID := ch.StringConfigForKey(configApplicationID, "")
	entityID := ch.StringConfigForKey(configEntityID, "")
	transliteration := ch.StringConfigForKey(configTransliteration, "")

	if appID == "" && entityID == "" && transliteration == "" {
		return nil
	}

	opts := &mtOptions{Transliteration: transliteration}
	if appID != "" || entityID != "" {
		opts.Platform = &mtPlatform{ApplicationID: appID, EntityID: entityID}
	}
	return opts
}

func (h *handler) RedactValues(ch *models.Channel) []string {
	redacted := []string{}
	if apiKey := ch.StringConfigForKey(models.ConfigAPIKey, ""); apiKey != "" {
		redacted = append(redacted, "App "+apiKey)
	}
	if username := ch.StringConfigForKey(models.ConfigUsername, ""); username != "" {
		redacted = append(redacted, httpx.BasicAuth(username, ch.StringConfigForKey(models.ConfigPassword, "")))
	}
	return redacted
}
