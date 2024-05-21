package wechat

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

	"errors"

	"github.com/buger/jsonparser"
	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/urns"
)

var (
	sendURL      = "https://api.weixin.qq.com/cgi-bin"
	maxMsgLength = 1600
)

const (
	configAppID     = "wechat_app_id"
	configAppSecret = "wechat_app_secret"
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
		BaseHandler:     handlers.NewBaseHandler(courier.ChannelType("WC"), "WeChat"),
		fetchTokenMutex: sync.Mutex{},
	}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodGet, "", courier.ChannelLogTypeWebhookVerify, h.VerifyURL)
	s.AddHandlerRoute(h, http.MethodPost, "", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
	return nil
}

type verifyForm struct {
	Signature string `name:"signature"`
	Timestamp string `name:"timestamp"`
	Nonce     string `name:"nonce"`
	EchoStr   string `name:"echostr"`
}

// VerifyURL is our HTTP handler function for WeChat config URL verification callbacks
func (h *handler) VerifyURL(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	form := &verifyForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	dictOrder := []string{channel.StringConfigForKey(courier.ConfigSecret, ""), form.Timestamp, form.Nonce}
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
	FromUsername string `xml:"FromUserName"    validate:"required"`
	MsgType      string `xml:"MsgType"         validate:"required"`
	CreateTime   int64  `xml:"CreateTime"`
	MsgID        string `xml:"MsgId"`
	Event        string `xml:"Event"`
	Content      string `xml:"Content"`
	MediaID      string `xml:"MediaId"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	payload := &moPayload{}
	err := handlers.DecodeAndValidateXML(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	if payload.MsgID == "" && payload.Event == "" {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("missing parameters, must have either 'MsgId' or 'Event'"))
	}

	date := time.Unix(payload.CreateTime/1000, payload.CreateTime%1000*1000000).UTC()
	urn, err := urns.New(urns.WeChat, payload.FromUsername)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// subscribe event, trigger a new conversation
	if payload.MsgType == "event" && payload.Event == "subscribe" {
		clog.SetType(courier.ChannelLogTypeEventReceive)

		channelEvent := h.Backend().NewChannelEvent(channel, courier.EventTypeNewConversation, urn, clog)

		err := h.Backend().WriteChannelEvent(ctx, channelEvent, clog)
		if err != nil {
			return nil, err
		}

		return []courier.Event{channelEvent}, courier.WriteChannelEventSuccess(w, channelEvent)
	}

	// unknown event type (we only deal with subscribe)
	if payload.MsgType == "event" {
		clog.SetType(courier.ChannelLogTypeEventReceive)

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

// WriteMsgSuccessResponse writes our response
func (h *handler) WriteMsgSuccessResponse(ctx context.Context, w http.ResponseWriter, msgs []courier.MsgIn) error {
	w.WriteHeader(200)
	_, err := fmt.Fprint(w, "") // WeChat expected empty string to not retry looking for passive reply
	return err
}

func buildMediaURL(mediaID string) string {
	mediaURL, _ := url.Parse(fmt.Sprintf("%s/%s", sendURL, "media/get"))
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

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	accessToken, err := h.getAccessToken(ctx, msg.Channel(), clog)
	if err != nil {
		return err
	}

	form := url.Values{
		"access_token": []string{accessToken},
	}
	partSendURL, _ := url.Parse(fmt.Sprintf("%s/%s", sendURL, "message/custom/send"))
	partSendURL.RawQuery = form.Encode()
	parts := handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength)
	for _, part := range parts {
		wcMsg := &mtPayload{}
		wcMsg.MsgType = "text"
		wcMsg.ToUser = msg.URN().Path()
		wcMsg.Text.Content = part

		requestBody := &bytes.Buffer{}
		json.NewEncoder(requestBody).Encode(wcMsg)

		// build our request
		req, err := http.NewRequest(http.MethodPost, partSendURL.String(), requestBody)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, _, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 == 5 {
			return courier.ErrConnectionFailed
		}
	}

	return nil
}

// DescribeURN handles WeChat contact details
func (h *handler) DescribeURN(ctx context.Context, channel courier.Channel, urn urns.URN, clog *courier.ChannelLog) (map[string]string, error) {
	accessToken, err := h.getAccessToken(ctx, channel, clog)
	if err != nil {
		return nil, err
	}

	_, path, _, _ := urn.ToParts()

	form := url.Values{
		"openid":       []string{path},
		"access_token": []string{accessToken},
	}

	reqURL, _ := url.Parse(fmt.Sprintf("%s/%s", sendURL, "user/info"))
	reqURL.RawQuery = form.Encode()

	req, _ := http.NewRequest(http.MethodGet, reqURL.String(), nil)

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return nil, errors.New("unable to look up contact data")
	}

	nickname, err := jsonparser.GetString(respBody, "nickname")
	if err != nil {
		return nil, err
	}

	return map[string]string{"name": nickname}, nil
}

func (h *handler) RedactValues(ch courier.Channel) []string {
	return []string{
		ch.StringConfigForKey(courier.ConfigSecret, ""),
		ch.StringConfigForKey(configAppSecret, ""),
	}
}

// BuildAttachmentRequest download media for message attachment
func (h *handler) BuildAttachmentRequest(ctx context.Context, b courier.Backend, channel courier.Channel, attachmentURL string, clog *courier.ChannelLog) (*http.Request, error) {
	accessToken, err := h.getAccessToken(ctx, channel, clog)
	if err != nil {
		return nil, err
	}

	parsedURL, err := url.Parse(attachmentURL)
	form := parsedURL.Query()
	form["access_token"] = []string{accessToken}
	parsedURL.RawQuery = form.Encode()
	if err != nil {
		return nil, err
	}

	// first fetch our media
	req, _ := http.NewRequest(http.MethodGet, parsedURL.String(), nil)
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
	form := url.Values{
		"grant_type": []string{"client_credential"},
		"appid":      []string{channel.StringConfigForKey(configAppID, "")},
		"secret":     []string{channel.StringConfigForKey(configAppSecret, "")},
	}
	tokenURL, _ := url.Parse(fmt.Sprintf("%s/%s", sendURL, "token"))
	tokenURL.RawQuery = form.Encode()

	req, _ := http.NewRequest(http.MethodGet, tokenURL.String(), nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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
