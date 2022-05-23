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
)

var apiURL = "https://slack.com/api"

const (
	configBotToken        = "bot_token"
	configValidationToken = "verification_token"
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
		text := payload.Event.Text

		msg := h.Backend().NewIncomingMsg(channel, urn, text).WithReceivedOn(date).WithExternalID(payload.EventID).WithContactName("")

		return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r)
	}
	return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "Ignoring request, no message")
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
		Type        string `json:"type,omitempty"`
		Channel     string `json:"channel,omitempty"`
		User        string `json:"user,omitempty"`
		Text        string `json:"text,omitempty"`
		Ts          string `json:"ts,omitempty"`
		EventTs     string `json:"event_ts,omitempty"`
		ChannelType string `json:"channel_type,omitempty"`
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
