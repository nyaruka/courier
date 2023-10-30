package arabiacell

import (
	"context"
	"encoding/xml"
	"fmt"
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

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for AC channel")
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for AC channel")
	}

	serviceID := msg.Channel().StringConfigForKey(configServiceID, "")
	if password == "" {
		return nil, fmt.Errorf("no service_id set for AC channel")
	}

	chargingLevel := msg.Channel().StringConfigForKey(configChargingLevel, "")
	if chargingLevel == "" {
		return nil, fmt.Errorf("no charging_level set for AC channel")
	}

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)
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
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/xml")

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 != 2 {
			return status, nil
		}

		// parse our response as XML
		response := &mtResponse{}
		err = xml.Unmarshal(respBody, response)
		if err != nil {
			clog.Error(courier.ErrorResponseUnparseable("XML"))
			break
		}

		// we always get 204 on success
		if response.Code == "204" {
			status.SetStatus(courier.MsgStatusWired)
			status.SetExternalID(response.MessageID)
		} else {
			status.SetStatus(courier.MsgStatusFailed)
			clog.Error(courier.ErrorResponseStatusCode())
			break
		}
	}

	return status, nil
}
