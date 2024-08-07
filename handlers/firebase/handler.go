package firebase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/buger/jsonparser"
	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/option"
)

const (
	configTitle           = "FCM_TITLE"
	configNotification    = "FCM_NOTIFICATION"
	configKey             = "FCM_KEY"
	configCredentialsFile = "FCM_CREDENTIALS_JSON"
)

var (
	sendURL      = "https://fcm.googleapis.com/fcm/send"
	maxMsgLength = 1024
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler

	fetchTokenMutex sync.Mutex
}

func newHandler() courier.ChannelHandler {
	return &handler{
		BaseHandler:     handlers.NewBaseHandler(courier.ChannelType("FCM"), "Firebase", handlers.WithRedactConfigKeys(configKey)),
		fetchTokenMutex: sync.Mutex{},
	}
}

func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodPost, "register", courier.ChannelLogTypeEventReceive, h.registerContact)
	return nil
}

type receiveForm struct {
	From     string `name:"from"       validate:"required"`
	Msg      string `name:"msg"`
	FCMToken string `name:"fcm_token"`
	Date     string `name:"date"`
	Name     string `name:"name"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	form := &receiveForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	date := time.Now().UTC()
	if form.Date != "" {
		date, err = time.Parse("2006-01-02T15:04:05.000", form.Date)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unable to parse date: %s", form.Date))
		}
	}

	// create our URN
	urn, err := urns.New(urns.Firebase, form.From)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// if a new auth token was provided, record that
	var authTokens map[string]string
	if form.FCMToken != "" {
		authTokens = map[string]string{"default": form.FCMToken}
	}

	// build our msg
	dbMsg := h.Backend().NewIncomingMsg(channel, urn, form.Msg, "", clog).WithReceivedOn(date).WithContactName(form.Name).WithURNAuthTokens(authTokens)

	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{dbMsg}, w, r, clog)
}

type registerForm struct {
	URN      string `name:"urn"       validate:"required"`
	FCMToken string `name:"fcm_token" validate:"required"`
	Name     string `name:"name"`
}

// registerContact is our HTTP handler function for when a contact is registered (or renewed)
func (h *handler) registerContact(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	form := &registerForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// create our URN
	urn, err := urns.New(urns.Firebase, form.URN)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// create our contact
	contact, err := h.Backend().GetContact(ctx, channel, urn, map[string]string{"default": form.FCMToken}, form.Name, clog)
	if err != nil {
		return nil, err
	}

	// return our contact UUID
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(map[string]string{"contact_uuid": string(contact.UUID())})
	return nil, err
}

type mtPayload struct {
	Data struct {
		Type          string   `json:"type"`
		Title         string   `json:"title"`
		Message       string   `json:"message"`
		MessageID     int64    `json:"message_id"`
		SessionStatus string   `json:"session_status"`
		QuickReplies  []string `json:"quick_replies,omitempty"`
	} `json:"data"`
	Notification *mtNotification `json:"notification,omitempty"`
	Token        string          `json:"token"`
	Android      struct {
		Priority string `json:"priority"`
	} `json:"android,omitempty"`
}

type mtAPIKeyPayload struct {
	Data struct {
		Type          string   `json:"type"`
		Title         string   `json:"title"`
		Message       string   `json:"message"`
		MessageID     int64    `json:"message_id"`
		SessionStatus string   `json:"session_status"`
		QuickReplies  []string `json:"quick_replies,omitempty"`
	} `json:"data"`
	Notification     *mtNotification `json:"notification,omitempty"`
	ContentAvailable bool            `json:"content_available"`
	To               string          `json:"to"`
	Priority         string          `json:"priority"`
}

type mtNotification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	fcmKey := msg.Channel().StringConfigForKey(configKey, "")

	if fcmKey != "" {
		return h.sendWithAPIKey(ctx, msg, res, clog)
	}

	return h.sendWithCredsJSON(ctx, msg, res, clog)
}

func (h *handler) sendWithAPIKey(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	title := msg.Channel().StringConfigForKey(configTitle, "")
	fcmKey := msg.Channel().StringConfigForKey(configKey, "")
	if title == "" || fcmKey == "" {
		return courier.ErrChannelConfig
	}

	configNotification := msg.Channel().ConfigForKey(configNotification, false)
	notification, _ := configNotification.(bool)
	msgParts := make([]string, 0)
	if msg.Text() != "" {
		msgParts = handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength)
	}

	for i, part := range msgParts {
		payload := mtAPIKeyPayload{}

		payload.Data.Type = "rapidpro"
		payload.Data.Title = title
		payload.Data.Message = part
		payload.Data.MessageID = int64(msg.ID())
		payload.Data.SessionStatus = msg.SessionStatus()

		// include any quick replies on the last piece we send
		if i == len(msgParts)-1 {
			payload.Data.QuickReplies = msg.QuickReplies()
		}

		payload.To = msg.URNAuth()
		payload.Priority = "high"

		if notification {
			payload.Notification = &mtNotification{
				Title: title,
				Body:  part,
			}
			payload.ContentAvailable = true
		}

		jsonPayload := jsonx.MustMarshal(payload)

		req, err := http.NewRequest(http.MethodPost, sendURL, bytes.NewReader(jsonPayload))
		if err != nil {
			return err
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("key=%s", fcmKey))

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 == 5 {
			return courier.ErrConnectionFailed
		} else if resp.StatusCode/100 != 2 {
			return courier.ErrResponseStatus
		}

		// was this successful
		success, _ := jsonparser.GetInt(respBody, "success")
		if success != 1 {
			return courier.ErrResponseUnexpected
		}

		externalID, err := jsonparser.GetInt(respBody, "multicast_id")
		if err != nil {
			return courier.ErrResponseUnexpected
		}
		res.AddExternalID(fmt.Sprintf("%d", externalID))

	}

	return nil
}

func (h *handler) sendWithCredsJSON(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	title := msg.Channel().StringConfigForKey(configTitle, "")

	credentialsFile := msg.Channel().ConfigForKey(configCredentialsFile, nil)
	if credentialsFile == nil {
		return courier.ErrChannelConfig
	}

	credentialsFileJSON, ok := credentialsFile.(map[string]string)
	if !ok {
		return courier.ErrChannelConfig
	}

	accessToken, err := h.getAccessToken(ctx, msg.Channel(), clog)
	if err != nil {
		return err
	}

	configNotification := msg.Channel().ConfigForKey(configNotification, false)
	notification, _ := configNotification.(bool)
	msgParts := make([]string, 0)
	if msg.Text() != "" {
		msgParts = handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength)
	}
	sendURL := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", credentialsFileJSON["project_id"])

	for i, part := range msgParts {
		payload := mtPayload{}

		payload.Data.Type = "rapidpro"
		payload.Data.Title = title
		payload.Data.Message = part
		payload.Data.MessageID = int64(msg.ID())
		payload.Data.SessionStatus = msg.SessionStatus()

		if i == len(msgParts)-1 {
			payload.Data.QuickReplies = msg.QuickReplies()
		}

		payload.Token = msg.URNAuth()
		payload.Android.Priority = "high"

		if notification {
			payload.Notification = &mtNotification{
				Title: title,
				Body:  part,
			}
		}

		jsonPayload := jsonx.MustMarshal(payload)

		req, err := http.NewRequest(http.MethodPost, sendURL, bytes.NewReader(jsonPayload))
		if err != nil {
			return err
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 == 5 {
			return courier.ErrConnectionFailed
		} else if resp.StatusCode/100 != 2 {
			return courier.ErrResponseStatus
		}

		responseName, err := jsonparser.GetString(respBody, "name")
		if err != nil {
			return courier.ErrResponseUnexpected
		}

		if !strings.Contains(responseName, fmt.Sprintf("projects/%s/messages/", credentialsFileJSON["project_id"])) {
			return courier.ErrResponseUnexpected
		}
		externalID := strings.TrimLeft(responseName, fmt.Sprintf("projects/%s/messages/", credentialsFileJSON["project_id"]))
		if externalID == "" {
			return courier.ErrResponseUnexpected
		}

		res.AddExternalID(externalID)

	}

	return nil
}

func (h *handler) getAccessToken(ctx context.Context, channel courier.Channel, clog *courier.ChannelLog) (string, error) {
	rc := h.Backend().RedisPool().Get()
	defer rc.Close()

	tokenKey := fmt.Sprintf("channel-token:%s", channel.UUID())

	h.fetchTokenMutex.Lock()
	defer h.fetchTokenMutex.Unlock()

	token, err := redis.String(rc.Do("GET", tokenKey))
	if err != nil && err != redis.ErrNil {
		return "", fmt.Errorf("error reading cached access token: %w", err)
	}

	if token != "" {
		return token, nil
	}

	token, expires, err := h.fetchAccessToken(ctx, channel, clog)
	if err != nil {
		return "", fmt.Errorf("error fetching new access token: %w", err)
	}

	_, err = rc.Do("SET", tokenKey, token, "EX", int(expires/time.Second))
	if err != nil {
		return "", fmt.Errorf("error updating cached access token: %w", err)
	}

	return token, nil
}

// fetchAccessToken tries to fetch a new token for our channel, setting the result in redis
func (h *handler) fetchAccessToken(ctx context.Context, channel courier.Channel, clog *courier.ChannelLog) (string, time.Duration, error) {

	credentialsFile := channel.StringConfigForKey(configCredentialsFile, "")
	if credentialsFile == "" {
		return "", 0, courier.ErrChannelConfig
	}

	var credentialsFileJSON map[string]string

	err := json.Unmarshal([]byte(credentialsFile), &credentialsFileJSON)
	if err != nil {
		return "", 0, courier.ErrChannelConfig
	}

	sendURL := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", credentialsFileJSON["project_id"])

	ts, err := idtoken.NewTokenSource(ctx, sendURL, option.WithCredentialsJSON([]byte(credentialsFile)))
	if err != nil {
		return "", 0, fmt.Errorf("failed to create NewTokenSource: %w", err)
	}

	token, err := ts.Token()
	if err != nil {
		return "", 0, err
	}

	return token.AccessToken, token.Expiry.UTC().Sub(time.Now().UTC()), nil
}
