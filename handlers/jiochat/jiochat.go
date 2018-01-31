package jiochat

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/buger/jsonparser"
	"github.com/garyburd/redigo/redis"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

var maxMsgLength = 1600
var jiochatFetchTimeout = time.Second * 2

const configJiochatAppID = "jiochat_app_id"
const configJiochatAppSecret = "jiochat_app_secret"

var jiochatURL = "https://channels.jiochat.com"

func init() {
	courier.RegisterHandler(newHandler(&jiochatClient{}))
}

type handler struct {
	handlers.BaseHandler
	jiochatClient jiochatAccess
}

func newHandler(ja jiochatAccess) courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("JC"), "Jiochat"), ja}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	err := s.AddHandlerRoute(h, http.MethodGet, "", h.VerifyURL)
	if err != nil {
		return err
	}

	err = s.AddHandlerRoute(h, http.MethodPost, "rcv/msg/message", h.ReceiveMessage)
	if err != nil {
		return err
	}

	err = s.AddHandlerRoute(h, http.MethodPost, "rcv/event/menu", h.ReceiveMessage)
	if err != nil {
		return err
	}

	return s.AddHandlerRoute(h, http.MethodPost, "rcv/event/follow", h.ReceiveMessage)
}

type verifyRequest struct {
	Signature string `name:"signature"`
	Timestamp string `name:"timestamp"`
	Nonce     string `name:"nonce"`
	EchoStr   string `name:"echostr"`
}

// VerifyURL is our HTTP handler function for Jiochat config URL verification callbacks
func (h *handler) VerifyURL(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	jcVerify := &verifyRequest{}
	err := handlers.DecodeAndValidateQueryParams(jcVerify, r)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	dictOrder := []string{channel.StringConfigForKey(configJiochatAppSecret, ""), jcVerify.Timestamp, jcVerify.Nonce}
	sort.Sort(sort.StringSlice(dictOrder))

	combinedParams := strings.Join(dictOrder, "")

	hash := sha1.New()
	hash.Write([]byte(combinedParams))
	encoded := hex.EncodeToString(hash.Sum(nil))

	ResponseText := "unknown request"
	StatusCode := 400

	if encoded == jcVerify.Signature {
		ResponseText = jcVerify.EchoStr
		StatusCode = 200
		go h.doFetchAccessToken(channel)
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(StatusCode)
	_, err = fmt.Fprint(w, ResponseText)
	return nil, err
}

type moMsg struct {
	FromUsername string `json:"FromUserName"    validate:"required"`
	MsgType      string `json:"MsgType"         validate:"required"`
	CreateTime   int64  `json:"CreateTime"`
	MsgID        int64  `json:"MsgId"`
	Event        string `json:"Event"`
	Content      string `json:"Content"`
	MediaID      string `json:"MediaId"`
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *handler) ReceiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	jcRequest := &moMsg{}
	err := handlers.DecodeAndValidateJSON(jcRequest, r)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	if jcRequest.MsgID == 0 && jcRequest.Event == "" {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf("missing parameters, must have either 'MsgId' or 'Event'"))
	}

	date := time.Unix(jcRequest.CreateTime, 0).UTC()

	urn := urns.NewURNFromParts(urns.JiochatScheme, jcRequest.FromUsername, "")

	if jcRequest.MsgType == "event" && jcRequest.Event == "subscribe" {

		// build the channel event
		channelEvent := h.Backend().NewChannelEvent(channel, courier.NewConversation, urn)

		err := h.Backend().WriteChannelEvent(ctx, channelEvent)
		if err != nil {
			return nil, err
		}

		return []courier.Event{channelEvent}, courier.WriteChannelEventSuccess(ctx, w, r, channelEvent)
	}

	if jcRequest.MsgType == "event" {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf("unknown event"))
	}

	// create our message
	msg := h.Backend().NewIncomingMsg(channel, urn, jcRequest.Content).WithExternalID(fmt.Sprintf("%d", jcRequest.MsgID)).WithReceivedOn(date)
	if jcRequest.MsgType == "image" || jcRequest.MsgType == "video" || jcRequest.MsgType == "voice" {
		mediaURL := resolveMediaID(jcRequest.MediaID)
		msg.WithAttachment(mediaURL)
	}

	err = h.Backend().WriteMsg(ctx, msg)
	if err != nil {
		return nil, err
	}

	return []courier.Event{msg}, courier.WriteMsgSuccess(ctx, w, r, []courier.Msg{msg})
}

