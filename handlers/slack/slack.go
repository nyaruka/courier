package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
		return nil, fmt.Errorf("Wrong validation token for channel: %s", channel.UUID())
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

	date := time.Unix(int64(payload.EventTime), 0)

	urn := urns.URN(fmt.Sprintf("%s:%s", "slack", payload.Event.User))
	// urn, err := urns.NewURNFromParts("slack", payload.Event.User, "", "")
	// if err != nil {
	// 	return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	// }

	if strings.Contains(payload.Event.Type, "message") {
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
		msg := h.Backend().NewIncomingMsg(channel, urn, text).WithReceivedOn(date).WithExternalID(payload.EventID).WithContactName("")

		for _, attURL := range attachmentURLs {
			msg.WithAttachment(attURL)
		}

		return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r)
	}
	return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "Ignoring request, no message")
}

func (h *handler) resolveFile(ctx context.Context, channel courier.Channel, file moFile) (string, error) {
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

	var fResponse fileResponse
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
	sendURL := apiURL + "/chat.postMessage"

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	msgPayload := &mtPayload{
		Channel: msg.URN().Path(),
		Text:    msg.Text(),
	}

	body, err := json.Marshal(msgPayload)
	if err != nil {
		return status, err
	}

	req, err := http.NewRequest(http.MethodPost, sendURL, bytes.NewReader(body))
	if err != nil {
		return status, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", botToken))

	rr, err := utils.MakeHTTPRequest(req)

	log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
	status.AddLog(log)

	ok, err := jsonparser.GetBoolean([]byte(rr.Body), "ok")
	if err != nil || !ok {
		return status, err
	}

	status.SetStatus(courier.MsgWired)

	return status, nil
}

type mtPayload struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
}

type moPayload struct {
	Token    string `json:"token,omitempty"`
	TeamID   string `json:"team_id,omitempty"`
	APIAppID string `json:"api_app_id,omitempty"`
	Event    struct {
		Type        string   `json:"type,omitempty"`
		Channel     string   `json:"channel,omitempty"`
		User        string   `json:"user,omitempty"`
		Text        string   `json:"text,omitempty"`
		Ts          string   `json:"ts,omitempty"`
		EventTs     string   `json:"event_ts,omitempty"`
		ChannelType string   `json:"channel_type,omitempty"`
		Files       []moFile `json:"files"`
	} `json:"event,omitempty"`
	Type           string   `json:"type,omitempty"`
	AuthedUsers    []string `json:"authed_users,omitempty"`
	AuthedTeams    []string `json:"authed_teams,omitempty"`
	Authorizations []struct {
		EnterpriseID string `json:"enterprise_id,omitempty"`
		TeamID       string `json:"team_id,omitempty"`
		UserID       string `json:"user_id,omitempty"`
		IsBot        bool   `json:"is_bot,omitempty"`
	} `json:"authorizations,omitempty"`
	EventContext string `json:"event_context,omitempty"`
	EventID      string `json:"event_id,omitempty"`
	EventTime    int    `json:"event_time,omitempty"`
	Challenge    string `json:"challenge,omitempty"`
}

type item struct {
	Type    string `json:"type,omitempty"`
	Channel string `json:"channel,omitempty"`
	Ts      string `json:"ts,omitempty"`
}

type moFile struct {
	ID                 string `json:"id"`
	Created            int    `json:"created"`
	Timestamp          int    `json:"timestamp"`
	Name               string `json:"name"`
	Title              string `json:"title"`
	Mimetype           string `json:"mimetype"`
	Filetype           string `json:"filetype"`
	PrettyType         string `json:"pretty_type"`
	User               string `json:"user"`
	Editable           bool   `json:"editable"`
	Size               int    `json:"size"`
	Mode               string `json:"mode"`
	IsExternal         bool   `json:"is_external"`
	ExternalType       string `json:"external_type"`
	IsPublic           bool   `json:"is_public"`
	PublicURLShared    bool   `json:"public_url_shared"`
	DisplayAsBot       bool   `json:"display_as_bot"`
	Username           string `json:"username"`
	URLPrivate         string `json:"url_private"`
	URLPrivateDownload string `json:"url_private_download"`
	MediaDisplayType   string `json:"media_display_type"`
	Thumb64            string `json:"thumb_64"`
	Thumb80            string `json:"thumb_80"`
	Thumb360           string `json:"thumb_360"`
	Thumb360W          int    `json:"thumb_360_w"`
	Thumb360H          int    `json:"thumb_360_h"`
	Thumb160           string `json:"thumb_160"`
	OriginalW          int    `json:"original_w"`
	OriginalH          int    `json:"original_h"`
	ThumbTiny          string `json:"thumb_tiny"`
	Permalink          string `json:"permalink"`
	PermalinkPublic    string `json:"permalink_public"`
	HasRichPreview     bool   `json:"has_rich_preview"`
}

type fileResponse struct {
	OK    bool   `json:"ok"`
	File  moFile `json:"file"`
	Error string `json:"error"`
}
