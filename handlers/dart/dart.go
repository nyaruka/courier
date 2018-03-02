package dart

/*
GET /handlers/dartmedia/received/uuid?userid=username&password=xxxxxxxx&original=6285218761111&sendto=93456&messagetype=0&messageid=170503131327@170504131327@93456SMS9755064&message=Msg&date=20170503131559&dcs=0&udhl=0&charset=utf-8
*/

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
)

var (
	sendURL      = "http://202.43.169.11/APIhttpU/receive2waysms.php"
	maxMsgLength = 160
)

type handler struct {
	handlers.BaseHandler
	sendURL   string
	maxLength int
}

// NewHandler returns a new DartMedia ready to be registered
func NewHandler(channelType string, name string, sendURL string, maxLength int) courier.ChannelHandler {
	return &handler{
		handlers.NewBaseHandler(courier.ChannelType(channelType), name),
		sendURL,
		maxLength,
	}
}

func init() {
	courier.RegisterHandler(NewHandler("DA", "DartMedia", sendURL, maxMsgLength))
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	err := s.AddHandlerRoute(h, http.MethodGet, "receive", h.receiveMessage)
	if err != nil {
		return err
	}

	return s.AddHandlerRoute(h, http.MethodGet, "delivered", h.receiveStatus)
}

type moForm struct {
	Message  string `name:"message"`
	Original string `name:"original"`
	SendTo   string `name:"sendto"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	form := &moForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	if form.Original == "" || form.SendTo == "" {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf("missing required parameters original and sendto"))
	}

	// create our URN
	urn, err := urns.NewTelURNForCountry(form.Original, channel.Country())
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, form.Message)

	// and finally queue our message
	err = h.Backend().WriteMsg(ctx, msg)
	if err != nil {
		return nil, err
	}

	return []courier.Event{msg}, h.writeReceiveSuccess(ctx, w, r, msg)
}

type statusForm struct {
	MessageID string `name:"messageid"`
	Status    string `name:"status"`
}

// receiveStatus is our HTTP handler function for status updates
func (h *handler) receiveStatus(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	form := &statusForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	if form.Status == "" || form.MessageID == "" {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf("parameters messageid and status should not be empty"))
	}

	statusInt, err := strconv.Atoi(form.Status)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf("parsing failed: status '%s' is not an integer", form.Status))
	}

	msgStatus := courier.MsgSent
	if statusInt >= 10 && statusInt <= 12 {
		msgStatus = courier.MsgDelivered
	}

	if statusInt > 20 {
		msgStatus = courier.MsgFailed
	}

	msgID, err := strconv.ParseInt(form.MessageID, 10, 64)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf("parsing failed: messageid '%s' is not an integer", form.MessageID))
	}

	// write our status
	status := h.Backend().NewMsgStatusForID(channel, courier.NewMsgID(msgID), msgStatus)
	err = h.Backend().WriteMsgStatus(ctx, status)
	if err != nil && err != courier.ErrMsgNotFound {
		return nil, err
	}

	return []courier.Event{status}, h.writeStatusSuccess(ctx, w, r, status)
}

// DartMedia expects "000" from a message receive request
func (h *handler) writeReceiveSuccess(ctx context.Context, w http.ResponseWriter, r *http.Request, msg courier.Msg) error {
	w.WriteHeader(200)
	_, err := fmt.Fprint(w, "000")
	return err
}

// DartMedia expects "000" from a status request
func (h *handler) writeStatusSuccess(ctx context.Context, w http.ResponseWriter, r *http.Request, status courier.MsgStatus) error {
	w.WriteHeader(200)
	_, err := fmt.Fprint(w, "000")
	return err
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

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	parts := handlers.SplitMsg(handlers.GetTextAndAttachments(msg), h.maxLength)
	for _, part := range parts {
		form := url.Values{
			"userid":    []string{username},
			"password":  []string{password},
			"sendto":    []string{strings.TrimPrefix(msg.URN().Path(), "+")},
			"original":  []string{strings.TrimPrefix(msg.Channel().Address(), "+")},
			"messageid": []string{msg.ID().String()},
			"udhl":      []string{"0"},
			"dcs":       []string{"0"},
			"message":   []string{part},
		}

		partSendURL, _ := url.Parse(h.sendURL)
		partSendURL.RawQuery = form.Encode()

		req, _ := http.NewRequest(http.MethodGet, partSendURL.String(), nil)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr, err := utils.MakeHTTPRequest(req)

		// record our status and log
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Send Error", err)
		status.AddLog(log)
		if err != nil {
			return status, nil
		}

		responseText := fmt.Sprintf("%s", rr.Body)
		if responseText != "000" {
			errorMessage := "Unknown error"
			if responseText == "001" {
				errorMessage = "Error 001: Authentication Error"
			}
			if responseText == "101" {
				errorMessage = "Error 101: Account expired or invalid parameters"
			}
			log.WithError("Send Error", fmt.Errorf(errorMessage))
			return status, nil
		}

		status.SetStatus(courier.MsgWired)

	}
	return status, nil
}
