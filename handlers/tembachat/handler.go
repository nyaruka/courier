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
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, handlers.JSONPayload(h, h.receiveMessage))
	return nil
}

type receivePayload struct {
	Type string `json:"type" validate:"required"`
	Msg  struct {
		Identifier string `json:"identifier"`
		Text       string `json:"text"`
	} `json:"msg"`
	Chat struct {
		Identifier string `json:"identifier"`
	} `json:"chat"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, c courier.Channel, w http.ResponseWriter, r *http.Request, payload *receivePayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	if payload.Type == "msg_in" {
		urn, err := urns.NewWebChatURN(payload.Msg.Identifier)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
		}

		msg := h.Backend().NewIncomingMsg(c, urn, payload.Msg.Text, "", clog)
		return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
	} else if payload.Type == "chat_started" {
		urn, err := urns.NewWebChatURN(payload.Chat.Identifier)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
		}

		evt := h.Backend().NewChannelEvent(c, courier.EventTypeNewConversation, urn, clog)
		err = h.Backend().WriteChannelEvent(ctx, evt, clog)
		if err != nil {
			return nil, err
		}
		return []courier.Event{evt}, courier.WriteChannelEventSuccess(w, evt)
	}
	return nil, handlers.WriteAndLogRequestIgnored(ctx, h, c, w, r, "")
}

type sendPayload struct {
	Identifier string                 `json:"identifier"`
	Text       string                 `json:"text"`
	Origin     string                 `json:"origin"`
	User       *courier.UserReference `json:"user,omitempty"`
}

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	sendURL := msg.Channel().StringConfigForKey(courier.ConfigSendURL, defaultSendURL)

	payload := &sendPayload{
		Identifier: msg.URN().Path(),
		Text:       msg.Text(),
		Origin:     string(msg.Origin()),
		User:       msg.User(),
	}
	req, _ := http.NewRequest("POST", sendURL, bytes.NewReader(jsonx.MustMarshal(payload)))

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusWired, clog)

	resp, _, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		status.SetStatus(courier.MsgStatusErrored)
	}

	return status, nil
}
