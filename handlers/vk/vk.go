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
	"github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	scheme = "vk"

	// callback API events
	eventTypeServerVerification = "confirmation"
	eventTypeNewMessage         = "message_new"

	configServerVerificationString = "callback_check_string"

	attachmentTypePhoto    = "photo"
	attachmentTypeGraffiti = "graffiti"
	attachmentTypeSticker  = "sticker"
	attachmentTypeVideo    = "video"
	attachmentTypeAudio    = "audio_message"
	attachmentTypeDoc      = "doc"

	// response check values
	responseIncomingMessage    = "ok"
	responseOutgoingMessageKey = "response"

	// base API values
	apiBaseURL       = "https://api.vk.com/method"
	apiVersion       = "5.103"
	paramApiVersion  = "v"
	paramAccessToken = "access_token"

	// get userPayload
	URLGetUser   = apiBaseURL + "/users.get.json"
	paramUserIds = "user_ids"

	// send message
	URLSendMessage = apiBaseURL + "/messages.send.json"
	paramUserId    = "user_id"
	paramMessage   = "message"
	paramRandomId  = "random_id"
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

// base body of callback API event
type moPayload struct {
	Type      string `json:"type"   validate:"required"`
	SecretKey string `json:"secret" validate:"required"`
}

// body to server verification event
type moServerVerificationPayload struct {
	CommunityId int64 `json:"group_id" validate:"required"`
}

// body to new message event
type moNewMessagePayload struct {
	Object struct {
		Message struct {
			Id          int64           `json:"id" validate:"required"`
			Date        int64           `json:"date" validate:"required"`
			UserId      int64           `json:"from_id" validate:"required"`
			Text        string          `json:"text"`
			Attachments json.RawMessage `json:"attachments"`
			Geo         struct {
				Coords struct {
					Lat float64 `json:"latitude"`
					Lng float64 `json:"longitude"`
				} `json:"coordinates"`
			} `json:"geo"`
		} `json:"message" validate:"required"`
	} `json:"object" validate:"required"`
}

type moAttachment struct {
	Type string `json:"type"`
}

