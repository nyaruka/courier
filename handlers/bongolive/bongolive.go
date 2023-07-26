package bongolive

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/gsm7"

	"github.com/buger/jsonparser"
)

var (
	sendURL      = "https://api.blsmsgw.com:8443/bin/send.json"
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
	return nil
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

type moForm struct {
	ID      string `name:"ID"`
	DLRID   string `name:"DLRID"`
	To      string `name:"DESTADDR"`
	From    string `name:"SOURCEADDR" `
	Message string `name:"MESSAGE"`
	MsgType int    `name:"MSGTYPE"`
	Status  int    `name:"STATUS"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	var err error
	form := &moForm{}
	err = handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	if form.MsgType == 5 {
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}

		msgStatus, found := statusMapping[form.Status]
		if !found {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unknown status '%d', must be one of 1,2,3,4,5,6,7,8,9,10,11", form.Status))
		}

		// write our status
		status := h.Backend().NewMsgStatusForExternalID(channel, form.DLRID, msgStatus, clog)
		return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
	}

	// create our URN
	urn, err := handlers.StrictTelForCountry(form.From, channel.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, form.Message, clog).WithExternalID(form.ID).WithReceivedOn(time.Now().UTC())

	// and finally queue our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r, clog)

}

func (h *handler) WriteMsgSuccessResponse(ctx context.Context, w http.ResponseWriter, msgs []courier.Msg) error {
	return writeBongoLiveResponse(w)
}

func (h *handler) WriteStatusSuccessResponse(ctx context.Context, w http.ResponseWriter, statuses []courier.MsgStatus) error {
	return writeBongoLiveResponse(w)
}

func (h *handler) WriteRequestIgnored(ctx context.Context, w http.ResponseWriter, details string) error {
	return writeBongoLiveResponse(w)
}

func writeBongoLiveResponse(w http.ResponseWriter) error {
	w.Header().Add("Content-type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte{})
	return err

}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.Msg, clog *courier.ChannelLog) (courier.MsgStatus, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for %s channel", msg.Channel().ChannelType())
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for %s channel", msg.Channel().ChannelType())
	}

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored, clog)
	parts := handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength)
	for _, part := range parts {
		form := url.Values{
			"USERNAME":   []string{username},
			"PASSWORD":   []string{password},
			"SOURCEADDR": []string{strings.TrimPrefix(msg.Channel().Address(), "+")},
			"DESTADDR":   []string{strings.TrimPrefix(msg.URN().Path(), "+")},
			"MESSAGE":    []string{part},
			"DLR":        []string{"1"},
		}

		replaced := gsm7.ReplaceSubstitutions(part)
		if gsm7.IsValid(replaced) {
			form["MESSAGE"] = []string{replaced}
		} else {
			form["CHARCODE"] = []string{"2"}
		}

		partSendURL, _ := url.Parse(sendURL)
		partSendURL.RawQuery = form.Encode()

		req, err := http.NewRequest(http.MethodPost, partSendURL.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, respBody, err := handlers.RequestHTTPInsecure(req, clog)
		if err != nil || resp.StatusCode/100 != 2 {
			return status, nil
		}

		// was this request successful?
		msgStatus, _ := jsonparser.GetString(respBody, "results", "[0]", "status")
		if msgStatus != "0" {
			status.SetStatus(courier.MsgErrored)
			return status, nil
		}
		// grab the external id if we can
		externalID, _ := jsonparser.GetString(respBody, "results", "[0]", "msgid")
		status.SetStatus(courier.MsgWired)
		handlers.CacheAndSetMsgExternalID(h.Backend().RedisPool(), status, externalID, msg)

	}
	return status, nil
}
