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
	statusMap    = map[string]courier.MsgStatus{
		"delivered":   courier.MsgStatusDelivered,
		"pending":     courier.MsgStatusSent,
		"soft-bounce": courier.MsgStatusErrored,
		"hard-bounce": courier.MsgStatusFailed,
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

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if username == "" || password == "" {
		return courier.ErrChannelConfig
	}

	for _, part := range handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength) {
		form := url.Values{
			"to":      []string{strings.TrimLeft(msg.URN().Path(), "+")},
			"from":    []string{msg.Channel().Address()},
			"message": []string{part},
		}

		req, err := http.NewRequest(http.MethodPost, sendURL, strings.NewReader(form.Encode()))
		if err != nil {
			return err
		}
		req.SetBasicAuth(username, password)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 == 5 {
			return courier.ErrConnectionFailed
		} else if resp.StatusCode/100 != 2 {
			return courier.ErrResponseStatus
		}

		response := &mtResponse{}
		err = json.Unmarshal(respBody, response)
		if err != nil {
			return courier.ErrResponseUnparseable
		}

		if response.MessageID != 0 {
			res.AddExternalID(fmt.Sprintf("%d", response.MessageID))
		} else {
			return courier.ErrResponseContent
		}
	}

	return nil
}

func (h *handler) RedactValues(ch courier.Channel) []string {
	return []string{
		httpx.BasicAuth(ch.StringConfigForKey(courier.ConfigUsername, ""), ch.StringConfigForKey(courier.ConfigPassword, "")),
	}
}
