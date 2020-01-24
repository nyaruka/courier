package vk

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	// attachment types of incoming messages
	attachmentTypePhoto    = "photo"
	attachmentTypeGraffiti = "graffiti"
	attachmentTypeSticker  = "sticker"
	attachmentTypeAudio    = "audio_message"
	attachmentTypeDoc      = "doc"

	// get user
	URLGetUser   = apiBaseURL + "/users.get.json"
	paramUserIds = "user_ids"

	// base upload media values
	paramServerId = "server"
	paramHash     = "hash"

	// upload media types
	mediaTypeImage = "image"

	// upload photos
	URLGetPhotoUploadServer  = apiBaseURL + "/photos.getMessagesUploadServer.json"
	URLSaveUploadedPhotoInfo = apiBaseURL + "/photos.saveMessagesPhoto.json"
)

var (
	// initialized on send photo attachment
	URLPhotoUploadServer = ""
)

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
		}
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

// response to get user request
type userPayload struct {
	Id        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
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
	AlbumId int64 `json:"album_id"`
	OwnerId int64 `json:"owner_id"`
}

// buildApiBaseParams builds required params to VK API requests
func buildApiBaseParams(channel courier.Channel) url.Values {
	return url.Values{
		paramApiVersion:  []string{apiVersion},
		paramAccessToken: []string{channel.StringConfigForKey(courier.ConfigAuthToken, "")},
	}
}

// retrieveUser retrieves VK user
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

// BuildMsgAttachmentsParam builds an attachments list param of the given msg, also returns the errors that occurred
func buildMsgAttachmentsParam(msg courier.Msg) (string, []error) {
	var msgAttachments []string
	var errs []error

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
			if attachment, err := handleMediaUploadAndGetAttachment(msg.Channel(), mediaTypeImage, mediaExt, mediaURL); err == nil {
				msgAttachments = append(msgAttachments, attachment)
			} else {
				errs = append(errs, err)
			}
		}
	}
	return strings.Join(msgAttachments, ","), errs
}

// handleMediaUploadAndGetAttachment handles media downloading, uploading, saving information and returns the attachment string
func handleMediaUploadAndGetAttachment(channel courier.Channel, mediaType, mediaExt, mediaURL string) (string, error) {
	switch mediaType {
	case mediaTypeImage:
		uploadKey := "photo"

		// initialize server URL to upload photos
		if URLPhotoUploadServer == "" {
			if serverURL, err := getUploadServerURL(channel, URLGetPhotoUploadServer); err == nil {
				URLPhotoUploadServer = serverURL
			}
		}
		download, err := downloadMedia(mediaURL)

		if err != nil {
			return "", nil
		}
		uploadResponse, err := uploadMedia(URLPhotoUploadServer, uploadKey, mediaExt, download)

		if err != nil {
			return "", err
		}
		payload := &photoUploadPayload{}

		if err := json.Unmarshal(uploadResponse, payload); err != nil {
			return "", err
		}
		serverId := strconv.FormatInt(payload.ServerId, 10)
		info, err := saveUploadedMediaInfo(channel, URLSaveUploadedPhotoInfo, serverId, payload.Hash, uploadKey, payload.Photo)

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
func getUploadServerURL(channel courier.Channel, sendURL string) (string, error) {
	req, err := http.NewRequest(http.MethodPost, sendURL, nil)

	if err != nil {
		return "", err
	}
	params := buildApiBaseParams(channel)
	req.URL.RawQuery = params.Encode()
	res, err := utils.MakeHTTPRequest(req)

	if err != nil {
		return "", err
	}
	uploadServer := &uploadServerPayload{}

	if err = json.Unmarshal(res.Body, uploadServer); err != nil {
		return "", nil
	}
	return uploadServer.Server.UploadURL, nil
}

// downloadMedia GET request to given media URL
func downloadMedia(mediaURL string) (io.Reader, error) {
	req, err := http.NewRequest(http.MethodGet, mediaURL, nil)

	if err != nil {
		return nil, err
	}
	if res, err := utils.GetHTTPClient().Do(req); err == nil {
		return res.Body, nil
	} else {
		return nil, err
	}
}

// uploadMedia multiform request that passes file key as uploadKey and file value as media to upload server
func uploadMedia(serverURL, uploadKey, mediaExt string, media io.Reader) ([]byte, error) {
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

	if res, err := utils.MakeHTTPRequest(req); err != nil {
		return nil, err
	} else {
		return res.Body, nil
	}
}

// saveUploadedMediaInfo saves uploaded media info and returns an object containing media/owner id
func saveUploadedMediaInfo(channel courier.Channel, sendURL, serverId, hash, mediaKey, mediaValue string) (*mediaUploadInfoPayload, error) {
	params := buildApiBaseParams(channel)
	params.Set(paramServerId, serverId)
	params.Set(paramHash, hash)
	params.Set(mediaKey, mediaValue)
	req, err := http.NewRequest(http.MethodPost, sendURL, nil)

	if err != nil {
		return nil, err
	}
	req.URL.RawQuery = params.Encode()
	res, err := utils.MakeHTTPRequest(req)

	if err != nil {
		return nil, err
	}
	type responsePayload struct {
		Response []mediaUploadInfoPayload `json:"response"`
	}
	medias := &responsePayload{}

	// try get first object
	if err = json.Unmarshal(res.Body, medias); err != nil || len(medias.Response) == 0 {
		return nil, errors.New("no response")
	} else {
		return &medias.Response[0], nil
	}
}
