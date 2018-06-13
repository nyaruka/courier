package burstsms

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
)

var (
	sendURL      = "https://api.transmitsms.com/send-sms.json"
	maxMsgLength = 612
	statusMap    = map[string]courier.MsgStatusValue{
		"delivered":   courier.MsgDelivered,
		"pending":     courier.MsgSent,
		"soft-bounce": courier.MsgErrored,
		"hard-bounce": courier.MsgFailed,
	}
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("BS"), "Burst SMS")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	receiveHandler := handlers.NewTelReceiveHandler(&h.BaseHandler, "mobile", "response")
	s.AddHandlerRoute(h, http.MethodGet, "receive", receiveHandler)

	statusHandler := handlers.NewExternalIDStatusHandler(&h.BaseHandler, statusMap, "message_id", "status")
	s.AddHandlerRoute(h, http.MethodGet, "status", statusHandler)
	return nil
}

// {
//     message_id: 19835,
//     recipients: 3,
//     cost: 1.000
// }
type mtResponse struct {
	MessageID int64 `json:"message_id"`
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(_ context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for BS channel")
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for BS channel")
	}

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	for _, part := range handlers.SplitMsg(handlers.GetTextAndAttachments(msg), maxMsgLength) {
		form := url.Values{
			"to":      []string{strings.TrimLeft(msg.URN().Path(), "+")},
			"from":    []string{msg.Channel().Address()},
			"message": []string{part},
		}

		req, _ := http.NewRequest(http.MethodPost, sendURL, strings.NewReader(form.Encode()))
		req.SetBasicAuth(username, password)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		rr, err := utils.MakeHTTPRequest(req)

		// record our status and log
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
		status.AddLog(log)
		if err != nil {
			return status, nil
		}

		// parse our response as json
		response := &mtResponse{}
		err = json.Unmarshal(rr.Body, response)
		if err != nil {
			log.WithError("Message Send Error", err)
			break
		}

		if response.MessageID != 0 {
			status.SetStatus(courier.MsgWired)
			status.SetExternalID(fmt.Sprintf("%d", response.MessageID))
		} else {
			status.SetStatus(courier.MsgFailed)
			log.WithError("Message Send Error", fmt.Errorf("Received invalid message id: %d", response.MessageID))
			break
		}
	}

	return status, nil
}
