package jiochat

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/buger/jsonparser"
	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/pkg/errors"
)

var (
	sendURL      = "https://channels.jiochat.com"
	maxMsgLength = 1600
)

const (
	configAppID     = "jiochat_app_id"
	configAppSecret = "jiochat_app_secret"
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
		BaseHandler:     handlers.NewBaseHandler(courier.ChannelType("JC"), "Jiochat"),
		fetchTokenMutex: sync.Mutex{},
	}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodGet, "", courier.ChannelLogTypeWebhookVerify, h.VerifyURL)
	s.AddHandlerRoute(h, http.MethodPost, "rcv/msg/message", courier.ChannelLogTypeMsgReceive, handlers.JSONPayload(h, h.receiveMessage))
	s.AddHandlerRoute(h, http.MethodPost, "rcv/event/menu", courier.ChannelLogTypeEventReceive, handlers.JSONPayload(h, h.receiveMessage))
	s.AddHandlerRoute(h, http.MethodPost, "rcv/event/follow", courier.ChannelLogTypeEventReceive, handlers.JSONPayload(h, h.receiveMessage))
	return nil
}

type verifyForm struct {
	Signature string `name:"signature"`
	Timestamp string `name:"timestamp"`
	Nonce     string `name:"nonce"`
	EchoStr   string `name:"echostr"`
}

