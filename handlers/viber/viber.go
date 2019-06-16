package viber

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
	"github.com/pkg/errors"
)

const (
	configViberWelcomeMessage = "welcome_message"
)

var (
	viberSignatureHeader = "X-Viber-Content-Signature"
	sendURL              = "https://chatapi.viber.com/pa/send_message"
	maxMsgLength         = 7000
	quickReplyTextSize   = 36
	descriptionMaxLength = 120
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("VP"), "Viber")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveEvent)
	return nil
}

type eventPayload struct {
	Event        string `json:"event"         validate:"required"`
	Timestamp    int64  `json:"timestamp"     validate:"required"`
	MessageToken int64  `json:"message_token" validate:"required"`
	UserID       string `json:"user_id"`
	Sender       struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"sender"`
	User struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"user"`
	Message struct {
		Text    string `json:"text"`
		Media   string `json:"media"`
		Contact struct {
			Name        string `json:"name"`
			PhoneNumber string `json:"phone_number"`
		}
		Location struct {
			Latitude  float64 `json:"lat"`
			Longitude float64 `json:"lon"`
		}
		Type         string `json:"type"`
		TrackingData string `json:"tracking_data"`
	} `json:"message"`
}

type welcomeMessagePayload struct {
	AuthToken    string            `json:"auth_token"`
	Text         string            `json:"text"`
	Type         string            `json:"type"`
	TrackingData string            `json:"tracking_data"`
	Sender       map[string]string `json:"sender,omitempty"`
}

// receiveEvent is our HTTP handler function for incoming messages
func (h *handler) receiveEvent(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	err := h.validateSignature(channel, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	payload := &eventPayload{}
	err = handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	event := payload.Event
	switch event {
	case "webhook":
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "webhook valid")

	case "conversation_started":
		msgText := channel.StringConfigForKey(configViberWelcomeMessage, "")
		if msgText == "" {
			return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignored conversation start")
		}

		viberID := payload.User.ID
		ContactName := payload.User.Name

		// build the URN
		urn, err := urns.NewURNFromParts(urns.ViberScheme, viberID, "", "")
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}
		// build the channel event
		channelEvent := h.Backend().NewChannelEvent(channel, courier.WelcomeMessage, urn).WithContactName(ContactName)

		err = h.Backend().WriteChannelEvent(ctx, channelEvent)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}

		return []courier.Event{channelEvent}, writeWelcomeMessageResponse(w, channel, channelEvent)

	case "subscribed":
		viberID := payload.User.ID
		ContactName := payload.User.Name

		// build the URN
		urn, err := urns.NewURNFromParts(urns.ViberScheme, viberID, "", "")
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}

		// build the channel event
		channelEvent := h.Backend().NewChannelEvent(channel, courier.NewConversation, urn).WithContactName(ContactName)

		err = h.Backend().WriteChannelEvent(ctx, channelEvent)
		if err != nil {
			return nil, err
		}

		return []courier.Event{channelEvent}, courier.WriteChannelEventSuccess(ctx, w, r, channelEvent)

	case "unsubscribed":
		viberID := payload.UserID

		// build the URN
		urn, err := urns.NewURNFromParts(urns.ViberScheme, viberID, "", "")
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}
		// build the channel event
		channelEvent := h.Backend().NewChannelEvent(channel, courier.StopContact, urn)

		err = h.Backend().WriteChannelEvent(ctx, channelEvent)
		if err != nil {
			return nil, err
		}

		return []courier.Event{channelEvent}, courier.WriteChannelEventSuccess(ctx, w, r, channelEvent)

	case "failed":
		msgStatus := h.Backend().NewMsgStatusForExternalID(channel, string(payload.MessageToken), courier.MsgFailed)
		return handlers.WriteMsgStatusAndResponse(ctx, h, channel, msgStatus, w, r)

	case "delivered":
		// we ignore delivered events for viber as they send these for incoming messages too and its not worth the db hit to verify that
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring delivered status")

	case "message":
		sender := payload.Sender.ID
		if sender == "" {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("missing required sender id"))
		}

		contactName := payload.Sender.Name

		// create our URN
		urn, err := urns.NewURNFromParts(urns.ViberScheme, sender, "", "")
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}

		text := payload.Message.Text
		mediaURL := ""

		// process any attached media
		messageType := payload.Message.Type
		switch messageType {

		case "picture":
			mediaURL = payload.Message.Media

		case "video":
			mediaURL = payload.Message.Media

		case "contact":
			text = fmt.Sprintf("%s: %s", payload.Message.Contact.Name, payload.Message.Contact.PhoneNumber)

		case "url":
			text = payload.Message.Media

		case "location":
			mediaURL = fmt.Sprintf("geo:%f,%f", payload.Message.Location.Latitude, payload.Message.Location.Longitude)

		case "text":
			text = payload.Message.Text

		default:
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unknown message type: %s", messageType))
		}

		if text == "" && mediaURL == "" {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("missing text or media in message in request body"))
		}

		// build our msg
		msg := h.Backend().NewIncomingMsg(channel, urn, text).WithExternalID(fmt.Sprintf("%d", payload.MessageToken)).WithContactName(contactName)
		if mediaURL != "" {
			msg.WithAttachment(mediaURL)
		}
		// and finally write our message
		return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r)
	}

	return nil, courier.WriteError(ctx, w, r, fmt.Errorf("not handled, unknown event: %s", event))
}

