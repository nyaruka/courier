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

	// https://developers.viber.com/docs/api/rest-bot-api/#error-codes
	sendErrorCodes = map[int]string{
		1:  "The webhook URL is not valid",
		2:  "The authentication token is not valid",
		3:  "There is an error in the request itself (missing comma, brackets, etc.)",
		4:  "Some mandatory data is missing",
		5:  "The receiver is not registered to Viber",
		6:  "The receiver is not subscribed to the account",
		7:  "The account is blocked",
		8:  "The account associated with the token is not a account.",
		9:  "The account is suspended",
		10: "No webhook was set for the account",
		11: "The receiver is using a device or a Viber version that don’t support accounts",
		12: "Rate control breach",
		13: "Maximum supported account version by all user’s devices is less than the minApiVersion in the message",
		14: "minApiVersion is not compatible to the message fields",
		15: "The account is not authorized",
		16: "Inline message not allowed",
		17: "The account is not inline",
		18: "Failed to post to public account. The bot is missing a Public Chat interface",
		19: "Cannot send broadcast message",
		20: "Attempt to send broadcast message from the bot",
		21: "The message sent is not supported in the destination country",
		22: "The bot does not support payment messages",
		23: "The non-billable bot has reached the monthly threshold of free out of session messages",
		24: "No balance for a billable bot (when the “free out of session messages” threshold has been reached)",
	}
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
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeUnknown, handlers.JSONPayload(h, h.receiveEvent))
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
func (h *handler) receiveEvent(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *eventPayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	err := h.validateSignature(channel, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	event := payload.Event
	switch event {
	case "webhook":
		clog.SetType(courier.ChannelLogTypeWebhookVerify)

		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "webhook valid")

	case "conversation_started":
		clog.SetType(courier.ChannelLogTypeEventReceive)

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
		channelEvent := h.Backend().NewChannelEvent(channel, courier.EventTypeWelcomeMessage, urn, clog).WithContactName(ContactName)

		err = h.Backend().WriteChannelEvent(ctx, channelEvent, clog)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}

		return []courier.Event{channelEvent}, writeWelcomeMessageResponse(w, channel, channelEvent)

	case "subscribed":
		clog.SetType(courier.ChannelLogTypeEventReceive)

		viberID := payload.User.ID
		ContactName := payload.User.Name

		// build the URN
		urn, err := urns.NewURNFromParts(urns.ViberScheme, viberID, "", "")
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}

		// build the channel event
		channelEvent := h.Backend().NewChannelEvent(channel, courier.EventTypeNewConversation, urn, clog).WithContactName(ContactName)

		err = h.Backend().WriteChannelEvent(ctx, channelEvent, clog)
		if err != nil {
			return nil, err
		}

		return []courier.Event{channelEvent}, courier.WriteChannelEventSuccess(w, channelEvent)

	case "unsubscribed":
		clog.SetType(courier.ChannelLogTypeEventReceive)

		viberID := payload.UserID

		// build the URN
		urn, err := urns.NewURNFromParts(urns.ViberScheme, viberID, "", "")
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}
		// build the channel event
		channelEvent := h.Backend().NewChannelEvent(channel, courier.EventTypeStopContact, urn, clog)

		err = h.Backend().WriteChannelEvent(ctx, channelEvent, clog)
		if err != nil {
			return nil, err
		}

		return []courier.Event{channelEvent}, courier.WriteChannelEventSuccess(w, channelEvent)

	case "failed":
		clog.SetType(courier.ChannelLogTypeMsgStatus)

		msgStatus := h.Backend().NewStatusUpdateByExternalID(channel, fmt.Sprintf("%d", payload.MessageToken), courier.MsgStatusFailed, clog)
		return handlers.WriteMsgStatusAndResponse(ctx, h, channel, msgStatus, w, r)

	case "delivered":
		clog.SetType(courier.ChannelLogTypeMsgStatus)

		// we ignore delivered events for viber as they send these for incoming messages too and its not worth the db hit to verify that
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring delivered status")

	case "message":
		clog.SetType(courier.ChannelLogTypeMsgReceive)

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
		msg := h.Backend().NewIncomingMsg(channel, urn, text, fmt.Sprintf("%d", payload.MessageToken), clog).WithContactName(contactName)
		if mediaURL != "" {
			msg.WithAttachment(mediaURL)
		}
		// and finally write our message
		return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
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

type mtResponse struct {
	Status        int    `json:"status"`
	StatusMessage string `json:"status_message"`
}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	authToken := msg.Channel().StringConfigForKey(courier.ConfigAuthToken, "")
	if authToken == "" {
		return nil, fmt.Errorf("missing auth token in config")
	}

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)

	// figure out whether we have a keyboard to send as well
	qrs := msg.QuickReplies()
	var keyboard *Keyboard

	if len(qrs) > 0 {
		buttonLayout := msg.Channel().ConfigForKey("button_layout", map[string]any{}).(map[string]any)
		keyboard = NewKeyboardFromReplies(qrs, buttonLayout)
	}

	for _, part := range handlers.SplitMsg(msg, handlers.SplitOptions{MaxTextLen: maxMsgLength, MaxCaptionLen: descriptionMaxLength, Captionable: []handlers.MediaType{handlers.MediaTypeImage}}) {
		msgType := "text"
		attSize := -1
		attURL := ""
		filename := ""
		msgText := ""
		var err error

		if part.Type == handlers.MsgPartTypeAttachment || part.Type == handlers.MsgPartTypeCaptionedAttachment {
			mediaType, mediaURL := handlers.SplitAttachment(part.Attachment)
			switch strings.Split(mediaType, "/")[0] {
			case "image":
				msgType = "picture"
				attURL = mediaURL
				msgText = part.Text

			case "video":
				msgType = "video"
				attURL = mediaURL
				attSize, err = h.getAttachmentSize(mediaURL, clog)
				if err != nil {
					return nil, err
				}
				msgText = ""

			case "audio":
				msgType = "file"
				attURL = mediaURL
				attSize, err = h.getAttachmentSize(mediaURL, clog)
				if err != nil {
					return nil, err
				}
				filename = "Audio"
				msgText = ""

			default:
				clog.Error(courier.ErrorMediaUnsupported(mediaType))
			}

		} else {
			msgText = part.Text
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

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 != 2 {
			clog.Error(courier.ErrorResponseStatusCode())
			return status, nil
		}

		respPayload := &mtResponse{}
		err = json.Unmarshal(respBody, respPayload)
		if err != nil {
			clog.Error(courier.ErrorResponseUnparseable("JSON"))
			return status, nil
		}

		if respPayload.Status != 0 {
			errorMessage, found := sendErrorCodes[respPayload.Status]
			if !found {
				errorMessage = "General error"
			}
			clog.Error(courier.ErrorExternal(strconv.Itoa(respPayload.Status), errorMessage))
			return status, nil
		}

		status.SetStatus(courier.MsgStatusWired)
		keyboard = nil
	}
	return status, nil
}

func (h *handler) getAttachmentSize(u string, clog *courier.ChannelLog) (int, error) {
	req, err := http.NewRequest(http.MethodHead, u, nil)
	if err != nil {
		return 0, err
	}

	resp, _, err := h.RequestHTTP(req, clog)
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
