package macrokiosk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/gsm7"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
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
	err := s.AddHandlerRoute(h, http.MethodPost, "status", h.receiveStatus)
	if err != nil {
		return err
	}
	err = s.AddHandlerRoute(h, http.MethodGet, "status", h.receiveStatus)
	if err != nil {
		return err
	}
	err = s.AddHandlerRoute(h, http.MethodGet, "receive", h.receiveMessage)
	if err != nil {
		return err
	}
	return s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
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
func (h *handler) receiveStatus(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	form := &statusForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	msgStatus, found := statusMapping[form.Status]
	if !found {
		return nil, courier.WriteAndLogRequestIgnored(ctx, w, r, channel, fmt.Sprintf("ignoring unknown status '%s'", form.Status))
	}
	// write our status
	status := h.Backend().NewMsgStatusForExternalID(channel, form.MsgID, msgStatus)
	err = h.Backend().WriteMsgStatus(ctx, status)
	if err == courier.ErrMsgNotFound {
		return nil, courier.WriteAndLogStatusMsgNotFound(ctx, w, r, channel)
	}

	if err != nil {
		return nil, err
	}

	return []courier.Event{status}, courier.WriteStatusSuccess(ctx, w, r, []courier.MsgStatus{status})

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
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	form := &moForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	recipient := form.Longcode
	sender := form.MSISDN
	if form.Shortcode != "" {
		recipient = form.Shortcode
		sender = form.From
	}

	if recipient == "" || sender == "" {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf("missing shortcode, longcode, from or msisdn parameters"))
	}

	if channel.Address() != recipient {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf("invalid to number [%s], expecting [%s]", recipient, channel.Address()))
	}

	loc, err := time.LoadLocation("Asia/Kuala_Lumpur")
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	date, err := time.ParseInLocation("2006-01-0215:04:05", form.Time, loc)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	// create our URN
	urn := urns.NewTelURNForCountry(sender, channel.Country())

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, form.Text).WithExternalID(form.MsgID).WithReceivedOn(date.UTC())

	// and write it
	err = h.Backend().WriteMsg(ctx, msg)
	if err != nil {
		return nil, err
	}
	courier.LogMsgReceived(r, msg)
	return []courier.Event{msg}, h.writeReceiveSuccess(ctx, w, r, "-1") // MacroKiosk expects "-1" back for successful requests
}

func (h *handler) writeReceiveSuccess(ctx context.Context, w http.ResponseWriter, r *http.Request, responseText string) error {
	w.WriteHeader(200)
	_, err := fmt.Fprint(w, responseText)
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

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
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

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	parts := handlers.SplitMsg(text, maxMsgLength)
	for _, part := range parts {
		payload := &mtPayload{}
		payload.From = senderID
		payload.ServID = servID
		payload.To = strings.TrimPrefix(msg.URN().Path(), "+")
		payload.Text = part
		payload.User = username
		payload.Pass = password
		payload.Type = encoding

		requestBody := &bytes.Buffer{}
		json.NewEncoder(requestBody).Encode(payload)

		// build our request
		req, err := http.NewRequest(http.MethodPost, sendURL, requestBody)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		if err != nil {
			return nil, err
		}

		rr, err := utils.MakeHTTPRequest(req)
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
		status.AddLog(log)
		if err != nil {
			return status, nil
		}

		externalID, err := jsonparser.GetString([]byte(rr.Body), "msgid")
		if err != nil {
			return status, fmt.Errorf("unable to parse response body from Macrokiosk")
		}

		status.SetStatus(courier.MsgWired)
		status.SetExternalID(externalID)
	}
	return status, nil
}
