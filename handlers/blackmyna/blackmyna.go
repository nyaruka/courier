package blackmyna

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/pkg/errors"
)

var sendURL = "http://api.blackmyna.com/2/smsmessaging/outbound"

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("BM"), "Blackmyna")}
}

func init() {
	courier.RegisterHandler(newHandler())
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodGet, "receive", h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodGet, "status", h.StatusMessage)
	return nil
}

type moForm struct {
	To   string `validate:"required" name:"to"`
	Text string `validate:"required" name:"text"`
	From string `validate:"required" name:"from"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLogger) ([]courier.Event, error) {
	// get our params
	form := &moForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, err
	}

	// create our URN
	urn, err := handlers.StrictTelForCountry(form.From, channel.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, form.Text)

	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r)
}

type statusForm struct {
	ID     string `validate:"required" name:"id"`
	Status int    `validate:"required" name:"status"`
}

var statusMapping = map[int]courier.MsgStatusValue{
	1:  courier.MsgDelivered,
	2:  courier.MsgFailed,
	8:  courier.MsgSent,
	16: courier.MsgFailed,
}

// StatusMessage is our HTTP handler function for status updates
func (h *handler) StatusMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLogger) ([]courier.Event, error) {
	// get our params
	form := &statusForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, err
	}

	msgStatus, found := statusMapping[form.Status]
	if !found {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unknown status '%d', must be one of 1, 2, 8 or 16", form.Status))
	}

	// write our status
	status := h.Backend().NewMsgStatusForExternalID(channel, form.ID, msgStatus)
	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.Msg, clog *courier.ChannelLogger) (courier.MsgStatus, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for BM channel")
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for BM channel")
	}

	apiKey := msg.Channel().StringConfigForKey(courier.ConfigAPIKey, "")
	if apiKey == "" {
		return nil, fmt.Errorf("no API key set for BM channel")
	}

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)

	// build our request
	form := url.Values{
		"address":       []string{msg.URN().Path()},
		"senderaddress": []string{msg.Channel().Address()},
		"message":       []string{handlers.GetTextAndAttachments(msg)},
	}

	req, err := http.NewRequest(http.MethodPost, sendURL, strings.NewReader(form.Encode()))

	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(username, password)

	resp, respBody, err := handlers.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return status, nil
	}

	// get our external id
	externalID, _ := jsonparser.GetString(respBody, "[0]", "id")
	if externalID == "" {
		return status, errors.Errorf("no external id returned in body")
	}

	status.SetStatus(courier.MsgWired)
	status.SetExternalID(externalID)

	return status, nil
}
