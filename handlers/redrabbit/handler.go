package redrabbit

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/gsm7"
)

var (
	sendURL      = "http://http1.javna.com/epicenter/GatewaySendG.asp"
	maxMsgLength = 1600
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("RR"), "Red Rabbit")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	return nil
}

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if username == "" || password == "" {
		return courier.ErrChannelConfig
	}

	text := handlers.GetTextAndAttachments(msg)
	form := url.Values{
		"LoginName":         []string{username},
		"Password":          []string{password},
		"Tracking":          []string{"1"},
		"Mobtyp":            []string{"1"},
		"MessageRecipients": []string{strings.TrimPrefix(msg.URN().Path(), "+")},
		"MessageBody":       []string{text},
		"SenderName":        []string{strings.TrimPrefix(msg.Channel().Address(), "+")},
	}

	if !gsm7.IsValid(text) {
		if len(text) >= 70 {
			form["MsgTyp"] = []string{"10"}
		} else {
			form["MsgTyp"] = []string{"9"}
		}
	} else if len(text) > 160 {
		form["MsgTyp"] = []string{"5"}
	}

	msgURL, _ := url.Parse(sendURL)
	msgURL.RawQuery = form.Encode()
	req, err := http.NewRequest(http.MethodGet, msgURL.String(), nil)
	if err != nil {
		return err
	}

	resp, _, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 == 5 {
		return courier.ErrConnectionFailed
	} else if resp.StatusCode/100 != 2 {
		return courier.ErrResponseStatus
	}

	return nil
}
