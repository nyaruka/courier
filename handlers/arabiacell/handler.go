package arabiacell

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/url"
	"strings"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
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
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("AC"), "Arabia Cell")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	receiveHandler := handlers.NewTelReceiveHandler(h, "M", "B")
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, receiveHandler)
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

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	serviceID := msg.Channel().StringConfigForKey(configServiceID, "")
	chargingLevel := msg.Channel().StringConfigForKey(configChargingLevel, "")

	if username == "" || password == "" || serviceID == "" || chargingLevel == "" {
		return courier.ErrChannelConfig
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
		if err != nil || resp.StatusCode/100 == 5 {
			return courier.ErrConnectionFailed
		} else if resp.StatusCode/100 != 2 {
			return courier.ErrResponseStatus
		}

		// parse our response as XML
		response := &mtResponse{}
		err = xml.Unmarshal(respBody, response)
		if err != nil {
			return courier.ErrResponseUnparseable
		}

		// we always get 204 on success
		if response.Code == "204" {
			res.AddExternalID(response.MessageID)
		} else {
			return courier.ErrResponseContent
		}
	}

	return nil
}
