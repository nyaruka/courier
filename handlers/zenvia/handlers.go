package zenvia

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
)

var (
	maxMsgLength    = 1152
	whatsappSendURL = "https://api.zenvia.com/v2/channels/whatsapp/messages"
	smsSendURL      = "https://api.zenvia.com/v2/channels/sms/messages"
)

func init() {
	courier.RegisterHandler(newHandler("ZVW", "Zenvia WhatsApp"))
	courier.RegisterHandler(newHandler("ZVS", "Zenvia SMS"))
}

type handler struct {
	handlers.BaseHandler
}

func newHandler(channelType courier.ChannelType, name string) courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(channelType, name)}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, handlers.JSONPayload(h, h.receiveMessage))
	s.AddHandlerRoute(h, http.MethodPost, "status", courier.ChannelLogTypeMsgStatus, handlers.JSONPayload(h, h.receiveStatus))
	return nil
}

type moContent struct {
	Type         string  `json:"type"   validate:"required"`
	Text         string  `json:"text"`
	Payload      string  `json:"payload"`
	FileURL      string  `json:"fileUrl"`
	FileMimeType string  `json:"fileMimeType"`
	FileCaption  string  `json:"fileCaption"`
	FileName     string  `json:"fileName"`
	Longitude    float32 `json:"longitude"`
	Latitude     float32 `json:"latitude"`
	Name         string  `json:"name"`
	Address      string  `json:"address"`
	URL          string  `json:"url"`
}

type moPayload struct {
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"     validate:"required"`
	Type      string `json:"type"          validate:"required" `
	Message   struct {
		ID        string      `json:"id"          validate:"required"`
		From      string      `json:"from"        validate:"required"`
		To        string      `json:"to"          validate:"required" `
		Direction string      `json:"direction"   validate:"required" `
		Channel   string      `json:"channel"`
		Contents  []moContent `json:"contents"    validate:"required" `
	} `json:"message"`
	Visitor struct {
		Name string `json:"name"`
	}
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *moPayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	if strings.ToUpper(payload.Type) != "MESSAGE" {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unsupported event type: %s", payload.Type))
	}

	// create our date from the timestamp
	// 2017-05-03T06:04:45Z
	date, err := time.Parse("2006-01-02T15:04:05Z", payload.Timestamp)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("invalid date format: %s", payload.Timestamp))
	}

	if strings.ToUpper(payload.Message.Direction) != "IN" {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring request, not incoming messages")
	}

	// create our URN
	urn, err := urns.NewWhatsAppURN(payload.Message.From)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	contactName := payload.Visitor.Name

	msgs := []courier.MsgIn{}

	for _, content := range payload.Message.Contents {

		text := ""
		mediaURL := ""

		if content.Type == "text" {
			text = content.Text
		} else if content.Type == "location" {
			mediaURL = fmt.Sprintf("geo:%f,%f", content.Latitude, content.Longitude)
		} else if content.Type == "file" {
			mediaURL = content.FileURL
		} else {
			// we received a message type we do not support.
			courier.LogRequestError(r, channel, fmt.Errorf("unsupported message type %s", content.Type))
		}

		// build our msg
		msg := h.Backend().NewIncomingMsg(channel, urn, text, payload.Message.ID, clog).WithReceivedOn(date.UTC()).WithContactName(contactName)
		if mediaURL != "" {
			msg.WithAttachment(mediaURL)
		}
		msgs = append(msgs, msg)
	}

	// and finally write our messages
	return handlers.WriteMsgsAndResponse(ctx, h, msgs, w, r, clog)
}

var statusMapping = map[string]courier.MsgStatus{
	"REJECTED":      courier.MsgStatusFailed,
	"NOT_DELIVERED": courier.MsgStatusFailed,
	"SENT":          courier.MsgStatusSent,
	"DELIVERED":     courier.MsgStatusDelivered,
	"READ":          courier.MsgStatusDelivered,
}

type statusPayload struct {
	ID            string `json:"id"`
	Type          string `json:"type"       validate:"required" `
	MessageID     string `json:"messageId"`
	MessageStatus struct {
		Timestamp string `json:"timestamp"`
		Code      string `json:"code"`
	} `json:"messageStatus"`
}

// receiveStatus is our HTTP handler function for status updates
func (h *handler) receiveStatus(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *statusPayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	if strings.ToUpper(payload.Type) != "MESSAGE_STATUS" {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unsupported event type: %s", payload.Type))
	}

	msgStatus, found := statusMapping[strings.ToUpper(payload.MessageStatus.Code)]
	if !found {
		msgStatus = courier.MsgStatusErrored
	}

	// write our status
	status := h.Backend().NewStatusUpdateByExternalID(channel, payload.MessageID, msgStatus, clog)
	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

type mtContent struct {
	Type         string `json:"type"`
	Text         string `json:"text,omitempty"`
	FileURL      string `json:"fileUrl,omitempty"`
	FileMimeType string `json:"fileMimeType,omitempty"`
	FileCaption  string `json:"fileCaption,omitempty"`
	FileName     string `json:"fileName,omitempty"`
}

type mtPayload struct {
	From     string      `json:"from"`
	To       string      `json:"to"`
	Contents []mtContent `json:"contents"`
}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	channel := msg.Channel()

	token := channel.StringConfigForKey(courier.ConfigAPIKey, "")
	if token == "" {
		return nil, fmt.Errorf("no token set for ZVW channel")
	}

	payload := mtPayload{
		From: strings.TrimLeft(channel.Address(), "+"),
		To:   strings.TrimLeft(msg.URN().Path(), "+"),
	}

	status := h.Backend().NewStatusUpdate(channel, msg.ID(), courier.MsgStatusErrored, clog)

	text := ""
	if channel.ChannelType() == "ZVW" {
		for _, attachment := range msg.Attachments() {
			attType, attURL := handlers.SplitAttachment(attachment)
			payload.Contents = append(payload.Contents, mtContent{
				Type:         "file",
				FileURL:      attURL,
				FileMimeType: attType,
			})

		}
		text = msg.Text()

	} else if channel.ChannelType() == "ZVS" {
		text = handlers.GetTextAndAttachments(msg)
	}

	msgParts := make([]string, 0)
	if text != "" {
		msgParts = handlers.SplitMsgByChannel(channel, text, maxMsgLength)
	}

	for _, msgPart := range msgParts {
		payload.Contents = append(payload.Contents, mtContent{
			Type: "text",
			Text: msgPart,
		})
	}

	jsonBody := jsonx.MustMarshal(payload)
	sendURL := whatsappSendURL
	if channel.ChannelType() == "ZVS" {
		sendURL = smsSendURL
	}

	req, err := http.NewRequest(http.MethodPost, sendURL, bytes.NewReader(jsonBody))

	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-API-TOKEN", token)

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return status, nil
	}

	externalID, err := jsonparser.GetString(respBody, "id")
	if err != nil {
		clog.Error(courier.ErrorResponseValueMissing("id"))
		return status, nil
	}

	status.SetExternalID(externalID)
	status.SetStatus(courier.MsgStatusWired)
	return status, nil
}
