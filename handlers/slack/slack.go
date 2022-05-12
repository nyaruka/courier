package slack

import (
	"context"
	"fmt"
	"net/http"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
)

var apiURL = "https://slack.com/api/"

const (
	configBotToken = "bot_token"
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
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
	return nil
}

func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	return nil, nil
}

func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	botToken := msg.Channel().StringConfigForKey(configBotToken, "")
	if botToken == "" {
		return nil, fmt.Errorf("missing bot token for SL channel")
	}
	return nil, nil
}

type moPayload struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
}
