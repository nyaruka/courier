package vk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/pkg/errors"
)

var (
	// callback API events
	eventTypeServerVerification = "confirmation"
	eventTypeNewMessage         = "message_new"

	configServerVerificationString = "callback_verification_string"

	// attachment types of incoming messages
	attachmentTypePhoto    = "photo"
	attachmentTypeGraffiti = "graffiti"
	attachmentTypeSticker  = "sticker"
	attachmentTypeAudio    = "audio_message"
	attachmentTypeDoc      = "doc"

	// base API values
	apiBaseURL       = "https://api.vk.com/method"
	apiVersion       = "5.103"
	paramApiVersion  = "v"
	paramAccessToken = "access_token"

	// response check values
	responseIncomingMessage    = "ok"
	responseOutgoingMessageKey = "response"

	// get user
	actionGetUser = "/users.get.json"
	paramUserIds  = "user_ids"

	// send message
	actionSendMessage = "/messages.send.json"
	paramUserId       = "user_id"
	paramMessage      = "message"
	paramAttachments  = "attachment"
	paramRandomId     = "random_id"
	paramKeyboard     = "keyboard"

	// base upload media values
	paramServerId = "server"
	paramHash     = "hash"

	// upload media types
	mediaTypeImage = "image"

	// upload photos
	actionGetPhotoUploadServer  = "/photos.getMessagesUploadServer.json"
	actionSaveUploadedPhotoInfo = "/photos.saveMessagesPhoto.json"
)

var (
	// initialized on send photo attachment
	URLPhotoUploadServer = ""
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
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeUnknown, handlers.JSONPayload(h, h.receiveEvent))
	return nil
}

// base body to callback API event
type moPayload struct {
	Type      string `json:"type"   validate:"required"`
	SecretKey string `json:"secret" validate:"required"`
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
			Payload string `json:"payload"`
		} `json:"message" validate:"required"`
	} `json:"object" validate:"required"`
}

// response to get user request
type userPayload struct {
	Id        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// Attachment types

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
		Url string `json:"url"`
	} `json:"graffiti"`
}

type moSticker struct {
	Sticker struct {
		Images []struct {
			Url    string `json:"url"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"images"`
	} `json:"sticker"`
}

type moAudio struct {
	Audio struct {
		Link string `json:"link_mp3"`
	} `json:"audio_message"`
}

type moDoc struct {
	Doc struct {
		Url string `json:"url"`
	} `json:"doc"`
}

// response to get photo upload server
type uploadServerPayload struct {
	Server struct {
		UploadURL string `json:"upload_url"`
	} `json:"response"`
}

// response to photo upload
type photoUploadPayload struct {
	ServerId int64  `json:"server"`
	Photo    string `json:"photo"`
	Hash     string `json:"hash"`
}

// response to media upload info
type mediaUploadInfoPayload struct {
	MediaId int64 `json:"id"`
	OwnerId int64 `json:"owner_id"`
}

// receiveEvent handles request event type
func (h *handler) receiveEvent(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *moPayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	// check shared secret key before proceeding
	secret := channel.StringConfigForKey(courier.ConfigSecret, "")

	if payload.SecretKey != secret {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, errors.New("wrong secret key"))
	}
	// check event type and decode body to correspondent struct
	switch payload.Type {
	case eventTypeServerVerification:
		clog.SetType(courier.ChannelLogTypeWebhookVerify)

		return h.verifyServer(channel, w)

	case eventTypeNewMessage:
		clog.SetType(courier.ChannelLogTypeMsgReceive)

		newMessage := &moNewMessagePayload{}

		if err := handlers.DecodeAndValidateJSON(newMessage, r); err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}
		return h.receiveMessage(ctx, channel, w, r, newMessage, clog)

	default:
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring request, no message or server verification event")
	}
}

// verifyServer handles VK's callback verification
func (h *handler) verifyServer(channel courier.Channel, w http.ResponseWriter) ([]courier.Event, error) {
	verificationString := channel.StringConfigForKey(configServerVerificationString, "")
	// write required response
	_, err := fmt.Fprint(w, verificationString)

	return nil, err
}

