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
	"time"

	"github.com/buger/jsonparser"
	"github.com/garyburd/redigo/redis"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
	"github.com/sirupsen/logrus"
)

var (
	sendURL      = "https://api.weixin.qq.com/cgi-bin"
	maxMsgLength = 1600
	fetchTimeout = time.Second * 2
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
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("WC"), "WeChat")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodGet, "", h.VerifyURL)
	s.AddHandlerRoute(h, http.MethodPost, "", h.receiveMessage)
	return nil
}

type verifyForm struct {
	Signature string `name:"signature"`
	Timestamp string `name:"timestamp"`
	Nonce     string `name:"nonce"`
	EchoStr   string `name:"echostr"`
}

// VerifyURL is our HTTP handler function for WeChat config URL verification callbacks
func (h *handler) VerifyURL(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	form := &verifyForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	dictOrder := []string{channel.StringConfigForKey(courier.ConfigSecret, ""), form.Timestamp, form.Nonce}
	sort.Sort(sort.StringSlice(dictOrder))

	combinedParams := strings.Join(dictOrder, "")

	hash := sha1.New()
	hash.Write([]byte(combinedParams))
	encoded := hex.EncodeToString(hash.Sum(nil))

	ResponseText := "unknown request"
	StatusCode := 400

	if encoded == form.Signature {
		ResponseText = form.EchoStr
		StatusCode = 200
		go func() {
			time.Sleep(fetchTimeout)
			h.fetchAccessToken(ctx, channel)
		}()
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(StatusCode)
	_, err = fmt.Fprint(w, ResponseText)
	return nil, err
}

// fetchAccessToken tries to fetch a new token for our channel, setting the result in redis
func (h *handler) fetchAccessToken(ctx context.Context, channel courier.Channel) error {
	start := time.Now()
	logs := make([]*courier.ChannelLog, 0, 1)

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
	rr, err := utils.MakeHTTPRequest(req)
	if err != nil {
		duration := time.Now().Sub(start)
		logs = append(logs, courier.NewChannelLogFromError("failed to fetch access token", channel, courier.NilMsgID, duration, err))
		return h.Backend().WriteChannelLogs(ctx, logs)
	}

	accessToken, err := jsonparser.GetString(rr.Body, "access_token")
	if err != nil {
		duration := time.Now().Sub(start)
		logs = append(logs, courier.NewChannelLogFromError("invalid json", channel, courier.NilMsgID, duration, err))
		return h.Backend().WriteChannelLogs(ctx, logs)
	}
	expiration, err := jsonparser.GetInt(rr.Body, "expires_in")
	if err != nil {
		expiration = 7200
	}

	rc := h.Backend().RedisPool().Get()
	defer rc.Close()

	cacheKey := fmt.Sprintf("wechat_channel_access_token:%s", channel.UUID().String())
	_, err = rc.Do("set", cacheKey, accessToken, expiration)

	if err != nil {
		logrus.WithError(err).Error("error setting the access token to redis")
	}
	return err
}

func (h *handler) getAccessToken(channel courier.Channel) (string, error) {
	rc := h.Backend().RedisPool().Get()
	defer rc.Close()

	cacheKey := fmt.Sprintf("wechat_channel_access_token:%s", channel.UUID().String())
	accessToken, err := redis.String(rc.Do(http.MethodGet, cacheKey))
	if err != nil {
		return "", err
	}
	if accessToken == "" {
		return "", fmt.Errorf("no access token for channel")
	}

	return accessToken, nil
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
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	payload := &moPayload{}
	err := handlers.DecodeAndValidateXML(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	if payload.MsgID == "" && payload.Event == "" {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("missing parameters, must have either 'MsgId' or 'Event'"))
	}

	date := time.Unix(payload.CreateTime/1000, payload.CreateTime%1000*1000000).UTC()
	urn, err := urns.NewURNFromParts(urns.WeChatScheme, payload.FromUsername, "", "")
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// subscribe event, trigger a new conversation
	if payload.MsgType == "event" && payload.Event == "subscribe" {
		channelEvent := h.Backend().NewChannelEvent(channel, courier.NewConversation, urn)

		err := h.Backend().WriteChannelEvent(ctx, channelEvent)
		if err != nil {
			return nil, err
		}

		return []courier.Event{channelEvent}, courier.WriteChannelEventSuccess(ctx, w, r, channelEvent)
	}

	// unknown event type (we only deal with subscribe)
	if payload.MsgType == "event" {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "unknown event type")
	}

	// create our message
	msg := h.Backend().NewIncomingMsg(channel, urn, payload.Content).WithExternalID(payload.MsgID).WithReceivedOn(date)
	if payload.MsgType == "image" || payload.MsgType == "video" || payload.MsgType == "voice" {
		mediaURL := buildMediaURL(payload.MediaID)
		msg.WithAttachment(mediaURL)
	}

	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r)
}

// WriteMsgSuccessResponse writes our response
func (h *handler) WriteMsgSuccessResponse(ctx context.Context, w http.ResponseWriter, r *http.Request, msgs []courier.Msg) error {
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

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	accessToken, err := h.getAccessToken(msg.Channel())
	if err != nil {
		return nil, err
	}

	form := url.Values{
		"access_token": []string{accessToken},
	}

	partSendURL, _ := url.Parse(fmt.Sprintf("%s/%s", sendURL, "message/custom/send"))
	partSendURL.RawQuery = form.Encode()

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	parts := handlers.SplitMsg(handlers.GetTextAndAttachments(msg), maxMsgLength)
	for _, part := range parts {
		wcMsg := &mtPayload{}
		wcMsg.MsgType = "text"
		wcMsg.ToUser = msg.URN().Path()
		wcMsg.Text.Content = part

		requestBody := &bytes.Buffer{}
		json.NewEncoder(requestBody).Encode(wcMsg)

		// build our request
		req, _ := http.NewRequest(http.MethodPost, partSendURL.String(), requestBody)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		rr, err := utils.MakeHTTPRequest(req)

		// record our status and log
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
		status.AddLog(log)

		if err != nil {
			return status, err
		}
		status.SetStatus(courier.MsgWired)
	}

	return status, nil
}

// DescribeURN handles WeChat contact details
func (h *handler) DescribeURN(ctx context.Context, channel courier.Channel, urn urns.URN) (map[string]string, error) {
	accessToken, err := h.getAccessToken(channel)
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

	rr, err := utils.MakeHTTPRequest(req)
	if err != nil {
		return nil, fmt.Errorf("unable to look up contact data:%s\n%s", err, rr.Response)
	}
	nickname, err := jsonparser.GetString(rr.Body, "nickname")
	if err != nil {
		return nil, err
	}

	return map[string]string{"name": nickname}, nil
}

// BuildDownloadMediaRequest download media for message attachment
func (h *handler) BuildDownloadMediaRequest(ctx context.Context, b courier.Backend, channel courier.Channel, attachmentURL string) (*http.Request, error) {
	accessToken, err := h.getAccessToken(channel)
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
