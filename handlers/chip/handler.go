package chip

import (
	"bytes"
	"context"
	"errors"
	"net/http"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
)

var (
	defaultSendURL = "http://textit.com/wc/send"

	statuses = map[string]courier.MsgStatus{
		"sent":      courier.MsgStatusSent,
		"delivered": courier.MsgStatusDelivered,
		"failed":    courier.MsgStatusFailed,
	}
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("CHP"), "Chip Web Chat", handlers.WithRedactConfigKeys(courier.ConfigSecret))}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMultiReceive, handlers.JSONPayload(h, h.receive))
	return nil
}

type receivePayload struct {
	ChatID string `json:"chat_id" validate:"required"`
	Secret string `json:"secret"  validate:"required"`
	Events []struct {
		Type string `json:"type"  validate:"required"`
		Msg  struct {
			Text string `json:"text"`
		} `json:"msg"`
		Status struct {
			MsgID  courier.MsgID `json:"msg_id"`
			Status string        `json:"status"`
		} `json:"status"`
	}
}

// receiveMessage is our HTTP handler function for incoming events
func (h *handler) receive(ctx context.Context, c courier.Channel, w http.ResponseWriter, r *http.Request, payload *receivePayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	secret := c.StringConfigForKey(courier.ConfigSecret, "")
	if !utils.SecretEqual(payload.Secret, secret) {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, errors.New("secret incorrect"))
	}

	urn, err := urns.New(urns.WebChat, payload.ChatID)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, errors.New("invalid chat id"))
	}

	events := make([]courier.Event, 0, 2)
	data := make([]any, 0, 2)

	for _, event := range payload.Events {
		if event.Type == "msg_in" {
			msg := h.Backend().NewIncomingMsg(ctx, c, urn, event.Msg.Text, "", clog)

			if err = h.Backend().WriteMsg(ctx, msg, clog); err != nil {
				return nil, err
			}

			events = append(events, msg)
			data = append(data, courier.NewMsgReceiveData(msg))

		} else if event.Type == "chat_started" {
			evt := h.Backend().NewChannelEvent(c, courier.EventTypeNewConversation, urn, clog)

			if err := h.Backend().WriteChannelEvent(ctx, evt, clog); err != nil {
				return nil, err
			}

			events = append(events, evt)
			data = append(data, courier.NewEventReceiveData(evt))
		} else if event.Type == "msg_status" {
			status := statuses[event.Status.Status]
			if status != "" {
				evt := h.Backend().NewStatusUpdate(c, event.Status.MsgID, status, clog)

				if err := h.Backend().WriteStatusUpdate(ctx, evt); err != nil {
					return nil, err
				}

				events = append(events, evt)
				data = append(data, courier.NewStatusData(evt))
			}
		}
	}

	return events, courier.WriteDataResponse(w, http.StatusOK, "Events Handled", data)
}

type sendMsg struct {
	ID           courier.MsgID     `json:"id"`
	Text         string            `json:"text"`
	Attachments  []string          `json:"attachments,omitempty"`
	QuickReplies []string          `json:"quick_replies,omitempty"`
	Origin       courier.MsgOrigin `json:"origin"`
	UserID       courier.UserID    `json:"user_id,omitempty"`
}

type sendPayload struct {
	ChatID string  `json:"chat_id"`
	Secret string  `json:"secret"`
	Msg    sendMsg `json:"msg"`
}

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	secret := msg.Channel().StringConfigForKey(courier.ConfigSecret, "")
	sendURL := msg.Channel().StringConfigForKey(courier.ConfigSendURL, defaultSendURL)
	if secret == "" || sendURL == "" {
		return courier.ErrChannelConfig
	}

	payload := &sendPayload{
		ChatID: msg.URN().Path(),
		Secret: secret,
		Msg: sendMsg{
			ID:           msg.ID(),
			Text:         msg.Text(),
			Attachments:  msg.Attachments(),
			QuickReplies: handlers.TextOnlyQuickReplies(msg.QuickReplies()),
			Origin:       msg.Origin(),
			UserID:       msg.UserID(),
		},
	}
	req, _ := http.NewRequest("POST", sendURL+"/"+string(msg.Channel().UUID())+"/", bytes.NewReader(jsonx.MustMarshal(payload)))
	req.Header.Set("Content-Type", "application/json")

	resp, _, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 == 5 {
		return courier.ErrConnectionFailed
	} else if resp.StatusCode/100 == 4 {
		return courier.ErrResponseUnexpected
	}

	return nil
}