func resolveMediaID(mediaID string) string {
	mediaURL, _ := url.Parse(fmt.Sprintf("%s/%s", jiochatURL, "media/download.action"))
	mediaURL.RawQuery = url.Values{"media_id": []string{mediaID}}.Encode()
	return mediaURL.String()
}

type jiochatAccess interface {
	fetchAccessToken(*handler, courier.Channel) error
	getAccessToken(*handler, courier.Channel) string
}

type jiochatClient struct{}

type refreshTokenData struct {
	GrantType    string `json:"grant_type"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

func (jCli *jiochatClient) fetchAccessToken(h *handler, channel courier.Channel) error {
	time.Sleep(jiochatFetchTimeout)

	tokenURL, _ := url.Parse(fmt.Sprintf("%s/%s", jiochatURL, "auth/token.action"))

	refreshTokenData := &refreshTokenData{
		GrantType:    "client_credentials",
		ClientID:     channel.StringConfigForKey(configJiochatAppID, ""),
		ClientSecret: channel.StringConfigForKey(configJiochatAppSecret, ""),
	}

	jsonBody, err := json.Marshal(refreshTokenData)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, tokenURL.String(), bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	rr, err := utils.MakeHTTPRequest(req)
	if err != nil {
		return err
	}

	accessToken, err := jsonparser.GetString([]byte(rr.Body), "access_token")
	if err != nil {
		return err
	}

	rc := h.Backend().RedisPool().Get()
	defer rc.Close()
	cacheKey := fmt.Sprintf("jiochat_channel_access_token:%s", channel.UUID().String())

	_, err = rc.Do("set", cacheKey, accessToken, 7200)
	return err
}

func (jCli *jiochatClient) getAccessToken(h *handler, channel courier.Channel) string {
	rc := h.Backend().RedisPool().Get()
	defer rc.Close()
	cacheKey := fmt.Sprintf("jiochat_channel_access_token:%s", channel.UUID().String())
	accessToken, _ := redis.String(rc.Do("GET", cacheKey))
	return accessToken
}

func (h *handler) findAccessToken(channel courier.Channel) string {
	return h.jiochatClient.getAccessToken(h, channel)
}

func (h *handler) doFetchAccessToken(channel courier.Channel) error {
	return h.jiochatClient.fetchAccessToken(h, channel)
}

type mtMsg struct {
	MsgType string `json:"msgtype"`
	ToUser  string `json:"touser"`
	Text    struct {
		Content string `json:"content"`
	} `json:"text"`
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	accessToken := h.findAccessToken(msg.Channel())

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	parts := handlers.SplitMsg(handlers.GetTextAndAttachments(msg), maxMsgLength)
	for _, part := range parts {
		jcMsg := &mtMsg{}
		jcMsg.MsgType = "text"
		jcMsg.ToUser = msg.URN().Path()
		jcMsg.Text.Content = part

		requestBody := new(bytes.Buffer)
		json.NewEncoder(requestBody).Encode(jcMsg)

		// build our request
		req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/%s", jiochatURL, "custom/custom_send.action"), requestBody)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
		if err != nil {
			return nil, err
		}

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
	accessToken := h.findAccessToken(channel)

	_, path, _ := urn.ToParts()

	form := url.Values{
		"openid": []string{path},
	}

	reqURL, _ := url.Parse(fmt.Sprintf("%s/%s", jiochatURL, "user/info.action"))
	reqURL.RawQuery = form.Encode()

	req, err := http.NewRequest(http.MethodGet, reqURL.String(), nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	if err != nil {
		return nil, err
	}

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
	parsedURL, err := url.Parse(attachmentURL)
	if err != nil {
		return nil, err
	}

	accessToken := h.findAccessToken(channel)

	// first fetch our media
	req, err := http.NewRequest("GET", parsedURL.String(), nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	if err != nil {
		return nil, err
	}

	return req, nil
}
