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
	return &handler{handlers.NewBaseHandler(courier.ChannelType("SL"), "Slack", handlers.WithRedactConfigKeys(configBotToken, configUserToken, configValidationToken))}
}

func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeUnknown, handlers.JSONPayload(h, h.receiveEvent))
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

func (h *handler) receiveEvent(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *moPayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	if payload.Type == "url_verification" {
		clog.SetType(courier.ChannelLogTypeWebhookVerify)

		return handleURLVerification(ctx, channel, w, r, payload)
	}

	// if event is not a message or is from the bot ignore it
	if payload.Event.Type == "message" && payload.Event.BotID == "" && payload.Event.ChannelType == "im" {
		clog.SetType(courier.ChannelLogTypeMsgReceive)

		date := time.Unix(int64(payload.EventTime), 0)

		urn, err := urns.NewURNFromParts(urns.SlackScheme, payload.Event.User, "", "")
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}

		attachmentURLs := make([]string, 0)
		for _, file := range payload.Event.Files {
			fileURL, err := h.resolveFile(ctx, channel, file, clog)
			if err != nil {
				courier.LogRequestError(r, channel, err)
			} else {
				attachmentURLs = append(attachmentURLs, fileURL)
			}
		}

		text := payload.Event.Text
		msg := h.Backend().NewIncomingMsg(channel, urn, text, payload.EventID, clog).WithReceivedOn(date)

		for _, attURL := range attachmentURLs {
			msg.WithAttachment(attURL)
		}

		return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
	}
	return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "Ignoring request, no message")
}

func (h *handler) resolveFile(ctx context.Context, channel courier.Channel, file File, clog *courier.ChannelLog) (string, error) {
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

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return "", errors.New("unable to resolve file")
	}

	var fResponse FileResponse
	if err := json.Unmarshal(respBody, &fResponse); err != nil {
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

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	botToken := msg.Channel().StringConfigForKey(configBotToken, "")
	if botToken == "" {
		return nil, fmt.Errorf("missing bot token for SL/slack channel")
	}

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)

	for _, attachment := range msg.Attachments() {
		fileAttachment, err := h.parseAttachmentToFileParams(msg, attachment, clog)
		if err != nil {
			clog.RawError(err)
			return status, nil
		}

		if fileAttachment != nil {
			err = h.sendFilePart(msg, botToken, fileAttachment, clog)
			if err != nil {
				clog.RawError(err)
				return status, nil
			}
		}
	}

	if msg.Text() != "" {
		err := h.sendTextMsgPart(msg, botToken, clog)
		if err != nil {
			clog.RawError(err)
			return status, nil
		}
	}

	status.SetStatus(courier.MsgStatusWired)
	return status, nil
}

func (h *handler) sendTextMsgPart(msg courier.MsgOut, token string, clog *courier.ChannelLog) error {
	sendURL := apiURL + "/chat.postMessage"

	msgPayload := &mtPayload{
		Channel: msg.URN().Path(),
		Text:    msg.Text(),
	}

	body, err := json.Marshal(msgPayload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, sendURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return errors.New("error sending message")
	}

	ok, err := jsonparser.GetBoolean(respBody, "ok")
	if err != nil {
		return err
	}

	if !ok {
		errDescription, err := jsonparser.GetString(respBody, "error")
		if err != nil {
			return err
		}
		return errors.New(errDescription)
	}
	return nil
}

func (h *handler) parseAttachmentToFileParams(msg courier.MsgOut, attachment string, clog *courier.ChannelLog) (*FileParams, error) {
	_, attURL := handlers.SplitAttachment(attachment)

	req, err := http.NewRequest(http.MethodGet, attURL, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "error building file request")
	}

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return nil, errors.New("error fetching attachment")
	}

	filename, err := utils.BasePathForURL(attURL)
	if err != nil {
		return nil, err
	}
	return &FileParams{File: respBody, FileName: filename, Channels: msg.URN().Path()}, nil
}

func (h *handler) sendFilePart(msg courier.MsgOut, token string, fileParams *FileParams, clog *courier.ChannelLog) error {
	uploadURL := apiURL + "/files.upload"

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	mediaPart, err := writer.CreateFormFile("file", fileParams.FileName)
	if err != nil {
		return errors.Wrapf(err, "failed to create file form field")
	}
	io.Copy(mediaPart, bytes.NewReader(fileParams.File))

	filenamePart, err := writer.CreateFormField("filename")
	if err != nil {
		return errors.Wrapf(err, "failed to create filename form field")
	}
	io.Copy(filenamePart, strings.NewReader(fileParams.FileName))

	channelsPart, err := writer.CreateFormField("channels")
	if err != nil {
		return errors.Wrapf(err, "failed to create channels form field")
	}
	io.Copy(channelsPart, strings.NewReader(fileParams.Channels))

	writer.Close()

	req, err := http.NewRequest(http.MethodPost, uploadURL, bytes.NewReader(body.Bytes()))
	if err != nil {
		return errors.Wrapf(err, "error building request to file upload endpoint")
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Add("Content-Type", writer.FormDataContentType())

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return errors.New("error uploading file to slack")
	}

	var fr FileResponse
	if err := json.Unmarshal(respBody, &fr); err != nil {
		return errors.Errorf("couldn't unmarshal file response: %v", err)
	}

	if !fr.OK {
		return errors.Errorf("error uploading file to slack: %s.", fr.Error)
	}

	return nil
}

// DescribeURN handles Slack user details
func (h *handler) DescribeURN(ctx context.Context, channel courier.Channel, urn urns.URN, clog *courier.ChannelLog) (map[string]string, error) {
	resource := "/users.info"
	urlStr := apiURL + resource

	req, _ := http.NewRequest(http.MethodGet, urlStr, nil)

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Authorization", "Bearer "+channel.StringConfigForKey(configBotToken, ""))

	q := req.URL.Query()
	q.Add("user", urn.Path())
	req.URL.RawQuery = q.Encode()

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return nil, errors.New("unable to look up user info")
	}

	var uInfo *UserInfo
	if err := json.Unmarshal(respBody, &uInfo); err != nil {
		return nil, fmt.Errorf("unmarshal user info error:%s", err)
	}

	return map[string]string{"name": uInfo.User.RealName}, nil
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
