package jasmin

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/gsm7"
	"github.com/nyaruka/gocommon/urns"
)

var idRegex = regexp.MustCompile(`Success \"(.*)\"`)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(models.ChannelType("JS"), "Jasmin")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodPost, "status", courier.ChannelLogTypeMsgStatus, h.receiveStatus)
	return nil
}

type statusForm struct {
	ID            string `name:"id"     validate:"required"`
	MessageStatus string `name:"message_status"`
	Delivered     int    `name:"dlvrd"`
	Err           int    `name:"err"`
}

var statusMapping = map[string]models.MsgStatus{
	"ACCEPTD": models.MsgStatusSent,
	"UNKNOWN": models.MsgStatusWired,
	"UNDELIV": models.MsgStatusFailed,
	"REJECTD": models.MsgStatusFailed,
	"EXPIRED": models.MsgStatusFailed,
	"DELETED": models.MsgStatusFailed,
	"DELIVRD": models.MsgStatusDelivered,
}

// receiveStatus is our HTTP handler function for status updates
func (h *handler) receiveStatus(ctx context.Context, c courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	form := &statusForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
	}

	// should have either delivered or err
	var reqStatus models.MsgStatus
	msgStatus, found := statusMapping[form.MessageStatus]
	if found {
		reqStatus = msgStatus
	} else if form.Delivered == 1 {
		reqStatus = models.MsgStatusDelivered
	} else if form.Err == 1 {
		reqStatus = models.MsgStatusFailed
	} else {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, fmt.Errorf("must have a known message_status or either dlvrd or err set to 1"))
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
	urn, err := urns.ParsePhone(form.From, c.Country(), true, false)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
	}

	// Decode from GSM7 if required
	text := string(form.Content)
	if form.Coding == "0" {
		text = gsm7.Decode([]byte(form.Content))
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(ctx, c, urn, text, form.ID, clog).WithReceivedOn(time.Now().UTC())

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

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	username := msg.Channel().StringConfigForKey(models.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(models.ConfigPassword, "")
	sendURL := msg.Channel().StringConfigForKey(models.ConfigSendURL, "")
	if username == "" || password == "" || sendURL == "" {
		return courier.ErrChannelConfig
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
	}

	// if we are smart, first try to convert to GSM7 chars
	replaced := gsm7.ReplaceSubstitutions(handlers.GetTextAndAttachments(msg))
	if gsm7.IsValid(replaced) {
		form["content"] = []string{replaced}
	} else {
		form["coding"] = []string{"8"}

		hexText := make([]byte, hex.EncodedLen(len(handlers.GetTextAndAttachments(msg))))
		hex.Encode(hexText, []byte(handlers.GetTextAndAttachments(msg)))
		form["hex-content"] = []string{string(hexText)}
	}

	fullURL, _ := url.Parse(sendURL)
	fullURL.RawQuery = form.Encode()

	req, err := http.NewRequest(http.MethodGet, fullURL.String(), nil)
	if err != nil {
		return err
	}

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 == 5 {
		return courier.ErrConnectionFailed
	} else if resp.StatusCode/100 != 2 {
		return courier.ErrResponseStatus
	}

	// try to read our external id out
	matches := idRegex.FindSubmatch(respBody)
	if len(matches) == 2 {
		res.AddExternalID(string(matches[1]))
	}

	return nil
}
