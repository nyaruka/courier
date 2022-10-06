package viber

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
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
	descriptionMaxLength = 512
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
		Text      string `json:"text"`
		Media     string `json:"media"`
		StickerID string `json:"sticker_id"`
		Contact   struct {
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
func (h *handler) receiveEvent(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
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
		channelEvent := h.Backend().NewChannelEvent(channel, courier.WelcomeMessage, urn, clog).WithContactName(ContactName)

		err = h.Backend().WriteChannelEvent(ctx, channelEvent, clog)
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
		channelEvent := h.Backend().NewChannelEvent(channel, courier.NewConversation, urn, clog).WithContactName(ContactName)

		err = h.Backend().WriteChannelEvent(ctx, channelEvent, clog)
		if err != nil {
			return nil, err
		}

		return []courier.Event{channelEvent}, courier.WriteChannelEventSuccess(w, channelEvent)

	case "unsubscribed":
		viberID := payload.UserID

		// build the URN
		urn, err := urns.NewURNFromParts(urns.ViberScheme, viberID, "", "")
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}
		// build the channel event
		channelEvent := h.Backend().NewChannelEvent(channel, courier.StopContact, urn, clog)

		err = h.Backend().WriteChannelEvent(ctx, channelEvent, clog)
		if err != nil {
			return nil, err
		}

		return []courier.Event{channelEvent}, courier.WriteChannelEventSuccess(w, channelEvent)

	case "failed":
		msgStatus := h.Backend().NewMsgStatusForExternalID(channel, fmt.Sprintf("%d", payload.MessageToken), courier.MsgFailed, clog)
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

		case "sticker":
			mediaURL = fmt.Sprintf("https://viber.github.io/docs/img/stickers/%s.png", payload.Message.StickerID)

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
		msg := h.Backend().NewIncomingMsg(channel, urn, text, clog).WithExternalID(fmt.Sprintf("%d", payload.MessageToken)).WithContactName(contactName)
		if mediaURL != "" {
			msg.WithAttachment(mediaURL)
		}
		// and finally write our message
		return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r, clog)
	}

	return nil, courier.WriteError(w, http.StatusBadRequest, fmt.Errorf("not handled, unknown event: %s", event))
}

func writeWelcomeMessageResponse(w http.ResponseWriter, channel courier.Channel, event courier.Event) error {

	authToken := channel.StringConfigForKey(courier.ConfigAuthToken, "")
	msgText := channel.StringConfigForKey(configViberWelcomeMessage, "")
	payload := welcomeMessagePayload{
		AuthToken:    authToken,
		Text:         msgText,
		Type:         "text",
		TrackingData: fmt.Sprintf("%d", event.EventID()),
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
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}

	r.Body = io.NopCloser(bytes.NewBuffer(body))
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
	Text         string            `json:"text,omitempty"`
	Type         string            `json:"type"`
	TrackingData string            `json:"tracking_data"`
	Sender       map[string]string `json:"sender,omitempty"`
	Media        string            `json:"media,omitempty"`
	Size         int               `json:"size,omitempty"`
	FileName     string            `json:"file_name,omitempty"`
	Keyboard     *Keyboard         `json:"keyboard,omitempty"`
}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.Msg, clog *courier.ChannelLog) (courier.MsgStatus, error) {
	authToken := msg.Channel().StringConfigForKey(courier.ConfigAuthToken, "")
	if authToken == "" {
		return nil, fmt.Errorf("missing auth token in config")
	}

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored, clog)

	// figure out whether we have a keyboard to send as well
	qrs := msg.QuickReplies()
	var keyboard *Keyboard

	if len(qrs) > 0 {
		buttonLayout := msg.Channel().ConfigForKey("button_layout", map[string]interface{}{}).(map[string]interface{})
		keyboard = NewKeyboardFromReplies(qrs, buttonLayout)
	}
	parts := handlers.SplitMsgByChannel(msg.Channel(), msg.Text(), maxMsgLength)

	descriptionPart := ""
	if len(msg.Attachments()) == 1 && len(msg.Text()) < descriptionMaxLength {
		mediaType, _ := handlers.SplitAttachment(msg.Attachments()[0])
		isImage := strings.Split(mediaType, "/")[0] == "image"

		if isImage {
			descriptionPart = msg.Text()
			parts = []string{}
		}

	}

	for i := 0; i < len(parts)+len(msg.Attachments()); i++ {
		msgType := "text"
		attSize := -1
		attURL := ""
		filename := ""
		msgText := ""
		var err error

		if i < len(msg.Attachments()) {
			mediaType, mediaURL := handlers.SplitAttachment(msg.Attachments()[0])
			switch strings.Split(mediaType, "/")[0] {
			case "image":
				msgType = "picture"
				attURL = mediaURL
				msgText = descriptionPart

			case "video":
				msgType = "video"
				attURL = mediaURL
				attSize, err = getAttachmentSize(mediaURL, clog)
				if err != nil {
					return nil, err
				}
				msgText = ""

			case "audio":
				msgType = "file"
				attURL = mediaURL
				attSize, err = getAttachmentSize(mediaURL, clog)
				if err != nil {
					return nil, err
				}
				filename = "Audio"
				msgText = ""

			default:
				clog.Error(courier.ErrorUnsupportedMedia(mediaType))
			}

		} else {
			msgText = parts[i-len(msg.Attachments())]
		}

		payload := mtPayload{
			AuthToken:    authToken,
			Receiver:     msg.URN().Path(),
			Text:         msgText,
			Type:         msgType,
			TrackingData: msg.ID().String(),
			Media:        attURL,
			FileName:     filename,
			Keyboard:     keyboard,
		}

		if attSize != -1 {
			payload.Size = attSize
		}

		requestBody := &bytes.Buffer{}
		err = json.NewEncoder(requestBody).Encode(payload)
		if err != nil {
			return nil, err
		}

		// build our request
		req, err := http.NewRequest(http.MethodPost, sendURL, requestBody)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, respBody, err := handlers.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 != 2 {
			return status, nil
		}
		responseStatus, err := jsonparser.GetInt(respBody, "status")
		if err != nil {
			clog.Error(courier.ErrorResponseUnparseable("JSON"))
			return status, nil
		}
		if responseStatus != 0 {
			clog.RawError(errors.Errorf("received non-0 status: '%d'", responseStatus))
			return status, nil
		}

		status.SetStatus(courier.MsgWired)
		keyboard = nil
	}
	return status, nil
}

func getAttachmentSize(u string, clog *courier.ChannelLog) (int, error) {
	req, err := http.NewRequest(http.MethodHead, u, nil)
	if err != nil {
		return 0, err
	}

	resp, _, err := handlers.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return 0, errors.New("unable to get attachment size")
	}

	contentLenHdr := resp.Header.Get("Content-Length")

	if resp.Header.Get("Content-Length") != "" {
		contentLength, err := strconv.Atoi(contentLenHdr)
		if err == nil {
			return contentLength, nil
		}
	}

	return 0, nil
}
