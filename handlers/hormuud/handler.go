package hormuud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/buger/jsonparser"
	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/urns"
)

var (
	maxMsgLength = 160
	tokenURL     = "https://smsapi.hormuud.com/token"
	sendURL      = "https://smsapi.hormuud.com/api/SendSMS"
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("HM"), "Hormuud")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
	return nil
}

type moPayload struct {
	Sender      string `validate:"required"`
	MessageText string
	ShortCode   string `validate:"required"`
	TimeSent    int64  // ignored as not reliable or accurate (e.g. 20230418, 202304172)
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, c courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	payload := &moPayload{}
	err := handlers.DecodeAndValidateForm(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
	}

	urn, err := urns.ParsePhone(payload.Sender, c.Country(), true, false)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
	}

	msg := h.Backend().NewIncomingMsg(ctx, c, urn, payload.MessageText, "", clog)
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
}

type mtPayload struct {
	Mobile   string `json:"mobile"`
	Message  string `json:"message"`
	SenderID string `json:"senderid"`
	MType    int    `json:"mType"`
	EType    int    `json:"eType"`
	UDH      string `json:"UDH"`
}

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if username == "" || password == "" {
		return courier.ErrChannelConfig
	}

	token, err := h.FetchToken(ctx, msg.Channel(), msg, username, password, clog)
	if err != nil {
		return err
	}

	parts := handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength)
	for _, part := range parts {
		payload := &mtPayload{}
		payload.Mobile = strings.TrimPrefix(msg.URN().Path(), "+")
		payload.Message = part
		payload.SenderID = msg.Channel().Address()
		payload.MType = -1
		payload.EType = -1
		payload.UDH = ""

		requestBody := &bytes.Buffer{}
		json.NewEncoder(requestBody).Encode(payload)

		// build our request
		req, err := http.NewRequest(http.MethodPost, sendURL, requestBody)
		if err != nil {
			return err
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 == 5 {
			return courier.ErrConnectionFailed
		} else if resp.StatusCode/100 != 2 {
			return courier.ErrResponseStatus
		}

		// try to get the message id out
		id, _ := jsonparser.GetString(respBody, "Data", "MessageID")
		if id != "" {
			res.AddExternalID(id)
		}
	}
	return nil
}

// FetchToken gets the current token for this channel, either from Redis if cached or by requesting it
func (h *handler) FetchToken(ctx context.Context, channel courier.Channel, msg courier.MsgOut, username, password string, clog *courier.ChannelLog) (string, error) {
	// first check whether we have it in redis
	var token string
	h.WithValkeyConn(func(rc redis.Conn) {
		token, _ = redis.String(rc.Do("GET", fmt.Sprintf("hm_token_%s", channel.UUID())))
	})

	// got a token, use it
	if token != "" {
		return token, nil
	}

	form := url.Values{
		"Username":   []string{username},
		"Password":   []string{password},
		"grant_type": []string{"password"},
	}

	// build our request
	req, _ := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil {
		return "", courier.ErrConnectionFailed
	} else if resp.StatusCode/100 != 2 {
		return "", courier.ErrResponseStatus
	}

	token, err = jsonparser.GetString(respBody, "access_token")
	if err != nil {
		return "", fmt.Errorf("error getting access_token from response: %w", err)
	}
	if token == "" {
		return "", errors.New("no access token returned")
	}

	expiration, err := jsonparser.GetInt(respBody, "expires_in")

	if err != nil {
		expiration = 3600
	}

	// we got a token, cache it to redis with an expiration from the response(we default to 60 minutes)
	h.WithValkeyConn(func(rc redis.Conn) {
		_, err = rc.Do("SETEX", fmt.Sprintf("hm_token_%s", channel.UUID()), expiration, token)
		if err != nil {
			slog.Error("error caching HM access token", "error", err)
		}
	})

	return token, nil
}
