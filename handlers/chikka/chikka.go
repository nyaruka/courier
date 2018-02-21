package chikka

import (
	"context"
	"fmt"
	"github.com/nyaruka/courier/utils"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/urns"
)

var (
	sendURL      = "https://post.chikka.com/smsapi/request"
	maxMsgLength = 160
)

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("CK"), "Chikka")}
}

func init() {
	courier.RegisterHandler(newHandler())
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	return s.AddHandlerRoute(h, http.MethodPost, "receive", h.ReceiveMessage)
}

type moMsg struct {
	MessageType  string  `name:"message_type" validate:"required"`
	RequestID    string  `name:"request_id"`
	MobileNumber string  `name:"mobile_number"`
	Message      string  `name:"message"`
	Timestamp    float64 `name:"timestamp"`
	MessageID    int64   `name:"message_id"`
	Status       string  `name:"status"`
}

var statusMapping = map[string]courier.MsgStatusValue{
	"SENT":   courier.MsgSent,
	"FAILED": courier.MsgFailed,
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *handler) ReceiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	ckRequest := &moMsg{}
	err := handlers.DecodeAndValidateForm(ckRequest, r)
	if err != nil {
		return nil, err
	}

	if ckRequest.MessageType == "outgoing" {

		msgStatus, found := statusMapping[ckRequest.Status]
		if !found {
			return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf(`unknown status '%s', must be either 'SENT' or 'FAILED'`, ckRequest.Status))
		}

		// write our status
		status := h.Backend().NewMsgStatusForID(channel, courier.NewMsgID(ckRequest.MessageID), msgStatus)
		err = h.Backend().WriteMsgStatus(ctx, status)
		if err != nil {
			return nil, err
		}

		return []courier.Event{status}, courier.WriteStatusSuccess(ctx, w, r, []courier.MsgStatus{status})

	} else if ckRequest.MessageType == "incoming" {

		// create our date from the timestamp
		date := time.Unix(0, int64(ckRequest.Timestamp*1000000000)).UTC()

		// create our URN
		urn := urns.NewTelURNForCountry(ckRequest.MobileNumber, channel.Country())

		// build our msg
		msg := h.Backend().NewIncomingMsg(channel, urn, ckRequest.Message).WithExternalID(ckRequest.RequestID).WithReceivedOn(date)

		// and finally queue our message
		err = h.Backend().WriteMsg(ctx, msg)
		if err != nil {
			return nil, err
		}

		return []courier.Event{msg}, courier.WriteMsgSuccess(ctx, w, r, []courier.Msg{msg})
	} else {
		return nil, courier.WriteAndLogRequestIgnored(ctx, w, r, channel, "unknown message_type request")
	}
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for CK channel")
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for CK channel")
	}

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	parts := handlers.SplitMsg(handlers.GetTextAndAttachments(msg), maxMsgLength)
	for _, part := range parts {
		// build our request
		form := url.Values{
			"message_type":  []string{"SEND"},
			"mobile_number": []string{strings.TrimLeft(msg.URN().Path(), "+")},
			"shortcode":     []string{strings.TrimLeft(msg.Channel().Address(), "+")},
			"message_id":    []string{msg.ID().String()},
			"message":       []string{part},
			"request_cost":  []string{"FREE"},
			"client_id":     []string{username},
			"secret_key":    []string{password},
		}
		if !msg.ResponseToID().IsZero() {
			form["message_type"] = []string{"REPLY"}
			form["request_id"] = []string{msg.ResponseToID().String()}
		}

		req, _ := http.NewRequest(http.MethodPost, sendURL, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr, err := utils.MakeHTTPRequest(req)

		// record our status and log
		status.AddLog(courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err))
		if err != nil {
			return status, nil
		}

		status.SetStatus(courier.MsgWired)

	}

	return status, nil
}
