package macrokiosk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/gocommon/gsm7"
	"github.com/nyaruka/gocommon/urns"

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
	channels.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() channels.Handler {
	return &handler{handlers.NewBaseHandler(models.ChannelType("MK"), "Macrokiosk")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(r *channels.Routes) error {
	r.Add(h, http.MethodPost, "status", models.ChannelLogTypeMsgStatus, h.receiveStatus)
	r.Add(h, http.MethodGet, "status", models.ChannelLogTypeMsgStatus, h.receiveStatus)
	r.Add(h, http.MethodGet, "receive", models.ChannelLogTypeMsgReceive, h.receiveMessage)
	r.Add(h, http.MethodPost, "receive", models.ChannelLogTypeMsgReceive, h.receiveMessage)
	return nil
}

type statusForm struct {
	MsgID  string `name:"msgid" validate:"required"`
	Status string `name:"status" validate:"required"`
}

var statusMapping = map[string]models.MsgStatus{
	"ACCEPTED":    models.MsgStatusSent,
	"DELIVERED":   models.MsgStatusDelivered,
	"UNDELIVERED": models.MsgStatusFailed,
	"PROCESSING":  models.MsgStatusWired,
}

// receiveStatus is our HTTP handler function for status updates
func (h *handler) receiveStatus(ctx context.Context, channel *models.Channel, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
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
	status := models.NewStatusUpdateByExternalID(channel, form.MsgID, msgStatus, clog)
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
func (h *handler) receiveMessage(ctx context.Context, channel *models.Channel, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
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
	urn, err := urns.ParsePhone(sender, channel.Country(), true, false)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// create and write the message
	msg := models.NewIncomingMsg(channel, urn, form.Text, form.MsgID, clog).WithReceivedOn(date.UTC())
	return handlers.WriteMsgsAndResponse(ctx, h, []*models.MsgIn{msg}, w, r, clog)
}

// WriteMsgSuccessResponse
func (h *handler) WriteMsgSuccessResponse(ctx context.Context, w http.ResponseWriter, msgs []*models.MsgIn) error {
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

func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	username := msg.Channel().StringConfigForKey(models.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(models.ConfigPassword, "")
	servID := msg.Channel().StringConfigForKey(configMacrokioskServiceID, "")
	senderID := msg.Channel().StringConfigForKey(configMacrokioskSenderID, "")
	if username == "" || password == "" || servID == "" || senderID == "" {
		return channels.ErrChannelConfig
	}

	// figure out if we need to send as unicode (encoding 5)
	text := gsm7.ReplaceSubstitutions(handlers.GetTextAndAttachments(msg))
	encoding := "0"
	if !gsm7.IsValid(text) {
		encoding = "5"
	}

	parts := handlers.SplitMsgByChannel(msg.Channel(), text, maxMsgLength)
	for _, part := range parts {
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
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err := handlers.ErrorFromResponse(resp, err); err != nil {
			return err
		}

		externalID, err := jsonparser.GetString(respBody, "MsgID")
		if err != nil {
			clog.Error(models.ErrorResponseValueMissing("MsgID"))
		} else {
			res.AddExternalID(externalID)
		}
	}
	return nil
}
