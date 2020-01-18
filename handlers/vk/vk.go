package vk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/buger/jsonparser"
	"github.com/go-errors/errors"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	scheme = "vk"

	eventTypeServerVerification = "confirmation"
	eventTypeNewMessage         = "message_new"

	configServerVerificationString = "callback_check_string"

	responseIncomingMessage = "ok"
	responseKeyOutgoingMessage = "response"

	sendMessageURL   = "https://api.vk.com/method/messages.send.json"
	apiVersion       = "5.103"
	paramApiVersion  = "v"
	paramAccessToken = "access_token"
	paramUserId      = "user_id"
	paramMessage     = "message"
	paramRandomId    = "random_id"
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
	Type      string `json:"type"   validate:"required"`
	SecretKey string `json:"secret" validate:"required"`
}

// request body to VK's server verification
type moServerVerificationPayload struct {
	CommunityId int64  `json:"group_id" validate:"required"`
}

// request body to new message
type moNewMessagePayload struct {
	Object struct {
		Message struct {
			Id     int64  `json:"id"      validate:"required"`
			Date   int64  `json:"date"    validate:"required"`
			UserId int64  `json:"from_id" validate:"required"`
			Text   string `json:"text"    validate:"required"`
		} `json:"message" validate:"required"`
	} `json:"object" validate:"required"`
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
	_, err := fmt.Fprint(w, responseIncomingMessage)

	return []courier.Event{msg}, err
}

func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)

	// build request parameters
	params := url.Values{
		paramApiVersion:  []string{apiVersion},
		paramAccessToken: []string{msg.Channel().StringConfigForKey(courier.ConfigAuthToken, "")},
		paramUserId:      []string{msg.URN().Path()},
		paramMessage:     []string{msg.Text()},
		paramRandomId:    []string{msg.ID().String()},
	}
	req, err := http.NewRequest(http.MethodPost, sendMessageURL, nil)

	if err != nil {
		fmt.Println(err)
		return status, errors.New("Cannot create send message request")
	}
	req.URL.RawQuery = params.Encode()
	res, err := utils.MakeHTTPRequest(req)

	log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), res).WithError("Message Send Error", err)
	status.AddLog(log)

	if err != nil {
		return status, err
	}
	externalMsgId, err := jsonparser.GetInt(res.Body, responseKeyOutgoingMessage)

	if err != nil {
		return status, errors.Errorf("no '%s'", responseKeyOutgoingMessage)
	}
	status.SetExternalID(strconv.FormatInt(externalMsgId, 10))
	status.SetStatus(courier.MsgSent)

	return status, nil
}
