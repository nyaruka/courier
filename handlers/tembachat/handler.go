package tembachat

import (
	"bytes"
	"context"
	"net/http"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
)

var (
	defaultSendURL = "http://chatserver:8070/send"
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("TWC"), "Temba Chat")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMultiReceive, handlers.JSONPayload(h, h.receive))
	return nil
}

type receivePayload struct {
	ChatID string `json:"chat_id" validate:"required"`
	Events []struct {
		Type string `json:"type" validate:"required"`
		Msg  struct {
			Text string `json:"text"`
		} `json:"msg"`
	}
}

// receiveMessage is our HTTP handler function for incoming events
func (h *handler) receive(ctx context.Context, c courier.Channel, w http.ResponseWriter, r *http.Request, payload *receivePayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	urn, err := urns.NewURNFromParts(urns.WebChatScheme, payload.ChatID, "", "")
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
	}

	events := make([]courier.Event, 0, 2)
	data := make([]any, 0, 2)

	for _, event := range payload.Events {
		if event.Type == "msg_in" {
			msg := h.Backend().NewIncomingMsg(c, urn, event.Msg.Text, "", clog)

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
		}
	}

	return events, courier.WriteDataResponse(w, http.StatusOK, "Events Handled", data)
}

type sendPayload struct {
	MsgID  courier.MsgID     `json:"msg_id"`
	ChatID string            `json:"chat_id"`
	Text   string            `json:"text"`
	Origin courier.MsgOrigin `json:"origin"`
	UserID courier.UserID    `json:"user_id,omitempty"`
}

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	sendURL := msg.Channel().StringConfigForKey(courier.ConfigSendURL, defaultSendURL)
	sendURL += "?channel=" + string(msg.Channel().UUID())

	payload := &sendPayload{
		MsgID:  msg.ID(),
		ChatID: msg.URN().Path(),
		Text:   msg.Text(),
		Origin: msg.Origin(),
		UserID: msg.UserID(),
	}
	req, _ := http.NewRequest("POST", sendURL, bytes.NewReader(jsonx.MustMarshal(payload)))

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusWired, clog)

	resp, _, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		status.SetStatus(courier.MsgStatusErrored)
	}

	return status, nil
}
