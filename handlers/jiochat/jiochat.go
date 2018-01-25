package jiochat

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
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

	dictOrder := []string{channel.StringConfigForKey(courier.ConfigSecret, ""), jcVerify.Timestamp, jcVerify.Nonce}
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

var mediaDownloadURL = "https://channels.jiochat.com/media/download.action"

func resolveMediaID(mediaID string) string {
	mediaURL, _ := url.Parse(mediaDownloadURL)
	mediaURL.RawQuery = url.Values{"media_id": []string{mediaID}}.Encode()
	return mediaURL.String()
}

var luaAccessToken = redis.NewScript(2, `-- KEYS: [LockKey, CacheKey]
	-- Do we have the lock for the refresh token happening?
	local lock

	-- Wait until the lock is released
	repeat
		lock = redis.call("get", KEYS[1])
	until (not lock)

	-- Get the current access token
	local accessToken = redis.call("get", KEYS[2])

	-- Return the access token
	return accessToken
`)

func (h *handler) getAccessToken(channel courier.Channel) string {
	rc := h.Backend().RedisPool().Get()
	defer rc.Close()

	lockName := fmt.Sprintf("jiochat_channel_access_token:refresh-lock:%s", channel.UUID().String())
	cacheKey := fmt.Sprintf("jiochat_channel_access_token:%s", channel.UUID().String())

	accessToken, _ := redis.String(luaAccessToken.Do(rc, lockName, cacheKey))

	return accessToken
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	return nil, fmt.Errorf("JC sending via Courier not yet implemented")
}

var userDetailsURL = "https://channels.jiochat.com/user/info.action"

// DescribeURN handles Jiochat contact details
func (h *handler) DescribeURN(ctx context.Context, channel courier.Channel, urn urns.URN) (map[string]string, error) {
	accessToken := h.getAccessToken(channel)

	_, path, _ := urn.ToParts()

	form := url.Values{
		"openid": []string{path},
	}

	reqURL, _ := url.Parse(userDetailsURL)
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
	nickname, _ := jsonparser.GetString(rr.Body, "nickname")

	return map[string]string{"name": nickname}, nil
}

// BuildDownloadMediaRequest download media for message attachment
func (h *handler) BuildDownloadMediaRequest(ctx context.Context, channel courier.Channel, attachmentURL string) (*http.Request, error) {
	parsedURL, err := url.Parse(attachmentURL)
	if err != nil {
		return nil, err
	}

	accessToken := h.getAccessToken(channel)

	// first fetch our media
	req, err := http.NewRequest("GET", parsedURL.String(), nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	if err != nil {
		return nil, err
	}

	return req, nil
}
