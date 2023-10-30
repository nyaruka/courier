package jasmin

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/gsm7"
)

var idRegex = regexp.MustCompile(`Success \"(.*)\"`)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("JS"), "Jasmin")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodPost, "status", courier.ChannelLogTypeMsgStatus, h.receiveStatus)
	return nil
}

type statusForm struct {
	ID        string `name:"id"     validate:"required"`
	Delivered int    `name:"dlvrd"`
	Err       int    `name:"err"`
}

// receiveStatus is our HTTP handler function for status updates
func (h *handler) receiveStatus(ctx context.Context, c courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	form := &statusForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
	}

	// should have either delivered or err
	reqStatus := courier.NilMsgStatus
	if form.Delivered == 1 {
		reqStatus = courier.MsgStatusDelivered
	} else if form.Err == 1 {
		reqStatus = courier.MsgStatusFailed
	} else {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, fmt.Errorf("must have either dlvrd or err set to 1"))
	}

	status := h.Backend().NewStatusUpdateByExternalID(c, form.ID, reqStatus, clog)
	return handlers.WriteMsgStatusAndResponse(ctx, h, c, status, w, r)
}

type moForm struct {
	Content string `name:"content"`
	Coding  string `name:"coding"   validate:"required"`
	From    string `name:"from"     validate:"required"`
	To      string `name:"to"       validate:"required"`
	ID      string `name:"id"       validate:"required"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, c courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	// get our params
	form := &moForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
	}

	// create our URN
	urn, err := handlers.StrictTelForCountry(form.From, c.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
	}

	// Decode from GSM7 if required
	text := string(form.Content)
	if form.Coding == "0" {
		text = gsm7.Decode([]byte(form.Content))
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(c, urn, text, form.ID, clog).WithReceivedOn(time.Now().UTC())

	// and finally queue our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
}

func (h *handler) WriteMsgSuccessResponse(ctx context.Context, w http.ResponseWriter, msgs []courier.MsgIn) error {
	return writeJasminACK(w)
}

func (h *handler) WriteStatusSuccessResponse(ctx context.Context, w http.ResponseWriter, statuses []courier.StatusUpdate) error {
	return writeJasminACK(w)
}

func (h *handler) WriteRequestIgnored(ctx context.Context, w http.ResponseWriter, details string) error {
	return writeJasminACK(w)
}

func writeJasminACK(w http.ResponseWriter) error {
	w.WriteHeader(200)
	_, err := fmt.Fprint(w, "ACK/Jasmin")
	return err
}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for JS channel")
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for JS channel")
	}

	sendURL := msg.Channel().StringConfigForKey(courier.ConfigSendURL, "")
	if sendURL == "" {
		return nil, fmt.Errorf("no send url set for JS channel")
	}

	callbackDomain := msg.Channel().CallbackDomain(h.Server().Config().Domain)
	dlrURL := fmt.Sprintf("https://%s/c/js/%s/status", callbackDomain, msg.Channel().UUID())

	// build our request
	form := url.Values{
		"username":   []string{username},
		"password":   []string{password},
		"from":       []string{strings.TrimPrefix(msg.Channel().Address(), "+")},
		"to":         []string{strings.TrimPrefix(msg.URN().Path(), "+")},
		"dlr":        []string{"yes"},
		"dlr-url":    []string{dlrURL},
		"dlr-level":  []string{"2"},
		"dlr-method": []string{http.MethodPost},
		"coding":     []string{"0"},
		"content":    []string{string(gsm7.Encode(gsm7.ReplaceSubstitutions(handlers.GetTextAndAttachments(msg))))},
	}

	fullURL, _ := url.Parse(sendURL)
	fullURL.RawQuery = form.Encode()

	req, err := http.NewRequest(http.MethodGet, fullURL.String(), nil)
	if err != nil {
		return nil, err
	}

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return status, nil
	}

	status.SetStatus(courier.MsgStatusWired)

	// try to read our external id out
	matches := idRegex.FindSubmatch(respBody)
	if len(matches) == 2 {
		status.SetExternalID(string(matches[1]))
	}

	return status, nil
}