// VerifyURL is our HTTP handler function for Jiochat config URL verification callbacks
func (h *handler) VerifyURL(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	form := &verifyForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	dictOrder := []string{channel.StringConfigForKey(configAppSecret, ""), form.Timestamp, form.Nonce}
	sort.Strings(dictOrder)

	combinedParams := strings.Join(dictOrder, "")

	hash := sha1.New()
	hash.Write([]byte(combinedParams))
	encoded := hex.EncodeToString(hash.Sum(nil))

	ResponseText := "unknown request"
	StatusCode := 400

	if encoded == form.Signature {
		ResponseText = form.EchoStr
		StatusCode = 200
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(StatusCode)
	_, err = fmt.Fprint(w, ResponseText)
	return nil, err
}

type moPayload struct {
	FromUsername string `json:"FromUserName"    validate:"required"`
	MsgType      string `json:"MsgType"         validate:"required"`
	CreateTime   int64  `json:"CreateTime"`
	MsgID        string `json:"MsgId"`
	Event        string `json:"Event"`
	Content      string `json:"Content"`
	MediaID      string `json:"MediaId"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *moPayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	if payload.MsgID == "" && payload.Event == "" {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("missing parameters, must have either 'MsgId' or 'Event'"))
	}

	date := time.Unix(payload.CreateTime/1000, payload.CreateTime%1000*1000000).UTC()
	urn, err := urns.NewURNFromParts(urns.JiochatScheme, payload.FromUsername, "", "")
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// subscribe event, trigger a new conversation
	if payload.MsgType == "event" && payload.Event == "subscribe" {
		channelEvent := h.Backend().NewChannelEvent(channel, courier.EventTypeNewConversation, urn, clog)

		err := h.Backend().WriteChannelEvent(ctx, channelEvent, clog)
		if err != nil {
			return nil, err
		}

		return []courier.Event{channelEvent}, courier.WriteChannelEventSuccess(w, channelEvent)
	}

	// unknown event type (we only deal with subscribe)
	if payload.MsgType == "event" {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "unknown event type")
	}

	// create our message
	msg := h.Backend().NewIncomingMsg(channel, urn, payload.Content, payload.MsgID, clog).WithReceivedOn(date)
	if payload.MsgType == "image" || payload.MsgType == "video" || payload.MsgType == "voice" {
		mediaURL := buildMediaURL(payload.MediaID)
		msg.WithAttachment(mediaURL)
	}

	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
}

func buildMediaURL(mediaID string) string {
	mediaURL, _ := url.Parse(fmt.Sprintf("%s/%s", sendURL, "media/download.action"))
	mediaURL.RawQuery = url.Values{"media_id": []string{mediaID}}.Encode()
	return mediaURL.String()
}

type mtPayload struct {
	MsgType string `json:"msgtype"`
	ToUser  string `json:"touser"`
	Text    struct {
		Content string `json:"content"`
	} `json:"text"`
}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	accessToken, err := h.getAccessToken(ctx, msg.Channel(), clog)
	if err != nil {
		return nil, err
	}

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)
	parts := handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength)
	for _, part := range parts {
		jcMsg := &mtPayload{}
		jcMsg.MsgType = "text"
		jcMsg.ToUser = msg.URN().Path()
		jcMsg.Text.Content = part

		requestBody := &bytes.Buffer{}
		json.NewEncoder(requestBody).Encode(jcMsg)

		// build our request
		req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/%s", sendURL, "custom/custom_send.action"), requestBody)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

		resp, _, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 != 2 {
			return status, nil
		}

		status.SetStatus(courier.MsgStatusWired)
	}

	return status, nil
}

// DescribeURN handles Jiochat contact details
func (h *handler) DescribeURN(ctx context.Context, channel courier.Channel, urn urns.URN, clog *courier.ChannelLog) (map[string]string, error) {
	accessToken, err := h.getAccessToken(ctx, channel, clog)
	if err != nil {
		return nil, err
	}

	_, path, _, _ := urn.ToParts()

	form := url.Values{
		"openid": []string{path},
	}

	reqURL, _ := url.Parse(fmt.Sprintf("%s/%s", sendURL, "user/info.action"))
	reqURL.RawQuery = form.Encode()

	req, _ := http.NewRequest(http.MethodGet, reqURL.String(), nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return nil, errors.New("unable to look up contact data")
	}

	nickname, _ := jsonparser.GetString(respBody, "nickname")
	return map[string]string{"name": nickname}, nil
}

func (h *handler) RedactValues(ch courier.Channel) []string {
	return []string{
		ch.StringConfigForKey(configAppSecret, ""),
	}
}

// BuildAttachmentRequest download media for message attachment
func (h *handler) BuildAttachmentRequest(ctx context.Context, b courier.Backend, channel courier.Channel, attachmentURL string, clog *courier.ChannelLog) (*http.Request, error) {
	parsedURL, err := url.Parse(attachmentURL)
	if err != nil {
		return nil, err
	}

	accessToken, err := h.getAccessToken(ctx, channel, clog)
	if err != nil {
		return nil, err
	}

	// first fetch our media
	req, _ := http.NewRequest(http.MethodGet, parsedURL.String(), nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	return req, nil
}

var _ courier.AttachmentRequestBuilder = (*handler)(nil)

func (h *handler) getAccessToken(ctx context.Context, channel courier.Channel, clog *courier.ChannelLog) (string, error) {
	rc := h.Backend().RedisPool().Get()
	defer rc.Close()

	tokenKey := fmt.Sprintf("channel-token:%s", channel.UUID())

	h.fetchTokenMutex.Lock()
	defer h.fetchTokenMutex.Unlock()

	token, err := redis.String(rc.Do("GET", tokenKey))
	if err != nil && err != redis.ErrNil {
		return "", errors.Wrap(err, "error reading cached access token")
	}

	if token != "" {
		return token, nil
	}

	token, expires, err := h.fetchAccessToken(ctx, channel, clog)
	if err != nil {
		return "", errors.Wrap(err, "error fetching new access token")
	}

	_, err = rc.Do("SET", tokenKey, token, "EX", int(expires/time.Second))
	if err != nil {
		return "", errors.Wrap(err, "error updating cached access token")
	}

	return token, nil
}

type fetchPayload struct {
	GrantType    string `json:"grant_type"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// fetchAccessToken tries to fetch a new token for our channel
func (h *handler) fetchAccessToken(ctx context.Context, channel courier.Channel, clog *courier.ChannelLog) (string, time.Duration, error) {
	tokenURL, _ := url.Parse(fmt.Sprintf("%s/%s", sendURL, "auth/token.action"))
	payload := &fetchPayload{
		GrantType:    "client_credentials",
		ClientID:     channel.StringConfigForKey(configAppID, ""),
		ClientSecret: channel.StringConfigForKey(configAppSecret, ""),
	}

	req, err := http.NewRequest(http.MethodPost, tokenURL.String(), bytes.NewReader(jsonx.MustMarshal(payload)))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return "", 0, err
	}

	token, err := jsonparser.GetString(respBody, "access_token")
	if err != nil {
		clog.Error(courier.ErrorResponseValueMissing("access_token"))
		return "", 0, err
	}

	expiration, err := jsonparser.GetInt(respBody, "expires_in")
	if err != nil || expiration == 0 {
		expiration = 7200
	}

	return token, time.Second * time.Duration(expiration), nil
}
