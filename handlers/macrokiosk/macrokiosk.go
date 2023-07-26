package macrokiosk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/gsm7"

	"github.com/buger/jsonparser"
)

const (
	configMacrokioskSenderID  = "macrokiosk_sender_id"
	configMacrokioskServiceID = "macrokiosk_service_id"
)

var (
	sendURL      = "https://www.etracker.cc/bulksms/send"
	maxMsgLength = 1600
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("MK"), "Macrokiosk")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "status", h.receiveStatus)
	s.AddHandlerRoute(h, http.MethodGet, "status", h.receiveStatus)
	s.AddHandlerRoute(h, http.MethodGet, "receive", h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
	return nil
}

type statusForm struct {
	MsgID  string `name:"msgid" validate:"required"`
	Status string `name:"status" validate:"required"`
}

var statusMapping = map[string]courier.MsgStatusValue{
	"ACCEPTED":    courier.MsgSent,
	"DELIVERED":   courier.MsgDelivered,
	"UNDELIVERED": courier.MsgFailed,
	"PROCESSING":  courier.MsgWired,
}

// receiveStatus is our HTTP handler function for status updates
func (h *handler) receiveStatus(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	form := &statusForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	msgStatus, found := statusMapping[form.Status]
	if !found {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, fmt.Sprintf("ignoring unknown status '%s'", form.Status))
	}
	// write our status
	status := h.Backend().NewMsgStatusForExternalID(channel, form.MsgID, msgStatus, clog)
	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)

}

type moForm struct {
	Longcode  string `name:"longcode"`
	Shortcode string `name:"shortcode"`
	MSISDN    string `name:"msisdn"`
	From      string `name:"from"`
	Text      string `name:"text"`
	MsgID     string `name:"msgId"`
	Time      string `name:"time"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	form := &moForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	recipient := form.Longcode
	sender := form.MSISDN
	if form.Shortcode != "" {
		recipient = form.Shortcode
		sender = form.From
	}

	if recipient == "" || sender == "" {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("missing shortcode, longcode, from or msisdn parameters"))
	}

	if channel.Address() != recipient {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("invalid to number [%s], expecting [%s]", recipient, channel.Address()))
	}

	loc, err := time.LoadLocation("Asia/Kuala_Lumpur")
	if err != nil {
		return nil, err
	}

	date, err := time.ParseInLocation("2006-01-0215:04:05", form.Time, loc)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// create our URN
	urn, err := handlers.StrictTelForCountry(sender, channel.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, form.Text, clog).WithExternalID(form.MsgID).WithReceivedOn(date.UTC())

	// and write it
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r, clog)
}

// WriteMsgSuccessResponse
func (h *handler) WriteMsgSuccessResponse(ctx context.Context, w http.ResponseWriter, msgs []courier.Msg) error {
	w.WriteHeader(200)
	_, err := fmt.Fprint(w, "-1") // MacroKiosk expects "-1" back for successful requests
	return err
}

type mtPayload struct {
	User   string `json:"user"`
	Pass   string `json:"pass"`
	To     string `json:"to"`
	Text   string `json:"text"`
	From   string `json:"from"`
	ServID string `json:"servid"`
	Type   string `json:"type"`
}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.Msg, clog *courier.ChannelLog) (courier.MsgStatus, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	servID := msg.Channel().StringConfigForKey(configMacrokioskServiceID, "")
	senderID := msg.Channel().StringConfigForKey(configMacrokioskSenderID, "")
	if username == "" || password == "" || servID == "" || senderID == "" {
		return nil, fmt.Errorf("missing username, password, serviceID or senderID for MK channel")
	}

	// figure out if we need to send as unicode (encoding 5)
	text := gsm7.ReplaceSubstitutions(handlers.GetTextAndAttachments(msg))
	encoding := "0"
	if !gsm7.IsValid(text) {
		encoding = "5"
	}

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored, clog)
	parts := handlers.SplitMsgByChannel(msg.Channel(), text, maxMsgLength)
	for i, part := range parts {
		payload := &mtPayload{
			From:   senderID,
			ServID: servID,
			To:     strings.TrimPrefix(msg.URN().Path(), "+"),
			Text:   part,
			User:   username,
			Pass:   password,
			Type:   encoding,
		}
		requestBody := &bytes.Buffer{}
		json.NewEncoder(requestBody).Encode(payload)

		// build our request
		req, err := http.NewRequest(http.MethodPost, sendURL, requestBody)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, respBody, err := handlers.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 != 2 {
			return status, nil
		}

		externalID, err := jsonparser.GetString(respBody, "MsgID")
		if err != nil {
			return status, fmt.Errorf("unable to parse response body from Macrokiosk")
		}

		// set the external id if this is our first part
		if i == 0 {
			handlers.CacheAndSetMsgExternalID(h.Backend().RedisPool(), status, externalID, msg)
		}
	}
	status.SetStatus(courier.MsgWired)
	return status, nil
}