func writeWelcomeMessageResponse(w http.ResponseWriter, channel courier.Channel, event courier.Event) error {

	authToken := channel.StringConfigForKey(courier.ConfigAuthToken, "")
	msgText := channel.StringConfigForKey(configViberWelcomeMessage, "")
	payload := welcomeMessagePayload{
		AuthToken:    authToken,
		Text:         msgText,
		Type:         "text",
		TrackingData: string(event.EventID()),
	}

	responseBody := &bytes.Buffer{}
	err := json.NewEncoder(responseBody).Encode(payload)
	if err != nil {
		return nil
	}

	w.WriteHeader(200)
	_, err = fmt.Fprint(w, responseBody)
	return err
}

// see https://developers.viber.com/docs/api/rest-bot-api/#callbacks
func (h *handler) validateSignature(channel courier.Channel, r *http.Request) error {
	actual := r.Header.Get(viberSignatureHeader)
	if actual == "" {
		return fmt.Errorf("missing request signature")
	}

	authToken := channel.StringConfigForKey(courier.ConfigAuthToken, "")
	if authToken == "" {
		return fmt.Errorf("invalid or missing auth token in config")
	}

	// read our body
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}

	r.Body = ioutil.NopCloser(bytes.NewBuffer(body))
	expected := calculateSignature(authToken, body)

	// compare signatures in way that isn't sensitive to a timing attack
	if !hmac.Equal([]byte(expected), []byte(actual)) {
		return fmt.Errorf("invalid request signature: %s", actual)
	}

	return nil
}

func calculateSignature(authToken string, contents []byte) string {
	mac := hmac.New(sha256.New, []byte(authToken))
	mac.Write(contents)
	return hex.EncodeToString(mac.Sum(nil))
}

type mtPayload struct {
	AuthToken    string            `json:"auth_token"`
	Receiver     string            `json:"receiver"`
	Text         string            `json:"text"`
	Type         string            `json:"type"`
	TrackingData string            `json:"tracking_data"`
	Sender       map[string]string `json:"sender,omitempty"`
	Media        string            `json:"media,omitempty"`
	Size         int               `json:"size,omitempty"`
	FileName     string            `json:"file_name,omitempty"`
	Keyboard     *mtKeyboard       `json:"keyboard,omitempty"`
}

type mtKeyboard struct {
	Type          string     `json:"Type"`
	DefaultHeight bool       `json:"DefaultHeight"`
	Buttons       []mtButton `json:"Buttons"`
}

