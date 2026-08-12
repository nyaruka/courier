package arabiacell

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/url"
	"strings"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
)

const (
	configServiceID     = "service_id"
	configChargingLevel = "charging_level"
)

var (
	sendURL      = "https://acsdp.arabiacell.net"
	maxMsgLength = 1530
)

func init() {
	channels.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() channels.Handler {
	return &handler{handlers.NewBaseHandler(models.ChannelType("AC"), "Arabia Cell")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(r *channels.Routes) error {
	receiveHandler := handlers.NewTelReceiveHandler(h, "M", "B")
	r.Add(h, http.MethodPost, "receive", models.ChannelLogTypeMsgReceive, receiveHandler)
	return nil
}

// <response>
//
//	<code>XXX</code>
//	<text>response_text</text>
//	<message_id>message_id_in_case_of_success_sending</message_id>
//
// </response>
type mtResponse struct {
	Code      string `xml:"code"`
	Text      string `xml:"text"`
	MessageID string `xml:"message_id"`
}

func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	username := msg.Channel().StringConfigForKey(models.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(models.ConfigPassword, "")
	serviceID := msg.Channel().StringConfigForKey(configServiceID, "")
	chargingLevel := msg.Channel().StringConfigForKey(configChargingLevel, "")

	if username == "" || password == "" || serviceID == "" || chargingLevel == "" {
		return channels.ErrChannelConfig
	}

	for _, part := range handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength) {
		form := url.Values{
			"userName":      []string{username},
			"password":      []string{password},
			"handlerType":   []string{"send_msg"},
			"serviceId":     []string{serviceID},
			"msisdn":        []string{msg.URN().Path()},
			"messageBody":   []string{part},
			"chargingLevel": []string{chargingLevel},
		}

		req, err := http.NewRequest(http.MethodPost, sendURL, strings.NewReader(form.Encode()))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/xml")

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err := handlers.ErrorFromResponse(resp, err); err != nil {
			return err
		}

		// parse our response as XML
		response := &mtResponse{}
		err = xml.Unmarshal(respBody, response)
		if err != nil {
			return channels.ErrResponseUnparseable
		}

		// we always get 204 on success
		if response.Code == "204" {
			res.AddExternalID(response.MessageID)
		} else {
			return channels.ErrResponseContent
		}
	}

	return nil
}
