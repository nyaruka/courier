package smscentral

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/gocommon/urns"
)

/*
POST /handlers/smscentral/receive/uuid/
mobile=9779811781111&message=Msg
*/

var sendURL = "http://smail.smscentral.com.np/bp/ApiSms.php"

func init() {
	channels.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() channels.Handler {
	return &handler{handlers.NewBaseHandler(models.ChannelType("SC"), "SMS Central")}
}

// Initialize registers the routes this handler serves
func (h *handler) Initialize(r *channels.Routes) error {
	r.AddReceive(h, http.MethodPost, "receive", channels.ReceiveKindMsg, handlers.FormPayload(h.receiveMessage))
	return nil
}

type moForm struct {
	Message string `name:"message"`
	Mobile  string `name:"mobile" validate:"required" `
}

// receiveMessage is our receive function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel *models.Channel, r *http.Request, form *moForm, in *channels.Received, clog *models.ChannelLog) error {
	// create our URN
	urn, err := urns.ParsePhone(form.Mobile, channel.Country(), true, false)
	if err != nil {
		return err
	}

	// build our msg
	msg := models.NewIncomingMsg(channel, urn, form.Message, "", clog)
	in.Msg(msg)
	return nil
}

func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	username := msg.Channel().StringConfigForKey(models.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(models.ConfigPassword, "")
	if username == "" || password == "" {
		return channels.ErrChannelConfig
	}

	// build our request
	form := url.Values{
		"user":    []string{username},
		"pass":    []string{password},
		"mobile":  []string{strings.TrimPrefix(msg.URN().Path(), "+")},
		"content": []string{handlers.GetTextAndAttachments(msg)},
	}

	req, err := http.NewRequest(http.MethodPost, sendURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, _, err := h.RequestHTTP(req, clog)
	if err := handlers.ErrorFromResponse(resp, err); err != nil {
		return err
	}

	return nil
}
