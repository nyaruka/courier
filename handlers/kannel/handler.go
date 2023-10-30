package kannel

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/gsm7"
)

const (
	configEncoding   = "encoding"
	configVerifySSL  = "verify_ssl"
	configDLRMask    = "dlr_mask"
	configIgnoreSent = "ignore_sent"

	encodingDefault = "D"
	encodingUnicode = "U"
	encodingSmart   = "S"

	// see: https://kannel.org/download/1.5.0/userguide-1.5.0/userguide.html#DELIVERY-REPORTS
	// registers us for submit to smsc failure, submit to smsc success, delivery to handset success, delivery to handset failure
	defaultDLRMask = "27"
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("KN"), "Kannel")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodGet, "status", courier.ChannelLogTypeMsgReceive, h.receiveStatus)
	return nil
}

type moForm struct {
	ID      string `validate:"required" name:"id"`
	TS      int64  `validate:"required" name:"ts"`
	Message string `name:"message"`
	Sender  string `validate:"required" name:"sender"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	// get our params
	form := &moForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// create our date from the timestamp
	date := time.Unix(form.TS, 0).UTC()

	// create our URN
	urn, err := handlers.StrictTelForCountry(form.Sender, channel.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, form.Message, form.ID, clog).WithReceivedOn(date)

	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
}

var statusMapping = map[int]courier.MsgStatus{
	1:  courier.MsgStatusDelivered,
	2:  courier.MsgStatusErrored,
	4:  courier.MsgStatusSent,
	8:  courier.MsgStatusSent,
	16: courier.MsgStatusErrored,
}

type statusForm struct {
	ID     courier.MsgID `validate:"required" name:"id"`
	Status int           `validate:"required" name:"status"`
}

// receiveStatus is our HTTP handler function for status updates
func (h *handler) receiveStatus(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	// get our params
	form := &statusForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	msgStatus, found := statusMapping[form.Status]
	if !found {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unknown status '%d', must be one of 1,2,4,8,16", form.Status))
	}

	// if we are ignoring delivery reports and this isn't failed then move on
	if channel.BoolConfigForKey(configIgnoreSent, false) && msgStatus == courier.MsgStatusSent {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring sent report (message aready wired)")
	}

	// write our status
	status := h.Backend().NewStatusUpdate(channel, form.ID, msgStatus, clog)
	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for KN channel")
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for KN channel")
	}

	sendURL := msg.Channel().StringConfigForKey(courier.ConfigSendURL, "")
	if sendURL == "" {
		return nil, fmt.Errorf("no send url set for KN channel")
	}

	dlrMask := msg.Channel().StringConfigForKey(configDLRMask, defaultDLRMask)

	callbackDomain := msg.Channel().CallbackDomain(h.Server().Config().Domain)
	dlrURL := fmt.Sprintf("https://%s/c/kn/%s/status?id=%s&status=%%d", callbackDomain, msg.Channel().UUID(), msg.ID().String())

	// build our request
	form := url.Values{
		"username": []string{username},
		"password": []string{password},
		"from":     []string{msg.Channel().Address()},
		"text":     []string{handlers.GetTextAndAttachments(msg)},
		"to":       []string{msg.URN().Path()},
		"dlr-url":  []string{dlrURL},
		"dlr-mask": []string{dlrMask},
	}

	if msg.HighPriority() {
		form["priority"] = []string{"1"}
	}

	useNationalStr := msg.Channel().ConfigForKey(courier.ConfigUseNational, false)
	useNational, _ := useNationalStr.(bool)

	// if we are meant to use national formatting (no country code) pull that out
	if useNational {
		nationalTo := msg.URN().Localize(msg.Channel().Country())
		form["to"] = []string{nationalTo.Path()}
	}

	// figure out what encoding to tell kannel to send as
	encoding := msg.Channel().StringConfigForKey(configEncoding, encodingSmart)

	// if we are smart, first try to convert to GSM7 chars
	if encoding == encodingSmart {
		replaced := gsm7.ReplaceSubstitutions(handlers.GetTextAndAttachments(msg))
		if gsm7.IsValid(replaced) {
			form["text"] = []string{replaced}
		} else {
			encoding = encodingUnicode
		}
	}

	// if we are UTF8, set our coding appropriately
	if encoding == encodingUnicode {
		form["coding"] = []string{"2"}
		form["charset"] = []string{"utf8"}
	}

	// our send URL may have form parameters in it already, append our own afterwards
	encodedForm := form.Encode()
	if strings.Contains(sendURL, "?") {
		sendURL = fmt.Sprintf("%s&%s", sendURL, encodedForm)
	} else {
		sendURL = fmt.Sprintf("%s?%s", sendURL, encodedForm)
	}

	// ignore SSL warnings if they ask
	verifySSLStr := msg.Channel().ConfigForKey(configVerifySSL, true)
	verifySSL, _ := verifySSLStr.(bool)

	req, err := http.NewRequest(http.MethodGet, sendURL, nil)
	if err != nil {
		return nil, err
	}

	var resp *http.Response
	if verifySSL {
		resp, _, err = h.RequestHTTP(req, clog)
	} else {
		resp, _, err = h.RequestHTTPInsecure(req, clog)
	}

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)
	if err == nil && resp.StatusCode/100 == 2 {
		status.SetStatus(courier.MsgStatusWired)
	}

	// kannel will respond with a 403 for non-routable numbers, fail permanently in these cases
	if resp != nil && resp.StatusCode == 403 {
		status.SetStatus(courier.MsgStatusFailed)
	}

	return status, nil
}
