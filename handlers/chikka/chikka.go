package chikka

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/buger/jsonparser"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
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
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
	return nil
}

type moForm struct {
	RequestID    string  `name:"request_id"    validate:"required"`
	MobileNumber string  `name:"mobile_number" validate:"required"`
	Message      string  `name:"message"`
	Timestamp    float64 `name:"timestamp"     validate:"required"`
}

type statusForm struct {
	MessageID int64  `name:"message_id" validate:"required"`
	Status    string `name:"status"     validate:"required"`
}

var statusMapping = map[string]courier.MsgStatusValue{
	"SENT":   courier.MsgSent,
	"FAILED": courier.MsgFailed,
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	err := r.ParseForm()
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}
	messageType := r.Form.Get("message_type")

	if messageType == "outgoing" {
		form := &statusForm{}
		err := handlers.DecodeAndValidateForm(form, r)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}

		msgStatus, found := statusMapping[form.Status]
		if !found {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf(`unknown status '%s', must be either 'SENT' or 'FAILED'`, form.Status))
		}

		// write our status
		status := h.Backend().NewMsgStatusForID(channel, courier.NewMsgID(form.MessageID), msgStatus)
		return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)

	} else if messageType == "incoming" {
		form := &moForm{}
		err := handlers.DecodeAndValidateForm(form, r)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}
		// create our date from the timestamp
		date := time.Unix(0, int64(form.Timestamp*1000000000)).UTC()

		// create our URN
		urn, err := handlers.StrictTelForCountry(form.MobileNumber, channel.Country())
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}
		// build our msg
		msg := h.Backend().NewIncomingMsg(channel, urn, form.Message).WithExternalID(form.RequestID).WithReceivedOn(date)

		// and finally write our message
		return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r)
	} else {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "unknown message_type request")
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
		if msg.ResponseToExternalID() != "" {
			form["message_type"] = []string{"REPLY"}
			form["request_id"] = []string{msg.ResponseToExternalID()}
		}

		req, _ := http.NewRequest(http.MethodPost, sendURL, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr, err := utils.MakeHTTPRequest(req)

		if rr.StatusCode == 400 {
			message, _ := jsonparser.GetString([]byte(rr.Body), "message")
			description, _ := jsonparser.GetString([]byte(rr.Body), "description")

			if message == "BAD REQUEST" && description == `Invalid\/Used Request ID` {
				delete(form, "request_id")
				form["message_type"] = []string{"SEND"}

				req, _ = http.NewRequest(http.MethodPost, sendURL, strings.NewReader(form.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				rr, err = utils.MakeHTTPRequest(req)

			}

		}

		// record our status and log
		status.AddLog(courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err))
		if err != nil {
			return status, nil
		}

		status.SetStatus(courier.MsgWired)

	}

	return status, nil
}
