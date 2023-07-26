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
	"github.com/nyaruka/gocommon/httpx"
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
	receiveHandler := handlers.NewTelReceiveHandler(h, "mobile", "response")
	s.AddHandlerRoute(h, http.MethodGet, "receive", courier.ChannelLogTypeMsgReceive, receiveHandler)

	statusHandler := handlers.NewExternalIDStatusHandler(h, statusMap, "message_id", "status")
	s.AddHandlerRoute(h, http.MethodGet, "status", courier.ChannelLogTypeMsgStatus, statusHandler)
	return nil
}

//	{
//	    message_id: 19835,
//	    recipients: 3,
//	    cost: 1.000
//	}
type mtResponse struct {
	MessageID int64 `json:"message_id"`
}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.Msg, clog *courier.ChannelLog) (courier.MsgStatus, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for BS channel")
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for BS channel")
	}

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored, clog)
	for _, part := range handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength) {
		form := url.Values{
			"to":      []string{strings.TrimLeft(msg.URN().Path(), "+")},
			"from":    []string{msg.Channel().Address()},
			"message": []string{part},
		}

		req, err := http.NewRequest(http.MethodPost, sendURL, strings.NewReader(form.Encode()))
		if err != nil {
			return nil, err
		}
		req.SetBasicAuth(username, password)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")

		resp, respBody, err := handlers.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 != 2 {
			return status, nil
		}

		// parse our response as json
		response := &mtResponse{}
		err = json.Unmarshal(respBody, response)
		if err != nil {
			clog.Error(courier.ErrorResponseUnparseable("XML"))
			break
		}

		if response.MessageID != 0 {
			status.SetStatus(courier.MsgWired)
			status.SetExternalID(fmt.Sprintf("%d", response.MessageID))
		} else {
			status.SetStatus(courier.MsgFailed)
			clog.Error(courier.ErrorResponseValueMissing("message_id"))
			break
		}
	}

	return status, nil
}

func (h *handler) RedactValues(ch courier.Channel) []string {
	return []string{
		httpx.BasicAuth(ch.StringConfigForKey(courier.ConfigUsername, ""), ch.StringConfigForKey(courier.ConfigPassword, "")),
	}
}
