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
	sendURL      = "https://channels.jiochat.com"
	maxMsgLength = 1600
	fetchTimeout = time.Second * 2
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
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("JC"), "Jiochat")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodGet, "", h.VerifyURL)
	s.AddHandlerRoute(h, http.MethodPost, "rcv/msg/message", h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodPost, "rcv/event/menu", h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodPost, "rcv/event/follow", h.receiveMessage)
	return nil
}

type verifyForm struct {
	Signature string `name:"signature"`
	Timestamp string `name:"timestamp"`
	Nonce     string `name:"nonce"`
	EchoStr   string `name:"echostr"`
}

// VerifyURL is our HTTP handler function for Jiochat config URL verification callbacks
func (h *handler) VerifyURL(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	form := &verifyForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	dictOrder := []string{channel.StringConfigForKey(configAppSecret, ""), form.Timestamp, form.Nonce}
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
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	payload := &moPayload{}
	err := handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

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

func buildMediaURL(mediaID string) string {
	mediaURL, _ := url.Parse(fmt.Sprintf("%s/%s", sendURL, "media/download.action"))
	mediaURL.RawQuery = url.Values{"media_id": []string{mediaID}}.Encode()
	return mediaURL.String()
}

type fetchPayload struct {
	GrantType    string `json:"grant_type"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// fetchAccessToken tries to fetch a new token for our channel, setting the result in redis
func (h *handler) fetchAccessToken(ctx context.Context, channel courier.Channel) error {
	start := time.Now()
	logs := make([]*courier.ChannelLog, 0, 1)

	tokenURL, _ := url.Parse(fmt.Sprintf("%s/%s", sendURL, "auth/token.action"))

	payload := &fetchPayload{
		GrantType:    "client_credentials",
		ClientID:     channel.StringConfigForKey(configAppID, ""),
		ClientSecret: channel.StringConfigForKey(configAppSecret, ""),
	}

	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, _ := http.NewRequest(http.MethodPost, tokenURL.String(), bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	rr, err := utils.MakeHTTPRequest(req)
	if err != nil {
		duration := time.Now().Sub(start)
		logs = append(logs, courier.NewChannelLogFromError("failed to fetch access token", channel, courier.NilMsgID, duration, err))
		return h.Backend().WriteChannelLogs(ctx, logs)
	}

	accessToken, err := jsonparser.GetString([]byte(rr.Body), "access_token")
	if err != nil {
		duration := time.Now().Sub(start)
		logs = append(logs, courier.NewChannelLogFromError("invalid json", channel, courier.NilMsgID, duration, err))
		return h.Backend().WriteChannelLogs(ctx, logs)
	}

	rc := h.Backend().RedisPool().Get()
	defer rc.Close()

	cacheKey := fmt.Sprintf("jiochat_channel_access_token:%s", channel.UUID().String())
	_, err = rc.Do("set", cacheKey, accessToken, 7200)

	if err != nil {
		logrus.WithError(err).Error("error setting the access token to redis")
	}
	return err
}

func (h *handler) getAccessToken(channel courier.Channel) (string, error) {
	rc := h.Backend().RedisPool().Get()
	defer rc.Close()

	cacheKey := fmt.Sprintf("jiochat_channel_access_token:%s", channel.UUID().String())
	accessToken, err := redis.String(rc.Do(http.MethodGet, cacheKey))
	if err != nil {
		return "", err
	}
	if accessToken == "" {
		return "", fmt.Errorf("no access token for channel")
	}

	return accessToken, nil
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

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	parts := handlers.SplitMsg(handlers.GetTextAndAttachments(msg), maxMsgLength)
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

// DescribeURN handles Jiochat contact details
func (h *handler) DescribeURN(ctx context.Context, channel courier.Channel, urn urns.URN) (map[string]string, error) {
	accessToken, err := h.getAccessToken(channel)
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

	rr, err := utils.MakeHTTPRequest(req)
	if err != nil {
		return nil, fmt.Errorf("unable to look up contact data:%s\n%s", err, rr.Response)
	}
	nickname, _ := jsonparser.GetString(rr.Body, "nickname")
	return map[string]string{"name": nickname}, nil
}

// BuildDownloadMediaRequest download media for message attachment
func (h *handler) BuildDownloadMediaRequest(ctx context.Context, b courier.Backend, channel courier.Channel, attachmentURL string) (*http.Request, error) {
	parsedURL, err := url.Parse(attachmentURL)
	if err != nil {
		return nil, err
	}

	accessToken, err := h.getAccessToken(channel)
	if err != nil {
		return nil, err
	}

	// first fetch our media
	req, _ := http.NewRequest(http.MethodGet, parsedURL.String(), nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	return req, nil
}
