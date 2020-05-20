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
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/courier/utils/dates"
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
	s.AddHandlerRoute(h, http.MethodGet, "receive", h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
	return nil
}

type moForm struct {
	Mobile  string `name:"mobile" validate:"required"`
	Message string `name:"msg" validate:"required"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	form := &moForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}
	// create our URN
	urn, err := handlers.StrictTelForCountry(form.Mobile, channel.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// build our msg
	dbMsg := h.Backend().NewIncomingMsg(channel, urn, form.Message)

	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{dbMsg}, w, r)

}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for TS channel")
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for TS channel")
	}

	privateKey := msg.Channel().StringConfigForKey(courier.ConfigSecret, "")
	if privateKey == "" {
		return nil, fmt.Errorf("no private key set for TS channel")
	}

	tsSendURL := msg.Channel().StringConfigForKey(courier.ConfigSendURL, sendURL)

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)

	for _, part := range handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength) {
		from := strings.TrimPrefix(msg.Channel().Address(), "+")
		to := fmt.Sprintf("0%s", strings.TrimPrefix(msg.URN().Localize(msg.Channel().Country()).Path(), "0"))

		// build our request
		form := url.Values{
			"username": []string{username},
			"password": []string{password},
			"to":       []string{to},
			"from":     []string{from},
			"msg":      []string{part},
		}

		date := dates.Now().UTC().Format("02/01/2006")

		hasher := md5.New()
		hasher.Write([]byte(username + "|" + password + "|" + to + "|" + part + "|" + from + "|" + date + "|" + privateKey))
		hash := hex.EncodeToString(hasher.Sum(nil))

		form["key"] = []string{strings.ToUpper(hash)}
		encodedForm := form.Encode()
		tsSendURL = fmt.Sprintf("%s?%s", tsSendURL, encodedForm)

		req, _ := http.NewRequest(http.MethodGet, tsSendURL, nil)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr, err := utils.MakeInsecureHTTPRequest(req)

		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
		status.AddLog(log)
		if err != nil {
			return status, nil
		}

		if strings.Contains(string(rr.Body), "Success") {
			status.SetStatus(courier.MsgWired)
		} else {
			log.WithError("Message Send Error", fmt.Errorf("Received invalid response content: %s", string(rr.Body)))
		}
	}
	return status, nil

}
