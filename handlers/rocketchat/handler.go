package rocketchat

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
)

const (
	configBaseURL        = "base_url"
	configSecret         = "secret"
	configBotUsername    = "bot_username"
	configAdminAuthToken = "admin_auth_token"
	configAdminUserID    = "admin_user_id"
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("RC"), "RocketChat")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, handlers.JSONPayload(h, h.receiveMessage))
	return nil
}

type Attachment struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type moPayload struct {
	User struct {
		URN      string `json:"urn"     validate:"required"`
		Username string `json:"username"`
		FullName string `json:"full_name"`
	} `json:"user" validate:"required"`
	Text        string       `json:"text"`
	Attachments []Attachment `json:"attachments"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *moPayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	// check authorization
	secret := channel.StringConfigForKey(configSecret, "")
	if fmt.Sprintf("Token %s", secret) != r.Header.Get("Authorization") {
		return nil, courier.WriteAndLogUnauthorized(w, r, channel, fmt.Errorf("invalid Authorization header"))
	}

	// check content empty
	if payload.Text == "" && len(payload.Attachments) == 0 {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, errors.New("no text or attachment"))
	}

	urn, err := urns.NewURNFromParts(urns.RocketChatScheme, payload.User.URN, "", payload.User.Username)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	msg := h.Backend().NewIncomingMsg(channel, urn, payload.Text, "", clog).WithContactName(payload.User.FullName)
	for _, attachment := range payload.Attachments {
		msg.WithAttachment(attachment.URL)
	}

	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
}

// BuildAttachmentRequest download media for message attachment with RC auth_token/user_id set
func (h *handler) BuildAttachmentRequest(ctx context.Context, b courier.Backend, channel courier.Channel, attachmentURL string, clog *courier.ChannelLog) (*http.Request, error) {
	adminAuthToken := channel.StringConfigForKey(configAdminAuthToken, "")
	adminUserID := channel.StringConfigForKey(configAdminUserID, "")
	if adminAuthToken == "" || adminUserID == "" {
		return nil, fmt.Errorf("missing token for RC channel")
	}

	req, _ := http.NewRequest(http.MethodGet, attachmentURL, nil)
	req.Header.Set("X-Auth-Token", adminAuthToken)
	req.Header.Set("X-User-Id", adminUserID)
	return req, nil
}

var _ courier.AttachmentRequestBuilder = (*handler)(nil)

type mtPayload struct {
	UserURN     string       `json:"user"`
	BotUsername string       `json:"bot"`
	Text        string       `json:"text,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	baseURL := msg.Channel().StringConfigForKey(configBaseURL, "")
	secret := msg.Channel().StringConfigForKey(configSecret, "")
	botUsername := msg.Channel().StringConfigForKey(configBotUsername, "")

	// the status that will be written for this message
	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)

	payload := &mtPayload{
		UserURN:     msg.URN().Path(),
		BotUsername: botUsername,
		Text:        msg.Text(),
	}
	for _, attachment := range msg.Attachments() {
		mimeType, url := handlers.SplitAttachment(attachment)
		payload.Attachments = append(payload.Attachments, Attachment{mimeType, url})
	}

	body := jsonx.MustMarshal(payload)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/message", bytes.NewReader(body))
	if err != nil {
		return status, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Token %s", secret))

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return status, nil
	}

	msgID, err := jsonparser.GetString(respBody, "id")
	if err == nil {
		status.SetExternalID(msgID)
	}

	status.SetStatus(courier.MsgStatusSent)
	return status, nil
}
