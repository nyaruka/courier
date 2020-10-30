package kaleyra

import (
	"context"
	"fmt"
	"github.com/buger/jsonparser"
	"github.com/gorilla/schema"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
	"github.com/pkg/errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	configAccountSID = "account_sid"
	configApiKey     = "api_key"
)

var baseURL = "https://api.kaleyra.io"

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("KWA"), "Kaleyra WhatsApp")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodGet, "receive", h.receiveMsg)
	s.AddHandlerRoute(h, http.MethodGet, "status", h.receiveStatus)
	return nil
}

type moMsgForm struct {
	CreatedAt string `name:"created_at"`
	Type      string `name:"type"`
	From      string `name:"from"`
	Name      string `name:"name"`
	Body      string `name:"body"`
	MediaURL  string `name:"media_url"`
}

type moStatusForm struct {
	ID     string `name:"id"     validate:"required"`
	Status string `name:"status" validate:"required"`
}

// receiveMsg is our HTTP handler function for incoming messages
func (h *handler) receiveMsg(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	form := &moMsgForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	if form.Type != "text" && form.Type != "image" && form.Type != "video" && form.Type != "voice" && form.Type != "document" {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring request, unknown message type")
	}
	if form.Body == "" && form.MediaURL == "" {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, errors.New("no text or media"))
	}

	urn, err := urns.NewWhatsAppURN(form.From)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	ts, err := strconv.ParseInt(form.CreatedAt, 10, 64)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("invalid created_at: %s", form.CreatedAt))
	}

	date := time.Unix(ts, 0).UTC()
	msg := h.Backend().NewIncomingMsg(channel, urn, form.Body).WithAttachment(form.MediaURL).WithReceivedOn(date).WithContactName(form.Name)

	return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r)
}

var statusMapping = map[string]courier.MsgStatusValue{
	"0":         courier.MsgFailed,
	"sent":      courier.MsgWired,
	"delivered": courier.MsgDelivered,
	"read":      courier.MsgDelivered,
}

// receiveStatus is our HTTP handler function for outgoing messages statuses
func (h *handler) receiveStatus(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	form := &moStatusForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	msgStatus, found := statusMapping[form.Status]
	if !found {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, fmt.Sprintf("unknown status: %s", form.Status))
	}

	status := h.Backend().NewMsgStatusForExternalID(channel, form.ID, msgStatus)
	if status == nil {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, fmt.Sprintf("ignoring request, message %s not found", form.ID))
	}

	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

type mtForm struct {
	ApiKey      string `schema:"api-key"`
	Channel     string `schema:"channel"`
	From        string `schema:"from"`
	CallbackURL string `schema:"callback_url"`
	Type        string `schema:"type"`
	To          string `schema:"to"`
}

type mtTextForm struct {
	mtForm
	Body string `schema:"body"`
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	accountSID := msg.Channel().StringConfigForKey(configAccountSID, "")
	apiKey := msg.Channel().StringConfigForKey(configApiKey, "")

	if accountSID == "" || apiKey == "" {
		return nil, errors.New("no account_sid or api_key config")
	}

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)

	textForm := mtTextForm{
		h.newSendForm(msg.Channel(), "text", msg.URN().Path()),
		msg.Text(),
	}
	form := url.Values{}

	err := schema.NewEncoder().Encode(textForm, form)
	if err != nil {
		return status, errors.Wrapf(err, "error encoding payload")
	}

	sendURL := fmt.Sprintf("%s/v1/%s/messages", baseURL, accountSID)
	req, _ := http.NewRequest(http.MethodPost, sendURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := utils.MakeHTTPRequest(req)
	log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), res).WithError("Message Send Error", err)
	status.AddLog(log)

	if err != nil {
		status.SetStatus(courier.MsgFailed)
		return status, nil
	}

	externalID, err := jsonparser.GetString(res.Body, "id")
	if err == nil {
		status.SetExternalID(externalID)
	}

	status.SetStatus(courier.MsgWired)
	return status, nil
}

func (h *handler) newSendForm(channel courier.Channel, msgType, toContact string) mtForm {
	callbackDomain := channel.CallbackDomain(h.Server().Config().Domain)
	statusURL := fmt.Sprintf("https://%s/c/kwa/%s/status", callbackDomain, channel.UUID())

	return mtForm{
		ApiKey:      channel.StringConfigForKey(configApiKey, ""),
		Channel:     "WhatsApp",
		From:        channel.Address(),
		CallbackURL: statusURL,
		Type:        msgType,
		To:          toContact,
	}
}
