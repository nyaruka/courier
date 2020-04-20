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
	"github.com/nyaruka/courier/utils"
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
	receiveHandler := handlers.NewTelReceiveHandler(&h.BaseHandler, "M", "B")
	s.AddHandlerRoute(h, http.MethodPost, "receive", receiveHandler)
	return nil
}

// <response>
//   <code>XXX</code>
//   <text>response_text</text>
//   <message_id>message_id_in_case_of_success_sending</message_id>
// </response>
type mtResponse struct {
	Code      string `xml:"code"`
	Text      string `xml:"text"`
	MessageID string `xml:"message_id"`
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(_ context.Context, msg courier.Msg) (courier.MsgStatus, error) {
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

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	for _, part := range handlers.SplitMsg(handlers.GetTextAndAttachments(msg), maxMsgLength) {
		form := url.Values{
			"userName":      []string{username},
			"password":      []string{password},
			"handlerType":   []string{"send_msg"},
			"serviceId":     []string{serviceID},
			"msisdn":        []string{msg.URN().Path()},
			"messageBody":   []string{part},
			"chargingLevel": []string{chargingLevel},
		}

		req, _ := http.NewRequest(http.MethodPost, sendURL, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/xml")
		rr, err := utils.MakeHTTPRequest(req)

		// record our status and log
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
		status.AddLog(log)
		if err != nil {
			return status, nil
		}

		// parse our response as XML
		response := &mtResponse{}
		err = xml.Unmarshal(rr.Body, response)
		if err != nil {
			log.WithError("Message Send Error", err)
			break
		}

		// we always get 204 on success
		if response.Code == "204" {
			status.SetStatus(courier.MsgWired)
			status.SetExternalID(response.MessageID)
		} else {
			status.SetStatus(courier.MsgFailed)
			log.WithError("Message Send Error", fmt.Errorf("Received invalid response code: %s", response.Code))
			break
		}
	}

	return status, nil
}
