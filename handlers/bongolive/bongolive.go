package bongolive

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/gsm7"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
)

var (
	sendURL      = "http://api.blsmsgw.com:8080/bin/send"
	maxMsgLength = 160
)

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("BL"), "Bongo Live")}
}

func init() {
	courier.RegisterHandler(newHandler())
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodGet, "receive", h.receiveMessage)

	s.AddHandlerRoute(h, http.MethodPost, "status", h.receiveStatus)
	s.AddHandlerRoute(h, http.MethodGet, "status", h.receiveStatus)
	return nil
}

type moForm struct {
	ID      string `name:"ID"`
	To      string `name:"DESTADDR"`
	From    string `name:"SOURCEADDR" validate:"required"`
	Message string `name:"MESSAGE"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	var err error
	form := &moForm{}

	err = handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// create our URN
	urn, err := handlers.StrictTelForCountry(form.From, channel.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, form.Message).WithExternalID(form.ID).WithReceivedOn(time.Now().UTC())

	// and finally queue our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r)
}

func (h *handler) WriteMsgSuccessResponse(ctx context.Context, w http.ResponseWriter, r *http.Request, msgs []courier.Msg) error {
	return writeBongoLiveResponse(w)
}

func (h *handler) WriteStatusSuccessResponse(ctx context.Context, w http.ResponseWriter, r *http.Request, statuses []courier.MsgStatus) error {
	return writeBongoLiveResponse(w)
}

func (h *handler) WriteRequestIgnored(ctx context.Context, w http.ResponseWriter, r *http.Request, details string) error {
	return writeBongoLiveResponse(w)
}

func writeBongoLiveResponse(w http.ResponseWriter) error {
	w.Header().Add("Content-type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte{})
	return err

}

var statusMapping = map[int]courier.MsgStatusValue{
	1:  courier.MsgDelivered,
	2:  courier.MsgSent,
	3:  courier.MsgErrored,
	4:  courier.MsgErrored,
	5:  courier.MsgErrored,
	6:  courier.MsgErrored,
	7:  courier.MsgErrored,
	8:  courier.MsgSent,
	9:  courier.MsgErrored,
	10: courier.MsgErrored,
	11: courier.MsgErrored,
}

type statusForm struct {
	ID     courier.MsgID `validate:"required" name:"id"`
	Status int           `validate:"required" name:"status"`
}

// receiveStatus is our HTTP handler function for status updates
func (h *handler) receiveStatus(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	// get our params
	form := &statusForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	msgStatus, found := statusMapping[form.Status]
	if !found {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unknown status '%d', must be one of 1,2,3,4,5,6,7,8,9,10,11", form.Status))
	}

	// write our status
	status := h.Backend().NewMsgStatusForID(channel, form.ID, msgStatus)
	err = h.Backend().WriteMsgStatus(ctx, status)
	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for %s channel", msg.Channel().ChannelType())
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for %s channel", msg.Channel().ChannelType())
	}

	apiKey := msg.Channel().StringConfigForKey(courier.ConfigAPIKey, "")
	if apiKey == "" {
		return nil, fmt.Errorf("no api key set for %s channel", msg.Channel().ChannelType())
	}

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	parts := handlers.SplitMsg(handlers.GetTextAndAttachments(msg), maxMsgLength)
	for _, part := range parts {
		form := url.Values{
			"username":   []string{username},
			"password":   []string{password},
			"apikey":     []string{apiKey},
			"sourceaddr": []string{strings.TrimPrefix(msg.Channel().Address(), "+")},
			"destaddr":   []string{strings.TrimPrefix(msg.URN().Path(), "+")},
			"message":    []string{part},
			"dlr":        []string{"1"},
			"dlrid":      []string{msg.ID().String()},
		}

		replaced := gsm7.ReplaceSubstitutions(part)
		if gsm7.IsValid(replaced) {
			form["message"] = []string{replaced}
		} else {
			form["charcode"] = []string{"2"}
		}

		partSendURL, _ := url.Parse(sendURL)
		partSendURL.RawQuery = form.Encode()

		req, _ := http.NewRequest(http.MethodGet, partSendURL.String(), nil)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr, err := utils.MakeHTTPRequest(req)

		// record our status and log
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Send Error", err)
		status.AddLog(log)
		if err == nil {
			status.SetStatus(courier.MsgWired)
		}

		if rr.StatusCode == 403 {
			status.SetStatus(courier.MsgFailed)
		}

		if err != nil {
			return status, nil
		}

	}
	return status, nil
}
