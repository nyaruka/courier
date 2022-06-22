package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
	"github.com/pkg/errors"
)

var apiURL = "https://slack.com/api"

const (
	configBotToken        = "bot_token"
	configUserToken       = "user_token"
	configValidationToken = "verification_token"
)

var (
	ErrAlreadyPublic         = "already_public"
	ErrPublicVideoNotAllowed = "public_video_not_allowed"
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("SL"), "Slack")}
}

func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveEvent)
	return nil
}

func handleURLVerification(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *moPayload) ([]courier.Event, error) {
	validationToken := channel.ConfigForKey(configValidationToken, "")
	if validationToken != payload.Token {
		w.WriteHeader(http.StatusForbidden)
		return nil, fmt.Errorf("wrong validation token for channel: %s", channel.UUID())
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(payload.Challenge))
	return nil, nil
}

func (h *handler) receiveEvent(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	payload := &moPayload{}
	err := handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	if payload.Type == "url_verification" {
		return handleURLVerification(ctx, channel, w, r, payload)
	}

	// if event is not a message or is from the bot ignore it
	if payload.Event.Type == "message" && payload.Event.BotID == "" && payload.Event.ChannelType == "im" {

		date := time.Unix(int64(payload.EventTime), 0)

		userInfo, log, err := getUserInfo(payload.Event.User, channel)
		if err != nil {
			h.Backend().WriteChannelLogs(ctx, []*courier.ChannelLog{log})
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}

		urn, err := urns.NewURNFromParts(urns.SlackScheme, payload.Event.User, "", "")
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}

		attachmentURLs := make([]string, 0)
		for _, file := range payload.Event.Files {
			fileURL, err := h.resolveFile(ctx, channel, file)
			if err != nil {
				courier.LogRequestError(r, channel, err)
			} else {
				attachmentURLs = append(attachmentURLs, fileURL)
			}
		}

		text := payload.Event.Text
		msg := h.Backend().NewIncomingMsg(channel, urn, text).WithReceivedOn(date).WithExternalID(payload.EventID).WithContactName(userInfo.User.RealName)

		for _, attURL := range attachmentURLs {
			msg.WithAttachment(attURL)
		}

		return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r)
	}
	return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "Ignoring request, no message")
}

func (h *handler) resolveFile(ctx context.Context, channel courier.Channel, file File) (string, error) {
	userToken := channel.StringConfigForKey(configUserToken, "")

	fileApiURL := apiURL + "/files.sharedPublicURL"

	data := strings.NewReader(fmt.Sprintf(`{"file":"%s"}`, file.ID))
	req, err := http.NewRequest(http.MethodPost, fileApiURL, data)
	if err != nil {
		courier.LogRequestError(req, channel, err)
		return "", err
	}
	req.Header.Add("Content-Type", "application/json; charset=utf-8")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", userToken))

	rr, err := utils.MakeHTTPRequest(req)
	if err != nil {
		log := courier.NewChannelLogFromRR("File Resolving", channel, courier.NilMsgID, rr).WithError("File Resolving Error", err)
		h.Backend().WriteChannelLogs(ctx, []*courier.ChannelLog{log})
		return "", err
	}

	var fResponse FileResponse
	if err := json.Unmarshal([]byte(rr.Body), &fResponse); err != nil {
		return "", errors.Errorf("couldn't unmarshal file response: %v", err)
	}

	currentFile := fResponse.File

	if !fResponse.OK {
		if fResponse.Error != ErrAlreadyPublic {
			if fResponse.Error == ErrPublicVideoNotAllowed {
				return "", errors.Errorf("public sharing of videos is not available for a free instance of Slack. file id: %s. error: %s", file.ID, fResponse.Error)
			}
			return "", errors.Errorf("couldn't resolve file for file id: %s. error: %s", file.ID, fResponse.Error)
		}
		currentFile = file
	}

	pubLnkSplited := strings.Split(currentFile.PermalinkPublic, "-")
	pubSecret := pubLnkSplited[len(pubLnkSplited)-1]
	filePath := currentFile.URLPrivateDownload + "?pub_secret=" + pubSecret

	return filePath, nil
}

func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	botToken := msg.Channel().StringConfigForKey(configBotToken, "")
	if botToken == "" {
		return nil, fmt.Errorf("missing bot token for SL/slack channel")
	}

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)

	hasError := true

	for _, attachment := range msg.Attachments() {
		fileAttachment, log, err := parseAttachmentToFileParams(msg, attachment)
		hasError = err != nil
		status.AddLog(log)

		if fileAttachment != nil {
			log, err = sendFilePart(msg, botToken, fileAttachment)
			hasError = err != nil
			status.AddLog(log)
		}
	}

	if msg.Text() != "" {
		log, err := sendTextMsgPart(msg, botToken)
		hasError = err != nil
		status.AddLog(log)
	}

	if !hasError {
		status.SetStatus(courier.MsgWired)
	}

	return status, nil
}

func sendTextMsgPart(msg courier.Msg, token string) (*courier.ChannelLog, error) {
	sendURL := apiURL + "/chat.postMessage"

	msgPayload := &mtPayload{
		Channel: msg.URN().Path(),
		Text:    msg.Text(),
	}

	body, err := json.Marshal(msgPayload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, sendURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	rr, err := utils.MakeHTTPRequest(req)

	log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)

	ok, err := jsonparser.GetBoolean([]byte(rr.Body), "ok")
	if err != nil {
		return log, err
	}

	if !ok {
		errDescription, err := jsonparser.GetString([]byte(rr.Body), "error")
		if err != nil {
			return log, err
		}
		return log, errors.New(errDescription)
	}
	return log, nil
}

