package redrabbit

import (
	"context"
	"fmt"
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

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if username == "" || password == "" {
		return nil, fmt.Errorf("Missing username or password for RR channel")
	}

	text := handlers.GetTextAndAttachments(msg)
	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)
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
		return nil, err
	}

	resp, _, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return status, nil
	}

	// all went well, set ourselves to wired
	status.SetStatus(courier.MsgStatusWired)

	return status, nil
}
