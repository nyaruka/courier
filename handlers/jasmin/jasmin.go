package kannel

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/nyaruka/courier/gsm7"

	"strings"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
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
	return &handler{handlers.NewBaseHandler(courier.ChannelType("JS"), "Jasmin")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	err := s.AddHandlerRoute(h, http.MethodPost, "receive", h.ReceiveMessage)
	if err != nil {
		return err
	}
	return s.AddHandlerRoute(h, http.MethodPost, "status", h.ReceiveStatus)
}

type statusRequest struct {
	ID        string `name:"id"     validate:"required"`
	Delivered int    `name:"dlvrd"`
	Err       int    `name:"err"`
}

// ReceiveStatus is our HTTP handler function for status updates
func (h *handler) ReceiveStatus(ctx context.Context, c courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	req := &statusRequest{}
	err := handlers.DecodeAndValidateForm(req, r)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, c, err)
	}

	// should have either delivered or err
	reqStatus := courier.NilMsgStatus
	if req.Delivered == 1 {
		reqStatus = courier.MsgDelivered
	} else if req.Err == 1 {
		reqStatus = courier.MsgFailed
	} else {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, c, fmt.Errorf("must have either dlvrd or err set to 1"))
	}

	status := h.Backend().NewMsgStatusForExternalID(c, req.ID, reqStatus)
	err = h.Backend().WriteMsgStatus(ctx, status)
	if err == courier.ErrMsgNotFound {
		return nil, writeJasminACK(w)
	}
	if err != nil {
		return nil, err
	}

	return []courier.Event{status}, writeJasminACK(w)
}

type moRequest struct {
	Content string `name:"content"`
	Coding  string `name:"coding"   validate:"required"`
	From    string `name:"from"     validate:"required"`
	To      string `name:"to"       validate:"required"`
	ID      string `name:"id"       validate:"required"`
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *handler) ReceiveMessage(ctx context.Context, c courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	// get our params
	req := &moRequest{}
	err := handlers.DecodeAndValidateForm(req, r)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, c, err)
	}

	// create our URN
	urn := urns.NewTelURNForCountry(req.From, c.Country())

	// Decode from GSM7 if required
	text := string(req.Content)
	if req.Coding == "0" {
		text = gsm7.Decode([]byte(req.Content))
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(c, urn, text).WithExternalID(req.ID).WithReceivedOn(time.Now().UTC())

	// and finally queue our message
	err = h.Backend().WriteMsg(ctx, msg)
	if err != nil {
		return nil, err
	}

	return []courier.Event{msg}, writeJasminACK(w)
}

func writeJasminACK(w http.ResponseWriter) error {
	w.WriteHeader(200)
	_, err := fmt.Fprint(w, "ACK/Jasmin")
	return err
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
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
		"dlr-method": []string{"POST"},
		"coding":     []string{"0"},
		"content":    []string{string(gsm7.Encode(gsm7.ReplaceSubstitutions(courier.GetTextAndAttachments(msg))))},
	}

	fullURL, _ := url.Parse(sendURL)
	fullURL.RawQuery = form.Encode()

	req, _ := http.NewRequest(http.MethodGet, fullURL.String(), nil)
	rr, err := utils.MakeHTTPRequest(req)

	// record our status and log
	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	status.AddLog(courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err))
	if err == nil {
		status.SetStatus(courier.MsgWired)
	}

	// try to read our external id out
	matches := idRegex.FindStringSubmatch(string(rr.Body))
	if len(matches) == 2 {
		status.SetExternalID(matches[1])
	}

	return status, nil
}