// receiveMessage handles new message event
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *moNewMessagePayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	userId := payload.Object.Message.UserId
	urn, err := urns.NewURNFromParts(urns.VKScheme, strconv.FormatInt(userId, 10), "", "")

	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}
	date := time.Unix(payload.Object.Message.Date, 0).UTC()
	text := payload.Object.Message.Text
	externalId := strconv.FormatInt(payload.Object.Message.Id, 10)
	msg := h.Backend().NewIncomingMsg(channel, urn, text, externalId, clog).WithReceivedOn(date)

	if attachment := takeFirstAttachmentUrl(*payload); attachment != "" {
		msg.WithAttachment(attachment)
	}
	// check for empty content
	if msg.Text() == "" && len(msg.Attachments()) == 0 {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, errors.New("no text or attachment"))
	}
	// save message to our backend
	if err := h.Backend().WriteMsg(ctx, msg, clog); err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}
	// write required response
	_, err = fmt.Fprint(w, responseIncomingMessage)

	return []courier.Event{msg}, err
}

// DescribeURN handles VK contact details
func (h *handler) DescribeURN(ctx context.Context, channel courier.Channel, urn urns.URN, clog *courier.ChannelLog) (map[string]string, error) {
	req, err := http.NewRequest(http.MethodPost, apiBaseURL+actionGetUser, nil)
	if err != nil {
		return nil, err
	}

	params := buildApiBaseParams(channel)
	_, urnPath, _, _ := urn.ToParts()
	params.Set(paramUserIds, urnPath)

	req.URL.RawQuery = params.Encode()

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return nil, errors.New("unable to look up user info")
	}

	// parsing response
	type responsePayload struct {
		Users []userPayload `json:"response" validate:"required"`
	}
	payload := &responsePayload{}
	err = json.Unmarshal(respBody, payload)

	if err != nil {
		return nil, err
	}
	if len(payload.Users) == 0 {
		return nil, errors.New("no user in response")
	}
	// get first and check if has user
	user := payload.Users[0]
	return map[string]string{"name": fmt.Sprintf("%s %s", user.FirstName, user.LastName)}, nil
}

// buildApiBaseParams builds required params to VK API requests
func buildApiBaseParams(channel courier.Channel) url.Values {
	return url.Values{
		paramApiVersion:  []string{apiVersion},
		paramAccessToken: []string{channel.StringConfigForKey(courier.ConfigAuthToken, "")},
	}
}

// takeFirstAttachmentUrl tries to take first attachment url, otherwise tries geolocation
func takeFirstAttachmentUrl(payload moNewMessagePayload) string {
	jsonBytes, err := payload.Object.Message.Attachments.MarshalJSON()

	if err != nil {
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

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)

	params := buildApiBaseParams(msg.Channel())
	params.Set(paramUserId, msg.URN().Path())
	params.Set(paramRandomId, msg.ID().String())

	text, attachments := h.buildTextAndAttachmentParams(msg, clog)
	params.Set(paramMessage, text)
	params.Set(paramAttachments, attachments)

	if len(msg.QuickReplies()) != 0 {
		qrs := msg.QuickReplies()
		keyboard := NewKeyboardFromReplies(qrs)

		params.Set(paramKeyboard, string(jsonx.MustMarshal(keyboard)))
	}

	req, err := http.NewRequest(http.MethodPost, apiBaseURL+actionSendMessage, nil)
	if err != nil {
		return status, errors.New("Cannot create send message request")
	}

	req.URL.RawQuery = params.Encode()

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return status, nil
	}

	externalMsgId, err := jsonparser.GetInt(respBody, responseOutgoingMessageKey)

	if err != nil {
		return status, errors.Errorf("no '%s' value in response", responseOutgoingMessageKey)
	}
	status.SetExternalID(strconv.FormatInt(externalMsgId, 10))
	status.SetStatus(courier.MsgStatusSent)

	return status, nil
}

// builds msg text with attachment links (if needed) and attachments list param, also returns the errors that occurred
func (h *handler) buildTextAndAttachmentParams(msg courier.MsgOut, clog *courier.ChannelLog) (string, string) {
	var msgAttachments []string

	textBuf := bytes.Buffer{}
	textBuf.WriteString(msg.Text())

	for _, attachment := range msg.Attachments() {
		// handle attachment type
		mediaPrefix, mediaURL := handlers.SplitAttachment(attachment)
		mediaPrefixParts := strings.Split(mediaPrefix, "/")

		if len(mediaPrefixParts) < 2 {
			continue
		}
		mediaType, mediaExt := mediaPrefixParts[0], mediaPrefixParts[1]

		switch mediaType {
		case mediaTypeImage:
			if attachment, err := h.handleMediaUploadAndGetAttachment(msg.Channel(), mediaTypeImage, mediaExt, mediaURL, clog); err == nil {
				msgAttachments = append(msgAttachments, attachment)
			} else {
				clog.RawError(err)
			}

		default:
			textBuf.WriteString("\n\n")
			textBuf.WriteString(mediaURL)
		}
	}
	return textBuf.String(), strings.Join(msgAttachments, ",")
}

