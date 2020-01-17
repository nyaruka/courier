package vk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-errors/errors"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/urns"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"
)

const (
	scheme = "vk"

	eventTypeServerVerification = "confirmation"
	eventTypeNewMessage         = "message_new"

	configServerVerificationString = "callback_check_string"

	responseRequest = "ok"
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("VK"), "VK")}
}

func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveEvent)
	return nil
}

// base request body
type moPayload struct {
	Type      string `json:"type"`
	SecretKey string `json:"secret"`
}

// request body to VK's server verification
type moServerVerificationPayload struct {
	Type        string `json:"type"`
	CommunityId int64  `json:"group_id"`
}

// request body to new message
type moNewMessagePayload struct {
	Type   string `json:"type"`
	Object struct {
		Message struct {
			Id     int64  `json:"id"`
			Date   int64  `json:"date"`
			UserId int64  `json:"from_id"`
			Text   string `json:"text"`
		} `json:"message"`
	} `json:"object"`
}

// receiveEvent handles request event type
func (h *handler) receiveEvent(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	// read request body
	bodyBytes, err := ioutil.ReadAll(io.LimitReader(r.Body, 100000))

	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unable to read request body: %s", err))
	}
	// restore body to its original state
	r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
	payload := &moPayload{}

	if err := json.Unmarshal(bodyBytes, payload); err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}
	// check auth token before proceed
	secret := channel.StringConfigForKey(courier.ConfigSecret, "")

	fmt.Println(secret, payload.SecretKey)

	if payload.SecretKey != secret {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, errors.New("Wrong auth token"))
	}
	switch payload.Type {
	case eventTypeServerVerification:
		serverVerificationPayload := &moServerVerificationPayload{}

		if err := handlers.DecodeAndValidateJSON(serverVerificationPayload, r); err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}
		return h.verifyServer(ctx, channel, w, r, serverVerificationPayload)

	case eventTypeNewMessage:
		newMessagePayload := &moNewMessagePayload{}

		if err := handlers.DecodeAndValidateJSON(newMessagePayload, r); err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}
		return h.receiveMessage(ctx, channel, w, r, newMessagePayload)

	default:
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "Ignoring request, no message or server verification event")
	}
}

// verifyServer handles VK's callback verification
func (h *handler) verifyServer(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *moServerVerificationPayload) ([]courier.Event, error) {
	communityId, _ := strconv.ParseInt(channel.Address(), 10, 64)

	if payload.CommunityId != communityId {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, errors.New("Wrong community id"))
	}
	verificationString := channel.StringConfigForKey(configServerVerificationString, "")
	// write required response
	_, err := fmt.Fprint(w, verificationString)

	return nil, err
}

// receiveMessage handles new message event
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *moNewMessagePayload) ([]courier.Event, error) {
	// TODO: fix "hardcoded" urn
	urn := urns.URN(fmt.Sprintf("%s:%d", scheme, payload.Object.Message.UserId))
	date := time.Unix(payload.Object.Message.Date, 0).UTC()
	text := payload.Object.Message.Text
	externalId := strconv.FormatInt(payload.Object.Message.Id, 10)
	msg := h.Backend().NewIncomingMsg(channel, urn, text).WithReceivedOn(date).WithExternalID(externalId)

	// save message to our backend
	if err := h.Backend().WriteMsg(ctx, msg); err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}
	// write required response
	_, err := fmt.Fprint(w, responseRequest)

	return []courier.Event{msg}, err
}

func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	return nil, nil
}