type moPhoto struct {
	Photo struct {
		Sizes []struct {
			Type   string `json:"type"`
			Url    string `json:"url"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"sizes"`
	} `json:"photo"`
}

type moGraffiti struct {
	Graffiti struct {
		Id        int64  `json:"id"`
		Url       string `json:"url"`
		AccessKey string `json:"access_key"`
	} `json:"graffiti"`
}

type moSticker struct {
	Sticker struct {
		Images []struct {
			Url    string `json:"url"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		}
	} `json:"sticker"`
}

type moVideo struct {
	Video struct {
		Id        int64  `json:"id"`
		AccessKey string `json:"access_key"`
	} `json:"video"`
}

type moAudio struct {
	Audio struct {
		Link string `json:"link_mp3"`
	} `json:"audio_message"`
}

type moDoc struct {
	Doc struct {
		Id        int64  `json:"id"`
		Url       string `json:"url"`
		AccessKey string `json:"access_key"`
	} `json:"doc"`
}

// body to get user request
type userPayload struct {
	Id        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
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
	// check shared secret key before proceeding
	secret := channel.StringConfigForKey(courier.ConfigSecret, "")

	if payload.SecretKey != secret {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, errors.New("wrong auth token"))
	}
	// check event type and decode body to correspondent struct
	switch payload.Type {
	case eventTypeServerVerification:
		serverVerification := &moServerVerificationPayload{}

		if err := handlers.DecodeAndValidateJSON(serverVerification, r); err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}
		return h.verifyServer(ctx, channel, w, r, serverVerification)

	case eventTypeNewMessage:
		newMessage := &moNewMessagePayload{}

		if err := handlers.DecodeAndValidateJSON(newMessage, r); err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}
		return h.receiveMessage(ctx, channel, w, r, newMessage)

	default:
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring request, no message or server verification event")
	}
}

// verifyServer handles VK's callback verification
func (h *handler) verifyServer(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *moServerVerificationPayload) ([]courier.Event, error) {
	verificationString := channel.StringConfigForKey(configServerVerificationString, "")
	// write required response
	_, err := fmt.Fprint(w, verificationString)

	return nil, err
}

// receiveMessage handles new message event
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *moNewMessagePayload) ([]courier.Event, error) {
	// get userPayload to set contact name
	userId := payload.Object.Message.UserId
	user, err := retrieveUser(channel, userId)
	contactName := ""

	if err != nil {
		logrus.WithField("channel_uuid", channel.UUID()).WithField("userPayload id", userId).Error("error getting VK userPayload", err)
	} else {
		contactName = fmt.Sprintf("%s %s", user.FirstName, user.LastName)
	}
	urn := urns.URN(fmt.Sprintf("%s:%d", scheme, payload.Object.Message.UserId))
	date := time.Unix(payload.Object.Message.Date, 0).UTC()
	text := payload.Object.Message.Text
	externalId := strconv.FormatInt(payload.Object.Message.Id, 10)
	msg := h.Backend().NewIncomingMsg(channel, urn, text).WithReceivedOn(date).WithContactName(contactName).WithExternalID(externalId)

	if attachment := takeFirstAttachmentUrl(*payload); attachment != "" {
		msg = msg.WithAttachment(attachment)
	}
	// save message to our backend
	if err := h.Backend().WriteMsg(ctx, msg); err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}
	// write required response
	_, err = fmt.Fprint(w, responseIncomingMessage)

	return []courier.Event{msg}, err
}

// takeFirstAttachmentUrl tries to take first attachment url, otherwise tries geolocation
func takeFirstAttachmentUrl(payload moNewMessagePayload) string {
	jsonBytes, err := payload.Object.Message.Attachments.MarshalJSON()
	// check if is json array
	if err != nil || !isArrayJson(jsonBytes) {
		return ""
	}
	attachments := &[]moAttachment{}

	if err = json.Unmarshal(jsonBytes, attachments); err != nil || len(*attachments) == 0 {
		// try take geolocation
		lat := payload.Object.Message.Geo.Coords.Lat
		lng := payload.Object.Message.Geo.Coords.Lng

		if lat != 0 && lng != 0 {
			return fmt.Sprintf("geo:%f,%f", lat, lng)
		}
		return ""
	}
	switch (*attachments)[0].Type {
	case attachmentTypePhoto:
		photos := &[]moPhoto{}
		if err = json.Unmarshal(jsonBytes, photos); err == nil {
			photoUrl := ""
			// search by image size "x"
			for _, size := range (*photos)[0].Photo.Sizes {
				photoUrl = size.Url

				if size.Type == "x" {
					break
				}
			}
			return photoUrl
		}

	case attachmentTypeGraffiti:
		graffiti := &[]moGraffiti{}
		if err = json.Unmarshal(jsonBytes, graffiti); err == nil {
			return (*graffiti)[0].Graffiti.Url
		}

	case attachmentTypeSticker:
		stickers := &[]moSticker{}
		// search by image with 128px width/height
		if err = json.Unmarshal(jsonBytes, stickers); err == nil {
			stickerUrl := ""
			for _, image := range (*stickers)[0].Sticker.Images {
				stickerUrl = image.Url
				if image.Width == 128 {
					break
				}
			}
			return stickerUrl
		}

	case attachmentTypeVideo:
		videos := &[]moVideo{}
		if err = json.Unmarshal(jsonBytes, videos); err == nil {
			return (*videos)[0].Video.AccessKey
		}

	case attachmentTypeAudio:
		audios := &[]moAudio{}
		if err = json.Unmarshal(jsonBytes, audios); err == nil {
			return (*audios)[0].Audio.Link
		}

	case attachmentTypeDoc:
		docs := &[]moDoc{}
		if err = json.Unmarshal(jsonBytes, docs); err == nil {
			return (*docs)[0].Doc.Url
		}
	}
	return ""
}

// isArrayJson checks if given bytes is a json array
func isArrayJson(bytes []byte) bool {
	length := len(bytes)

	if length >= 2 && bytes[0] == '[' && bytes[length-1] == ']' {
		return true
	}
	return false
}

// buildApiBaseParams builds required params to VK API requests
func buildApiBaseParams(channel courier.Channel) url.Values {
	return url.Values{
		paramApiVersion:  []string{apiVersion},
		paramAccessToken: []string{channel.StringConfigForKey(courier.ConfigAuthToken, "")},
	}
}

// retrieveUser retrieves VK userPayload
func retrieveUser(channel courier.Channel, userId int64) (*userPayload, error) {
	req, err := http.NewRequest(http.MethodPost, URLGetUser, nil)

	if err != nil {
		return nil, err
	}
	params := buildApiBaseParams(channel)
	params.Set(paramUserIds, strconv.FormatInt(userId, 10))

	req.URL.RawQuery = params.Encode()
	res, err := utils.MakeHTTPRequest(req)

	if err != nil {
		return nil, err
	}
	// parsing response
	type usersResponse struct {
		Users []userPayload `json:"response" validate:"required"`
	}
	payload := &usersResponse{}
	err = json.Unmarshal(res.Body, payload)

	if err != nil {
		return nil, err
	}
	// get first and check if has user
	user := &payload.Users[0]

	if user == nil {
		return nil, errors.New("no user in response")
	}
	return user, nil
}

func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	req, err := http.NewRequest(http.MethodPost, URLSendMessage, nil)

	if err != nil {
		return status, errors.New("Cannot create send message request")
	}
	params := buildApiBaseParams(msg.Channel())
	params.Set(paramUserId, msg.URN().Path())
	params.Set(paramMessage, msg.Text())
	params.Set(paramRandomId, msg.ID().String())

	req.URL.RawQuery = params.Encode()
	res, err := utils.MakeHTTPRequest(req)

	log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), res).WithError("Message Send Error", err)
	status.AddLog(log)

	if err != nil {
		return status, err
	}
	externalMsgId, err := jsonparser.GetInt(res.Body, responseOutgoingMessageKey)

	if err != nil {
		return status, errors.Errorf("no '%s' value in response", responseOutgoingMessageKey)
	}
	status.SetExternalID(strconv.FormatInt(externalMsgId, 10))
	status.SetStatus(courier.MsgSent)

	return status, nil
}