// handles media downloading, uploading, saving information and returns the attachment string
func (h *handler) handleMediaUploadAndGetAttachment(channel courier.Channel, mediaType, mediaExt, mediaURL string, clog *courier.ChannelLog) (string, error) {
	switch mediaType {
	case mediaTypeImage:
		uploadKey := "photo"

		// initialize server URL to upload photos
		if URLPhotoUploadServer == "" {
			if serverURL, err := h.getUploadServerURL(channel, apiBaseURL+actionGetPhotoUploadServer, clog); err == nil {
				URLPhotoUploadServer = serverURL
			}
		}
		download, err := h.downloadMedia(mediaURL)

		if err != nil {
			return "", err
		}
		uploadResponse, err := h.uploadMedia(URLPhotoUploadServer, uploadKey, mediaExt, download, clog)

		if err != nil {
			return "", err
		}
		payload := &photoUploadPayload{}

		if err := json.Unmarshal(uploadResponse, payload); err != nil {
			return "", err
		}
		serverId := strconv.FormatInt(payload.ServerId, 10)
		info, err := h.saveUploadedMediaInfo(channel, apiBaseURL+actionSaveUploadedPhotoInfo, serverId, payload.Hash, uploadKey, payload.Photo, clog)

		if err != nil {
			return "", err
		} else {
			// return in the appropriate format
			return fmt.Sprintf("%s%d_%d", uploadKey, info.OwnerId, info.MediaId), nil
		}

	default:
		return "", errors.New("invalid media type")
	}
}

// getUploadServerURL gets VK's media upload server
func (h *handler) getUploadServerURL(channel courier.Channel, sendURL string, clog *courier.ChannelLog) (string, error) {
	req, err := http.NewRequest(http.MethodPost, sendURL, nil)

	if err != nil {
		return "", err
	}
	params := buildApiBaseParams(channel)
	req.URL.RawQuery = params.Encode()

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return "", errors.New("unable to get upload server URL")
	}

	uploadServer := &uploadServerPayload{}

	if err = json.Unmarshal(respBody, uploadServer); err != nil {
		return "", nil
	}
	return uploadServer.Server.UploadURL, nil
}

// downloadMedia GET request to given media URL
func (h *handler) downloadMedia(mediaURL string) (io.Reader, error) {
	req, err := http.NewRequest(http.MethodGet, mediaURL, nil)

	if err != nil {
		return nil, err
	}
	if res, err := h.Backend().HttpClient(true).Do(req); err == nil {
		return res.Body, nil
	} else {
		return nil, err
	}
}

// uploadMedia multiform request that passes file key as uploadKey and file value as media to upload server
func (h *handler) uploadMedia(serverURL, uploadKey, mediaExt string, media io.Reader, clog *courier.ChannelLog) ([]byte, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	fileName := fmt.Sprintf("%s.%s", uploadKey, mediaExt)

	part, err := writer.CreateFormFile(uploadKey, fileName)
	if err != nil {
		return nil, err
	}

	_, err = io.Copy(part, media)
	if err != nil {
		return nil, err
	}

	err = writer.Close()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, serverURL, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return nil, errors.New("unable to upload media")
	}
	return respBody, nil
}

// saveUploadedMediaInfo saves uploaded media info and returns an object containing media/owner id
func (h *handler) saveUploadedMediaInfo(channel courier.Channel, sendURL, serverId, hash, mediaKey, mediaValue string, clog *courier.ChannelLog) (*mediaUploadInfoPayload, error) {
	params := buildApiBaseParams(channel)
	params.Set(paramServerId, serverId)
	params.Set(paramHash, hash)
	params.Set(mediaKey, mediaValue)

	req, err := http.NewRequest(http.MethodPost, sendURL, nil)
	if err != nil {
		return nil, err
	}

	req.URL.RawQuery = params.Encode()

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return nil, errors.New("unable to save uploaded media info")
	}

	// parsing response
	type responsePayload struct {
		Response []mediaUploadInfoPayload `json:"response"`
	}
	medias := &responsePayload{}

	// try get first object
	if err = json.Unmarshal(respBody, medias); err != nil || len(medias.Response) == 0 {
		return nil, errors.New("no response")
	} else {
		return &medias.Response[0], nil
	}
}
