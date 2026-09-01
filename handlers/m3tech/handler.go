package m3tech

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/gocommon/gsm7"
)

var (
	sendURL      = "https://secure.m3techservice.com/GenericServiceRestAPI/api/SendSMS"
	maxMsgLength = 160
)

func init() {
	channels.RegisterHandler(newHandler)
}

type handler struct {
	handlers.BaseHandler
}

func newHandler(rt *runtime.Runtime, r *channels.Routes) channels.Handler {
	h := &handler{handlers.NewBaseHandler(rt, models.ChannelType("M3"), "M3Tech")}

	r.AddReceive(h, http.MethodPost, "receive", channels.ReceiveKindMsg, handlers.NewTelReceiveHandler("from", "text"))
	return h
}

// RespondMsgs writes a success response for the messages
func (h *handler) RespondMsgs(ctx context.Context, w http.ResponseWriter, msgs []*models.MsgIn) error {
	w.Header().Set("Content-Type", "application/json")
	_, err := fmt.Fprintf(w, "SMS Accepted: %s", msgs[0].UUID())
	return err
}

func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	username := msg.Channel().StringConfigForKey(models.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(models.ConfigPassword, "")
	if username == "" || password == "" {
		return channels.ErrChannelConfig
	}

	// figure out if we need to send as unicode (encoding 7)
	text := gsm7.ReplaceSubstitutions(handlers.GetTextAndAttachments(msg))
	encoding := "0"
	if !gsm7.IsValid(text) {
		encoding = "7"
	}

	for _, part := range handlers.SplitMsgByChannel(msg.Channel(), text, maxMsgLength) {
		// build our request
		params := url.Values{
			"AuthKey":     []string{"m3-Tech"},
			"UserId":      []string{username},
			"Password":    []string{password},
			"SMS":         []string{part},
			"SMSType":     []string{encoding},
			"MobileNo":    []string{strings.TrimPrefix(msg.URN().Path(), "+")},
			"MsgId":       []string{string(msg.UUID())},
			"MsgHeader":   []string{strings.TrimPrefix(msg.Channel().Address(), "+")},
			"HandsetPort": []string{"0"},
			"SMSChannel":  []string{"0"},
			"Telco":       []string{"0"},
		}

		msgURL, _ := url.Parse(sendURL)

		msgURL.RawQuery = params.Encode()
		req, err := http.NewRequest(http.MethodGet, msgURL.String(), nil)
		if err != nil {
			return err
		}

		resp, _, err := h.RequestHTTP(req, clog)
		if err := handlers.ErrorFromResponse(resp, err); err != nil {
			return err
		}
	}

	return nil
}
