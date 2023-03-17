package mtn

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/buger/jsonparser"
	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/pkg/errors"
)

var (
	sendURL      = "https://api.mtn.com/v2/messages/sms/outbound"
	maxMsgLength = 160
	tokenURL     = "https://api.mtn.com/v1/oauth/access_token?grant_type=client_credentials"
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
		BaseHandler:     handlers.NewBaseHandler(courier.ChannelType("MTN"), "MTN Developer Portal"),
		fetchTokenMutex: sync.Mutex{},
	}
}

// Initialize implements courier.ChannelHandler
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodPost, "status", h.receiveStatus)
	return nil
}

var statusMapping = map[string]courier.MsgStatusValue{
	"DeliveredToTerminal": courier.MsgDelivered,
	"DeliveryUncertain":   courier.MsgSent,
	"DeliveryImpossible":  courier.MsgErrored,
	"DeliveredToNetwork":  courier.MsgSent,

	// no changes
	"MessageWaiting":                   courier.MsgWired,
	"DeliveryNotificationNotSupported": courier.MsgWired,
}

type statusPayload struct {
	RequestID      string `json:"requestId"`
	DeliveryStatus []struct {
		ReceiverAddress string `json:"receiverAddress"`
		Status          string `json:"status"`
	} `json:"deliveryStatus"`
}

// receiveStatus is our HTTP handler function for status updates
func (h *handler) receiveStatus(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	payload := &statusPayload{}
	err := handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}
	msgStatus, found := statusMapping[payload.DeliveryStatus[0].Status]
	if !found {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r,
			fmt.Errorf("unknown status '%s'", payload.DeliveryStatus[0].Status))
	}

	if msgStatus == courier.MsgWired {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "no status changed, ignored")
	}

	// write our status
	status := h.Backend().NewMsgStatusForExternalID(channel, payload.RequestID, msgStatus, clog)
	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

type moPayload struct {
	From    string `json:"senderAddress"`
	To      string `json:"receiverAddress"`
	Message string `json:"message"`
	Created int64  `json:"created"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	payload := &moPayload{}

	err := handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	date := time.Unix(payload.Created/1000, payload.Created%1000*1000000).UTC()
	urn, err := handlers.StrictTelForCountry(payload.From, channel.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// create our message
	msg := h.Backend().NewIncomingMsg(channel, urn, payload.Message, clog).WithReceivedOn(date)
	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r, clog)
}

type mtPayload struct {
	From    string   `json:"senderAddress"`
	To      []string `json:"receiverAddress"`
	Message string   `json:"message"`
}

// Send implements courier.ChannelHandler
func (h *handler) Send(ctx context.Context, msg courier.Msg, clog *courier.ChannelLog) (courier.MsgStatus, error) {
	accessToken, err := h.getAccessToken(ctx, msg.Channel(), clog)
	if err != nil {
		return nil, err
	}

	partSendURL, _ := url.Parse(sendURL)

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored, clog)
	parts := handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength)
	for i, part := range parts {
		mtMsg := &mtPayload{}
		mtMsg.From = strings.TrimPrefix(msg.Channel().Address(), "+")
		mtMsg.To = []string{strings.TrimPrefix(msg.URN().Path(), "+")}
		mtMsg.Message = part

		requestBody := &bytes.Buffer{}
		json.NewEncoder(requestBody).Encode(mtMsg)

		// build our request
		req, err := http.NewRequest(http.MethodPost, partSendURL.String(), requestBody)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", accessToken))

		resp, respBody, err := handlers.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 != 2 {
			return status, nil
		}

		externalID, err := jsonparser.GetString(respBody, "transactionId")
		if err != nil {
			clog.Error(courier.ErrorResponseValueMissing("transactionId"))
			return status, nil
		}

		// if this is our first message, record the external id
		if i == 0 {
			status.SetExternalID(externalID)
			status.SetStatus(courier.MsgWired)
		}

	}

	return status, nil
}

func (h *handler) RedactValues(ch courier.Channel) []string {
	return []string{
		ch.StringConfigForKey(courier.ConfigAPIKey, ""),
		ch.StringConfigForKey(courier.ConfigAuthToken, ""),
	}
}

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

// fetchAccessToken tries to fetch a new token for our channel, setting the result in redis
func (h *handler) fetchAccessToken(ctx context.Context, channel courier.Channel, clog *courier.ChannelLog) (string, time.Duration, error) {
	form := url.Values{
		"client_id":     []string{channel.StringConfigForKey(courier.ConfigAPIKey, "")},
		"client_secret": []string{channel.StringConfigForKey(courier.ConfigAuthToken, "")},
	}
	tokenURL, _ := url.Parse(tokenURL)

	req, _ := http.NewRequest(http.MethodPost, tokenURL.String(), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, respBody, err := handlers.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return "", 0, err
	}

	token, err := jsonparser.GetString(respBody, "access_token")
	if err != nil {
		clog.Error(courier.ErrorResponseValueMissing("access_token"))
		return "", 0, err
	}

	expirationStr, _ := jsonparser.GetString(respBody, "expires_in")
	expiration, err := strconv.Atoi(expirationStr)

	if err != nil || expiration == 0 {
		expiration = 3600
	}

	return token, time.Second * time.Duration(expiration), nil
}
