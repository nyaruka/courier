package telesom

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils/clogs"
	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/urns"
)

var (
	sendURL      = "http://telesom.com/sendsms"
	maxMsgLength = 160
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("TS"), "Telesom")}
}

func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodGet, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
	return nil
}

type moForm struct {
	Mobile  string `name:"mobile" validate:"required"`
	Message string `name:"msg" validate:"required"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	form := &moForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}
	// create our URN
	urn, err := urns.ParsePhone(form.Mobile, channel.Country(), true, false)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// build our msg
	dbMsg := h.Backend().NewIncomingMsg(ctx, channel, urn, form.Message, "", clog)

	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{dbMsg}, w, r, clog)

}

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	privateKey := msg.Channel().StringConfigForKey(courier.ConfigSecret, "")
	if username == "" || password == "" || privateKey == "" {
		return courier.ErrChannelConfig
	}
	tsSendURL := msg.Channel().StringConfigForKey(courier.ConfigSendURL, sendURL)

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
		if err != nil || resp.StatusCode/100 == 5 {
			return courier.ErrConnectionFailed
		} else if resp.StatusCode/100 != 2 {
			return courier.ErrResponseStatus
		}

		if !strings.Contains(string(respBody), "Success") {
			clog.Error(&clogs.Error{Message: fmt.Sprintf("Received invalid response content: %s", string(respBody))})
			return courier.ErrResponseContent
		}
	}

	return nil
}
