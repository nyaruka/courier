package nexmo

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nyaruka/courier/gsm7"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/pkg/errors"
)

const (
	configNexmoAPIKey        = "nexmo_api_key"
	configNexmoAPISecret     = "nexmo_api_secret"
	configNexmoAppID         = "nexmo_app_id"
	configNexmoAppPrivateKey = "nexmo_app_private_key"
)

var (
	maxMsgLength = 1600
	sendURL      = "https://rest.nexmo.com/sms/json"
	throttledRE  = regexp.MustCompile(`.*Throughput Rate Exceeded - please wait \[ (\d+) \] and retry.*`)
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("NX"), "Nexmo")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodGet, "receive", h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodPost, "status", h.receiveStatus)
	s.AddHandlerRoute(h, http.MethodGet, "status", h.receiveStatus)
	return nil
}

type statusForm struct {
	To        string `name:"to"`
	MessageID string `name:"messageID"`
	Status    string `name:"status"`
}

var statusMappings = map[string]courier.MsgStatusValue{
	"failed":    courier.MsgFailed,
	"expired":   courier.MsgFailed,
	"rejected":  courier.MsgFailed,
	"buffered":  courier.MsgSent,
	"accepted":  courier.MsgSent,
	"unknown":   courier.MsgWired,
	"delivered": courier.MsgDelivered,
}

// receiveStatus is our HTTP handler function for status updates
func (h *handler) receiveStatus(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	form := &statusForm{}
	handlers.DecodeAndValidateForm(form, r)

	if form.MessageID == "" {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "no messageId parameter, ignored")
	}

	msgStatus, found := statusMappings[form.Status]
	if !found {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring unknown status report")
	}

	status := h.Backend().NewMsgStatusForExternalID(channel, form.MessageID, msgStatus)

	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

type moForm struct {
	To        string `name:"to"`
	From      string `name:"msisdn"`
	Text      string `name:"text"`
	MessageID string `name:"messageId"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	form := &moForm{}
	handlers.DecodeAndValidateForm(form, r)

	if form.To == "" {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "no to parameter, ignored")
	}

	// create our URN
	urn, err := handlers.StrictTelForCountry(form.From, channel.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, form.Text)
	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r)
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	nexmoAPIKey := msg.Channel().StringConfigForKey(configNexmoAPIKey, "")
	if nexmoAPIKey == "" {
		return nil, fmt.Errorf("no nexmo API key set for NX channel")
	}
	nexmoAPISecret := msg.Channel().StringConfigForKey(configNexmoAPISecret, "")
	if nexmoAPISecret == "" {
		return nil, fmt.Errorf("no nexmo API secret set for NX channel")
	}

	// build our callback URL
	callbackDomain := msg.Channel().CallbackDomain(h.Server().Config().Domain)
	callbackURL := fmt.Sprintf("https://%s/c/nx/%s/status", callbackDomain, msg.Channel().UUID())

	text := handlers.GetTextAndAttachments(msg)

	textType := "text"
	if !gsm7.IsValid(text) {
		textType = "unicode"
	}

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	parts := handlers.SplitMsg(text, maxMsgLength)
	for _, part := range parts {
		form := url.Values{
			"api_key":           []string{nexmoAPIKey},
			"api_secret":        []string{nexmoAPISecret},
			"from":              []string{strings.TrimPrefix(msg.Channel().Address(), "+")},
			"to":                []string{strings.TrimPrefix(msg.URN().Path(), "+")},
			"text":              []string{part},
			"status-report-req": []string{"1"},
			"callback":          []string{callbackURL},
			"type":              []string{textType},
		}

		var rr *utils.RequestResponse
		var requestErr error
		for i := 0; i < 3; i++ {
			req, _ := http.NewRequest(http.MethodPost, sendURL, strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			rr, requestErr = utils.MakeHTTPRequest(req)
			matched := throttledRE.FindAllStringSubmatch(string([]byte(rr.Body)), -1)
			if len(matched) > 0 && len(matched[0]) > 0 {
				sleepTime, _ := strconv.Atoi(matched[0][1])
				time.Sleep(time.Duration(sleepTime) * time.Millisecond)
			} else {
				break
			}
		}

		// record our status and log
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr)
		status.AddLog(log)
		if requestErr != nil {
			log.WithError("Message Send Error", requestErr)
			return status, nil
		}

		nexmoStatus, err := jsonparser.GetString([]byte(rr.Body), "messages", "[0]", "status")
		if err != nil || nexmoStatus != "0" {
			log.WithError("Message Send Error", errors.Errorf("failed to send message, received error status [%s]", nexmoStatus))
			return status, nil
		}

		externalID, err := jsonparser.GetString([]byte(rr.Body), "messages", "[0]", "message-id")
		if err == nil {
			status.SetExternalID(externalID)
		}

	}
	status.SetStatus(courier.MsgWired)
	return status, nil
}
