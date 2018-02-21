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
	"github.com/nyaruka/gocommon/urns"
	"github.com/pkg/errors"
)

/*
GET /handlers/nexmo/status/uuid/?msisdn=4527631111&to=Tak&network-code=23820&messageId=0C0000002EEBDA56&price=0.01820000&status=delivered&scts=1705021324&err-code=0&message-timestamp=2017-05-02+11%3A24%3A03
GET /handlers/nexmo/receive/uuid/?msisdn=15862151111&to=12812581111&messageId=0B0000004B65F62F&text=Msg&type=text&keyword=Keyword&message-timestamp=2017-05-01+21%3A52%3A49
*/

const configNexmoAPIKey = "nexmo_api_key"
const configNexmoAPISecret = "nexmo_api_secret"
const configNexmoAppID = "nexmo_app_id"
const configNexmoAppPrivateKey = "nexmo_app_private_key"

var maxMsgLength = 1600
var sendURL = "https://rest.nexmo.com/sms/json"
var throttledRE = regexp.MustCompile(`.*Throughput Rate Exceeded - please wait \[ (\d+) \] and retry.*`)

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
	err := s.AddHandlerRoute(h, http.methodGet, "receive", h.ReceiveMessage)
	if err != nil {
		return err
	}
	err = s.AddHandlerRoute(h, http.methodPost, "receive", h.ReceiveMessage)
	if err != nil {
		return err
	}
	return s.AddHandlerRoute(h, http.methodGet, "status", h.StatusMessage)
}

type nexmoDeliveryReport struct {
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

// StatusMessage is our HTTP handler function for status updates
func (h *handler) StatusMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	nexmoDeliveryReport := &nexmoDeliveryReport{}
	handlers.DecodeAndValidateForm(nexmoDeliveryReport, r)

	if nexmoDeliveryReport.MessageID == "" {
		return nil, courier.WriteAndLogRequestIgnored(ctx, w, r, channel, "no messageId parameter, ignored")
	}

	msgStatus, found := statusMappings[nexmoDeliveryReport.Status]
	if !found {
		return nil, courier.WriteAndLogRequestIgnored(ctx, w, r, channel, "ignoring unknown status report")
	}

	status := h.Backend().NewMsgStatusForExternalID(channel, nexmoDeliveryReport.MessageID, msgStatus)

	// write our status
	err := h.Backend().WriteMsgStatus(ctx, status)

	// nexmo can return more than one message id when doing multipart, so ignore status updates which might be for one of those parts
	if err == courier.ErrMsgNotFound {
		return nil, courier.WriteAndLogStatusMsgNotFound(ctx, w, r, channel)
	}

	if err != nil {
		return nil, err
	}

	return []courier.Event{status}, courier.WriteStatusSuccess(ctx, w, r, []courier.MsgStatus{status})
}

type nexmoIncomingMessage struct {
	To        string `name:"to"`
	From      string `name:"msisdn"`
	Text      string `name:"text"`
	MessageID string `name:"messageId"`
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *handler) ReceiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	nexmoIncomingMessage := &nexmoIncomingMessage{}
	handlers.DecodeAndValidateForm(nexmoIncomingMessage, r)

	if nexmoIncomingMessage.To == "" {
		return nil, courier.WriteAndLogRequestIgnored(ctx, w, r, channel, "no to parameter, ignored")
	}

	// create our URN
	urn := urns.NewTelURNForCountry(nexmoIncomingMessage.From, channel.Country())

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, nexmoIncomingMessage.Text)

	// and write it
	err := h.Backend().WriteMsg(ctx, msg)
	if err != nil {
		return nil, err
	}

	return []courier.Event{msg}, courier.WriteMsgSuccess(ctx, w, r, []courier.Msg{msg})
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

		req, err := http.NewRequest(http.MethodPost, sendURL, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		var rr *utils.RequestResponse
		var requestErr error
		for i := 0; i < 3; i++ {
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
