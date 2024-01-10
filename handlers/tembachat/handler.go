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

const (
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
	Type    string `json:"type"`
	Message struct {
		Identifier string `json:"identifier"`
		Text       string `json:"text"`
	} `json:"message"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, c courier.Channel, w http.ResponseWriter, r *http.Request, payload *receivePayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	if payload.Type == "message" {
		urn, _ := urns.NewWebChatURN(payload.Message.Identifier)
		msg := h.Backend().NewIncomingMsg(c, urn, payload.Message.Text, "", clog)

		return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
	}
	return nil, handlers.WriteAndLogRequestIgnored(ctx, h, c, w, r, "")
}

type sendPayload struct {
	Identifier string `json:"identifier"`
	Text       string `json:"text"`
}

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	sendURL := msg.Channel().StringConfigForKey(courier.ConfigSendURL, defaultSendURL)

	payload := &sendPayload{
		Identifier: msg.URN().Path(),
		Text:       msg.Text(),
	}
	req, _ := http.NewRequest("POST", sendURL, bytes.NewReader(jsonx.MustMarshal(payload)))

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusWired, clog)

	resp, _, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		status.SetStatus(courier.MsgStatusErrored)
	}

	return status, nil
}
