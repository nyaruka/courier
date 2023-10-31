package highconnection

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
)

var (
	sendURL      = "https://highpushfastapi-v2.hcnx.eu/api"
	maxMsgLength = 1500
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("HX"), "High Connection")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodGet, "status", courier.ChannelLogTypeMsgStatus, h.receiveStatus)
	return nil
}

type moForm struct {
	ID          int64  `name:"ID"`
	To          string `name:"TO"              validate:"required"`
	From        string `name:"FROM"            validate:"required"`
	Message     string `name:"MESSAGE"`
	ReceiveDate string `name:"RECEPTION_DATE"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	form := &moForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	date := time.Now()
	if form.ReceiveDate != "" {
		date, err = time.Parse("2006-01-02T15:04:05", form.ReceiveDate)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}
	}

	// create our URN
	urn, err := handlers.StrictTelForCountry(form.From, channel.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// Hign connection URL encodes escapes ISO 8859 escape sequences
	text, _ := url.QueryUnescape(form.Message)
	// decode from ISO 8859
	text = mime.BEncoding.Encode("ISO-8859-1", text)
	text, _ = new(mime.WordDecoder).DecodeHeader(text)

	msgID := ""
	if form.ID != 0 {
		msgID = strconv.FormatInt(form.ID, 10)
	}

	// build our Message
	msg := h.Backend().NewIncomingMsg(channel, urn, text, msgID, clog).WithReceivedOn(date.UTC())

	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
}

type statusForm struct {
	RetID  int64 `name:"ret_id" validate:"required"`
	Status int   `name:"status" validate:"required"`
}

var statusMapping = map[int]courier.MsgStatus{
	2:  courier.MsgStatusFailed,
	4:  courier.MsgStatusSent,
	6:  courier.MsgStatusDelivered,
	11: courier.MsgStatusFailed,
	12: courier.MsgStatusFailed,
	13: courier.MsgStatusFailed,
	14: courier.MsgStatusFailed,
	15: courier.MsgStatusFailed,
	16: courier.MsgStatusFailed,
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
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unknown status '%d', must be one of 2, 4, 6, 11, 12, 13, 14, 15  or 16", form.Status))
	}

	// write our status
	status := h.Backend().NewStatusUpdate(channel, courier.MsgID(form.RetID), msgStatus, clog)
	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for HX channel")
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for HX channel")
	}

	callbackDomain := msg.Channel().CallbackDomain(h.Server().Config().Domain)
	statusURL := fmt.Sprintf("https://%s/c/hx/%s/status", callbackDomain, msg.Channel().UUID())
	receiveURL := fmt.Sprintf("https://%s/c/hx/%s/receive", callbackDomain, msg.Channel().UUID())

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)
	parts := handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength)

	var flowName string
	if msg.Flow() != nil {
		flowName = msg.Flow().Name
	}

	for _, part := range parts {
		form := url.Values{
			"accountid":  []string{username},
			"password":   []string{password},
			"text":       []string{part},
			"to":         []string{msg.URN().Path()},
			"ret_id":     []string{msg.ID().String()},
			"datacoding": []string{"8"},
			"user_data":  []string{flowName},
			"ret_url":    []string{statusURL},
			"ret_mo_url": []string{receiveURL},
		}

		msgURL, _ := url.Parse(sendURL)
		msgURL.RawQuery = form.Encode()

		req, err := http.NewRequest(http.MethodPost, msgURL.String(), nil)
		if err != nil {
			return nil, err
		}

		resp, _, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 != 2 {
			return status, nil
		}

		status.SetStatus(courier.MsgStatusWired)

	}

	return status, nil
}