func parseAttachmentToFileParams(msg courier.Msg, attachment string) (*FileParams, *courier.ChannelLog, error) {
	_, attURL := handlers.SplitAttachment(attachment)

	req, err := http.NewRequest(http.MethodGet, attURL, nil)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "error building file request")
	}
	resp, err := utils.MakeHTTPRequest(req)
	log := courier.NewChannelLogFromRR("Fetching attachment", msg.Channel(), msg.ID(), resp).WithError("error fetching media", err)

	filename, err := utils.BasePathForURL(attURL)
	if err != nil {
		return nil, log, err
	}
	return &FileParams{
		File:     resp.Body,
		FileName: filename,
		Channels: msg.URN().Path(),
	}, log, nil
}

func sendFilePart(msg courier.Msg, token string, fileParams *FileParams) (*courier.ChannelLog, error) {
	uploadURL := apiURL + "/files.upload"

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	mediaPart, err := writer.CreateFormFile("file", fileParams.FileName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create file form field")
	}
	io.Copy(mediaPart, bytes.NewReader(fileParams.File))

	filenamePart, err := writer.CreateFormField("filename")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create filename form field")
	}
	io.Copy(filenamePart, strings.NewReader(fileParams.FileName))

	channelsPart, err := writer.CreateFormField("channels")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create channels form field")
	}
	io.Copy(channelsPart, strings.NewReader(fileParams.Channels))

	writer.Close()

	req, err := http.NewRequest(http.MethodPost, uploadURL, bytes.NewReader(body.Bytes()))
	if err != nil {
		return nil, errors.Wrapf(err, "error building request to file upload endpoint")
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Add("Content-Type", writer.FormDataContentType())
	resp, err := utils.MakeHTTPRequest(req)
	if err != nil {
		return nil, errors.Wrapf(err, "error uploading file to slack")
	}

	var fr FileResponse
	if err := json.Unmarshal([]byte(resp.Body), &fr); err != nil {
		return nil, errors.Errorf("couldn't unmarshal file response: %v", err)
	}

	if !fr.OK {
		return nil, errors.Errorf("error uploading file to slack: %s.", fr.Error)
	}

	return courier.NewChannelLogFromRR("uploading file to Slack", msg.Channel(), msg.ID(), resp).WithError("Error uploading file to Slack", err), nil
}

func getUserInfo(userSlackID string, channel courier.Channel) (*UserInfo, *courier.ChannelLog, error) {
	resource := "/users.info"
	urlStr := apiURL + resource

	req, err := http.NewRequest(http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, nil, err
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Authorization", "Bearer "+channel.StringConfigForKey(configBotToken, ""))

	q := req.URL.Query()
	q.Add("user", userSlackID)
	req.URL.RawQuery = q.Encode()

	rr, err := utils.MakeHTTPRequest(req)
	if err != nil {
		log := courier.NewChannelLogFromRR("Get User info", channel, courier.NilMsgID, rr).WithError("Request User Info Error", err)
		return nil, log, err
	}

	var uInfo *UserInfo
	if err := json.Unmarshal(rr.Body, &uInfo); err != nil {
		log := courier.NewChannelLogFromRR("Get User info", channel, courier.NilMsgID, rr).WithError("Unmarshal User Info Error", err)
		return nil, log, err
	}

	return uInfo, nil, nil
}

// mtPayload is a struct that represents the body of a SendMmsg text part.
// https://api.slack.com/methods/chat.postMessage
type mtPayload struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
}

// moPayload is a struct that represents message payload from message type event.
// https://api.slack.com/events/message.im
type moPayload struct {
	Token string `json:"token,omitempty"`
	Event struct {
		Type        string `json:"type,omitempty"`
		Channel     string `json:"channel,omitempty"`
		User        string `json:"user,omitempty"`
		Text        string `json:"text,omitempty"`
		ChannelType string `json:"channel_type,omitempty"`
		Files       []File `json:"files"`
		BotID       string `json:"bot_id,omitempty"`
	} `json:"event,omitempty"`
	Type      string `json:"type,omitempty"`
	EventID   string `json:"event_id,omitempty"`
	EventTime int    `json:"event_time,omitempty"`
	Challenge string `json:"challenge,omitempty"`
}

// File is a struct that represents file item that can be present in Files list in message event, or in FileResponse or in FileParams
type File struct {
	ID                 string `json:"id"`
	Mimetype           string `json:"mimetype"`
	URLPrivateDownload string `json:"url_private_download"`
	PermalinkPublic    string `json:"permalink_public"`
}

// FileResponse is a struct that represents the response from a request in files.sharedPublicURL to make public and shareable a file that is sent in a message.
// https://api.slack.com/methods/files.sharedPublicURL.
type FileResponse struct {
	OK    bool   `json:"ok"`
	File  File   `json:"file"`
	Error string `json:"error"`
}

// FileParams is a struct that represents the request params send to slack api files.upload method to send a file to conversation.
// https://api.slack.com/methods/files.upload.
type FileParams struct {
	File     []byte `json:"file,omitempty"`
	FileName string `json:"filename,omitempty"`
	Channels string `json:"channels,omitempty"`
}

// UserInfo is a struct that represents the response from request in users.info slack api method.
// https://api.slack.com/methods/users.info.
type UserInfo struct {
	Ok   bool `json:"ok"`
	User struct {
		RealName string `json:"real_name"`
	} `json:"user"`
}
