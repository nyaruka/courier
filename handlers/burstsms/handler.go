package burstsms

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/gocommon/httpx"
)

var (
	sendURL      = "https://api.transmitsms.com/send-sms.json"
	maxMsgLength = 612
	statusMap    = map[string]models.MsgStatus{
		"delivered":   models.MsgStatusDelivered,
		"pending":     models.MsgStatusSent,
		"soft-bounce": models.MsgStatusErrored,
		"hard-bounce": models.MsgStatusFailed,
	}
)

func init() {
	channels.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() channels.Handler {
	return &handler{handlers.NewBaseHandler(models.ChannelType("BS"), "Burst SMS")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(r *channels.Routes) error {
	receiveHandler := handlers.NewTelReceiveHandler(h, "mobile", "response")
	r.Add(h, http.MethodGet, "receive", models.ChannelLogTypeMsgReceive, receiveHandler)

	statusHandler := handlers.NewExternalIDStatusHandler(h, statusMap, "message_id", "status")
	r.Add(h, http.MethodGet, "status", models.ChannelLogTypeMsgStatus, statusHandler)
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

func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	username := msg.Channel().StringConfigForKey(models.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(models.ConfigPassword, "")
	if username == "" || password == "" {
		return channels.ErrChannelConfig
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
		if err := handlers.ErrorFromResponse(resp, err); err != nil {
			return err
		}

		response := &mtResponse{}
		err = json.Unmarshal(respBody, response)
		if err != nil {
			return channels.ErrResponseUnparseable
		}

		if response.MessageID != 0 {
			res.AddExternalID(fmt.Sprintf("%d", response.MessageID))
		} else {
			return channels.ErrResponseContent
		}
	}

	return nil
}

func (h *handler) RedactValues(ch *models.Channel) []string {
	return []string{
		httpx.BasicAuth(ch.StringConfigForKey(models.ConfigUsername, ""), ch.StringConfigForKey(models.ConfigPassword, "")),
	}
}
