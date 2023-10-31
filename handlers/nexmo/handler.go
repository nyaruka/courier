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

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/gsm7"

	"github.com/buger/jsonparser"
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

	// https://developer.vonage.com/messaging/sms/guides/troubleshooting-sms#sms-api-error-codes
	sendErrorCodes = map[int]string{
		1:  "Throttled",
		2:  "Missing Parameters",
		3:  "Invalid Parameters",
		4:  "Invalid Credentials",
		5:  "Internal Error",
		6:  "Invalid Message",
		7:  "Number Barred",
		8:  "Partner Account Barred",
		9:  "Partner Quota Violation",
		10: "Too Many Existing Binds",
		11: "Account Not Enabled For HTTP",
		12: "Message Too Long",
		14: "Invalid Signature",
		15: "Invalid Sender Address",
		22: "Invalid Network Code",
		23: "Invalid Callback URL",
		29: "Non-Whitelisted Destination",
		32: "Signature And API Secret Disallowed",
		33: "Number De-activated",
	}

	// https://developer.vonage.com/messaging/sms/guides/delivery-receipts#dlr-error-codes
	dlrErrorCodes = map[int]string{
		1:  "Unknown",
		2:  "Absent Subscriber - Temporary",
		3:  "Absent Subscriber - Permanent",
		4:  "Call Barred by User",
		5:  "Portability Error",
		6:  "Anti-Spam Rejection",
		7:  "Handset Busy",
		8:  "Network Error",
		9:  "Illegal Number",
		10: "Illegal Message",
		11: "Unroutable",
		12: "Destination Unreachable",
		13: "Subscriber Age Restriction",
		14: "Number Blocked by Carrier",
		15: "Prepaid Insufficient Funds",
		16: "Gateway Quota Exceeded",
		50: "Entity Filter",
		51: "Header Filter",
		52: "Content Filter",
		53: "Consent Filter",
		54: "Regulation Error",
		99: "General Error",
	}
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("NX"), "Nexmo", handlers.WithRedactConfigKeys(configNexmoAPISecret, configNexmoAppPrivateKey))}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodGet, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodPost, "status", courier.ChannelLogTypeMsgStatus, h.receiveStatus)
	s.AddHandlerRoute(h, http.MethodGet, "status", courier.ChannelLogTypeMsgStatus, h.receiveStatus)
	return nil
}

// https://developer.vonage.com/messaging/sms/guides/delivery-receipts
type statusForm struct {
	To        string `name:"to"`
	MessageID string `name:"messageId"`
	Status    string `name:"status"`
	ErrCode   int    `name:"err-code"`
}

var statusMappings = map[string]courier.MsgStatus{
	"failed":    courier.MsgStatusFailed,
	"expired":   courier.MsgStatusFailed,
	"rejected":  courier.MsgStatusFailed,
	"buffered":  courier.MsgStatusSent,
	"accepted":  courier.MsgStatusSent,
	"unknown":   courier.MsgStatusWired,
	"delivered": courier.MsgStatusDelivered,
}

// receiveStatus is our HTTP handler function for status updates
func (h *handler) receiveStatus(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	form := &statusForm{}
	handlers.DecodeAndValidateForm(form, r)

	if form.MessageID == "" {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "no messageId parameter, ignored")
	}

	msgStatus, found := statusMappings[form.Status]
	if !found {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring unknown status report")
	}

	if form.ErrCode != 0 {
		clog.Error(courier.ErrorExternal("dlr:"+strconv.Itoa(form.ErrCode), dlrErrorCodes[form.ErrCode]))
	}

	status := h.Backend().NewStatusUpdateByExternalID(channel, form.MessageID, msgStatus, clog)

	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

type moForm struct {
	To        string `name:"to"`
	From      string `name:"msisdn"`
	Text      string `name:"text"`
	MessageID string `name:"messageId"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
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

	// create and write the message
	msg := h.Backend().NewIncomingMsg(channel, urn, form.Text, form.MessageID, clog)
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
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

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)
	parts := handlers.SplitMsgByChannel(msg.Channel(), text, maxMsgLength)
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

		var resp *http.Response
		var respBody []byte
		var requestErr error

		for i := 0; i < 3; i++ {
			req, err := http.NewRequest(http.MethodPost, sendURL, strings.NewReader(form.Encode()))
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			resp, respBody, requestErr = h.RequestHTTP(req, clog)
			matched := throttledRE.FindAllStringSubmatch(string(respBody), -1)
			if len(matched) > 0 && len(matched[0]) > 0 {
				sleepTime, _ := strconv.Atoi(matched[0][1])
				time.Sleep(time.Duration(sleepTime) * time.Millisecond)
			} else {
				break
			}
		}

		if requestErr != nil || resp.StatusCode/100 != 2 {
			return status, nil
		}

		nexmoStatus, err := jsonparser.GetString(respBody, "messages", "[0]", "status")
		errCode, _ := strconv.Atoi(nexmoStatus)
		if err != nil || nexmoStatus != "0" {
			// https://developer.vonage.com/messaging/sms/guides/troubleshooting-sms
			clog.Error(courier.ErrorExternal("send:"+nexmoStatus, sendErrorCodes[errCode]))
			return status, nil
		}

		externalID, err := jsonparser.GetString(respBody, "messages", "[0]", "message-id")
		if err == nil {
			status.SetExternalID(externalID)
		}

	}
	status.SetStatus(courier.MsgStatusWired)
	return status, nil
}
