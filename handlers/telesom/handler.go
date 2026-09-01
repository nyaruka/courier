package telesom

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/svclogs"
	"github.com/nyaruka/gocommon/urns"
)

var (
	sendURL      = "http://telesom.com/sendsms"
	maxMsgLength = 160
)

func init() {
	channels.RegisterHandler(newHandler)
}

type handler struct {
	handlers.BaseHandler
}

func newHandler(rt *runtime.Runtime, r *channels.Routes) channels.Handler {
	h := &handler{handlers.NewBaseHandler(rt, models.ChannelType("TS"), "Telesom")}

	r.AddReceive(h, http.MethodGet, "receive", channels.ReceiveKindMsg, handlers.FormPayload(h.receiveMessage))
	r.AddReceive(h, http.MethodPost, "receive", channels.ReceiveKindMsg, handlers.FormPayload(h.receiveMessage))
	return h
}

type moForm struct {
	Mobile  string `name:"mobile" validate:"required"`
	Message string `name:"msg" validate:"required"`
}

// receiveMessage is our receive function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel *models.Channel, r *http.Request, form *moForm, in *channels.Received, clog *models.ChannelLog) error {
	// create our URN
	urn, err := urns.ParsePhone(form.Mobile, channel.Country(), true, false)
	if err != nil {
		return err
	}

	// build our msg
	dbMsg := models.NewIncomingMsg(channel, urn, form.Message, "", clog)

	in.Msg(dbMsg)
	return nil

}

func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	username := msg.Channel().StringConfigForKey(models.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(models.ConfigPassword, "")
	privateKey := msg.Channel().StringConfigForKey(models.ConfigSecret, "")
	if username == "" || password == "" || privateKey == "" {
		return channels.ErrChannelConfig
	}
	tsSendURL := msg.Channel().StringConfigForKey(models.ConfigSendURL, sendURL)

	for _, part := range handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength) {
		from := strings.TrimPrefix(msg.Channel().Address(), "+")
		to := fmt.Sprintf("0%s", urns.ToLocalPhone(msg.URN(), msg.Channel().Country()))

		// build our request
		form := url.Values{
			"to":   []string{to},
			"from": []string{from},
			"msg":  []string{part},
		}

		date := dates.Now().UTC().Format("02/01/2006")

		hasher := md5.New()
		hasher.Write([]byte(username + "|" + password + "|" + to + "|" + part + "|" + from + "|" + date + "|" + privateKey))
		hash := hex.EncodeToString(hasher.Sum(nil))

		form["key"] = []string{strings.ToUpper(hash)}

		req, err := http.NewRequest(http.MethodPost, tsSendURL, strings.NewReader(form.Encode()))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err := handlers.ErrorFromResponse(resp, err); err != nil {
			return err
		}

		if !strings.Contains(string(respBody), "Success") {
			clog.Error(&svclogs.Error{Message: fmt.Sprintf("Received invalid response content: %s", string(respBody))})
			return channels.ErrResponseContent
		}
	}

	return nil
}