type mtButton struct {
	ActionType string `json:"ActionType"`
	ActionBody string `json:"ActionBody"`
	Text       string `json:"Text"`
	TextSize   string `json:"TextSize"`
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	authToken := msg.Channel().StringConfigForKey(courier.ConfigAuthToken, "")
	if authToken == "" {
		return nil, fmt.Errorf("missing auth token in config")
	}

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)

	// figure out whether we have a keyboard to send as well
	qrs := msg.QuickReplies()
	var replies *mtKeyboard

	if len(qrs) > 0 {
		buttons := make([]mtButton, len(qrs))
		for i, qr := range qrs {
			buttons[i].ActionType = "reply"
			buttons[i].TextSize = "regular"
			buttons[i].ActionBody = string(qr[:])
			buttons[i].Text = string(qr[:])
		}

		replies = &mtKeyboard{"keyboard", true, buttons}
	}
	parts := handlers.SplitMsg(msg.Text(), maxMsgLength)
	if len(msg.Attachments()) > 0 && len(parts[0]) > descriptionMaxLength {
		descriptionPart := handlers.SplitMsg(msg.Text(), descriptionMaxLength)[0]
		others := handlers.SplitMsg(strings.TrimSpace(strings.Replace(msg.Text(), descriptionPart, "", 1)), maxMsgLength)
		parts = []string{descriptionPart}
		parts = append(parts, others...)
	}

	for i, part := range parts {
		msgType := "text"
		attSize := -1
		attURL := ""
		filename := ""

		// add any media URL to the first part
		if len(msg.Attachments()) > 0 && i == 0 {
			mediaType, mediaURL := handlers.SplitAttachment(msg.Attachments()[0])
			switch strings.Split(mediaType, "/")[0] {
			case "image":
				msgType = "picture"
				attURL = mediaURL

			case "video":
				msgType = "video"
				attURL = mediaURL
				req, err := http.NewRequest(http.MethodHead, mediaURL, nil)
				if err != nil {
					return nil, err
				}
				rr, err := utils.MakeHTTPRequest(req)
				if err != nil {
					return nil, err
				}

				attSize = rr.ContentLength

			case "audio":
				msgType = "file"
				attURL = mediaURL
				req, err := http.NewRequest(http.MethodHead, mediaURL, nil)
				if err != nil {
					return nil, err
				}
				rr, err := utils.MakeHTTPRequest(req)
				if err != nil {
					return nil, err
				}
				attSize = rr.ContentLength
				filename = "Audio"

			default:
				status.AddLog(courier.NewChannelLog("Unknown media type: "+mediaType, msg.Channel(), msg.ID(), "", "", courier.NilStatusCode,
					"", "", time.Duration(0), fmt.Errorf("unknown media type: %s", mediaType)))

			}
		}

		payload := mtPayload{
			AuthToken:    authToken,
			Receiver:     msg.URN().Path(),
			Text:         part,
			Type:         msgType,
			TrackingData: msg.ID().String(),
			Media:        attURL,
			FileName:     filename,
			Keyboard:     replies,
		}

		if attSize != -1 {
			payload.Size = attSize
		}

		requestBody := &bytes.Buffer{}
		err := json.NewEncoder(requestBody).Encode(payload)
		if err != nil {
			return nil, err
		}

		// build our request
		req, _ := http.NewRequest(http.MethodPost, sendURL, requestBody)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		rr, err := utils.MakeHTTPRequest(req)

		// record log
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
		status.AddLog(log)
		if err != nil {
			return status, nil
		}

		responseStatus, err := jsonparser.GetInt(rr.Body, "status")
		if err != nil {
			log.WithError("Message Send Error", errors.Errorf("received invalid JSON response"))
			return status, nil
		}
		if responseStatus != 0 {
			log.WithError("Message Send Error", errors.Errorf("received non-0 status: '%d'", responseStatus))
			return status, nil
		}

		status.SetStatus(courier.MsgWired)
		replies = nil
	}
	return status, nil
}
