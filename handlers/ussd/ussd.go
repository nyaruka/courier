package ussd

import (
	"context"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"net/http"
	"strings"
	"time"
)

const (
	configStartMsg    = "start_msg"
	configTimeOut     = "time_out"
	configStripPrefix = "strip_prefix"
)

const (
	SessionStatusWaiting     = "W"
)
func init() {
	courier.RegisterHandler(newHandler())
}

type response struct {
	resp         string
	wantResponse bool
}

type handler struct {
	handlers.BaseHandler

	requests map[string]chan response // the request waiters, indexed by from+sessionID
}

func newHandler() courier.ChannelHandler {
	return &handler{
		BaseHandler: handlers.NewBaseHandler(courier.ChannelType("US"), "USSD"),
		requests:    make(map[string]chan response),
	}
}

func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodGet, "status", h.receiveStatus)
	return nil
}

type moForm struct {
	ID          string `validate:"required" name:"sessionID"`
	Input       string `validate:"required" name:"ussdString"`
	Sender      string `validate:"required" name:"from"`
	ServiceCode string `validate:"required" name:"to"`
	MsgID       string `name:"messageID"`
}

// Make the key into the handler requests.
func makeKey(sender string, id string) string {
	return sender + "-" + id // Concatenate them
}

func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, writer http.ResponseWriter, request *http.Request) ([]courier.Event, error) {
	form := &moForm{}
	err := handlers.DecodeAndValidateForm(form, request)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, writer, request, err)
	}
	// create our URN
	urn, err := handlers.StrictTelForCountry(form.Sender, channel.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, writer, request, err)
	}
	date := time.Now().UTC() // Current time...
	// build our msg
	var input = form.Input

	var fkey = makeKey(urn.Path(), form.ID) // Use canonical phone number in key...

	if h.requests[fkey] == nil { // New session
		h.requests[fkey] = make(chan response, 100) // For waiting for the response from rapidPro
		var smsg = channel.StringConfigForKey(configStartMsg, "")
		if len(smsg) > 0 { // Use provided start message
			input = smsg
		}
	} else {
		var strip_prefix = channel.BoolConfigForKey(configStripPrefix, false)

		if strip_prefix {
			var idx = strings.LastIndex(input, "*")
			if idx > -1 {
				input = input[idx+1:] // Everything after the *
			}
		}
	}

	msg := h.Backend().NewIncomingMsg(channel, urn, input).WithExternalID(form.ID).WithReceivedOn(date)
	events, err := writeMsgs(ctx, h, []courier.Msg{msg})

	// Now wait for the response from the  and send it back
	var timeout = channel.IntConfigForKey(configTimeOut, 30)
	var v string
	var status int
	select {
	case res := <-h.requests[fkey]:
		v = res.resp
		if res.wantResponse {
			status = http.StatusAccepted
		}
	case <-time.After(time.Second * time.Duration(timeout)):
		status = http.StatusGatewayTimeout
		v = "time out waiting for response"
		h.requests[fkey] = nil // Clear it. Right?
	}

	// Send HTTP response.
	writer.Header().Set("Content-Type", "text/plain")
	writer.WriteHeader(status)
	_, err = writer.Write([]byte(v))
	return events, err
}

func (h *handler) receiveStatus(ctx context.Context, channel courier.Channel, writer http.ResponseWriter, request *http.Request) ([]courier.Event, error) {

	return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, writer, request, "shouldn't happen.")
}

func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	var sender = msg.URN().Path()
	var sessionID = msg.ResponseToExternalID()

	var resp = response{
		resp:         handlers.GetTextAndAttachments(msg),
		wantResponse: msg.SessionStatus() == SessionStatusWaiting,
	}

	fkey := makeKey(sender, sessionID)

	c := h.requests[fkey]

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgFailed)

	// Push out.
	if c != nil {
		c <- resp
		status.SetStatus(courier.MsgSent)
	}
	return status, nil
}

// Write message to backend, do not send http response.
func writeMsgs(ctx context.Context, h handlers.ResponseWriter, msgs []courier.Msg) ([]courier.Event, error) {
	events := make([]courier.Event, len(msgs), len(msgs))
	for i, m := range msgs {
		err := h.Backend().WriteMsg(ctx, m)
		if err != nil {
			return nil, err
		}
		events[i] = m
	}

	return events, nil
}
