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

	dartmediaSendURL = "http://202.43.169.11/APIhttpU/receive2waysms.php"
	dartmediaMaxMsgLength = 160

	hub9SendURL = "http://175.103.48.29:28078/testing/smsmt.php"
	hub9MaxMsgLength = 1600
)

type handler struct {
	handlers.BaseHandler
}

// NewHandler returns a new DartMedia ready to be registered
func NewHandler(channelType string, name string) courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType(channelType), name)}
}

func init() {
	courier.RegisterHandler(NewHandler("DA", "DartMedia"))
	courier.RegisterHandler(NewHandler("H9", "Hub9"))
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	err := s.AddHandlerRoute(h, http.MethodGet, "receive", h.ReceiveMessage)
	if err != nil {
		return err
	}

	err = s.AddHandlerRoute(h, http.MethodGet, "received", h.ReceiveMessage)
	if err != nil {
		return err
	}

	return s.AddHandlerRoute(h, http.MethodGet, "delivered", h.StatusMessage)
}

type dartStatus struct {
	MessageID string `name:"messageid"`
	Status string `name:"status"`
}


type dartMessage struct {
	Message string `name:"message"`
	From string `name:"original"`
	To string `name:"sendto"`
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *handler) ReceiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	daMessage := &dartMessage{}
	err := handlers.DecodeAndValidateForm(daMessage, r)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	// create our URN
	urn := urns.NewTelURNForCountry(daMessage.From, channel.Country())

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, daMessage.Message)

	// and finally queue our message
	err = h.Backend().WriteMsg(ctx, msg)
	if err != nil {
		return nil, err
	}

	return []courier.Event{msg}, h.writeReceiveSuccess(ctx, w, r, msg)
}

// StatusMessage is our HTTP handler function for status updates
func (h *handler) StatusMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	daStatus := &dartStatus{}
	err := handlers.DecodeAndValidateForm(daStatus, r)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	if daStatus.Status == "" || daStatus.MessageID == "" {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf("parameters messageid and status should not be null"))
	}

	statusInt, err := strconv.Atoi(daStatus.Status)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf("parsing failed: status '%s' is not an integer", daStatus.Status))
	}

	msgStatus := courier.MsgSent
	if statusInt >= 10 && statusInt <= 12 {
		msgStatus = courier.MsgDelivered
	}

	if statusInt > 20 {
		msgStatus = courier.MsgFailed
	}

	msgID, err := strconv.ParseInt(daStatus.MessageID, 10, 64)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf("parsing failed: messageid '%s' is not an integer", daStatus.MessageID))
	}

	// write our status
	status := h.Backend().NewMsgStatusForID(channel, courier.NewMsgID(msgID), msgStatus)
	err = h.Backend().WriteMsgStatus(ctx, status)
	if err != nil {
		return nil, err
	}

	return []courier.Event{status}, h.writeStasusSuccess(ctx, w, r, status)
}

// DartMedia expects "000" from a message receive request
func (h *handler) writeReceiveSuccess(ctx context.Context, w http.ResponseWriter, r *http.Request, msg courier.Msg) error {
	courier.LogMsgReceived(r, msg)
	w.WriteHeader(200)
	_, err := fmt.Fprint(w, "000")
	return err
}

// DartMedia expects "000" from a status request
func (h *handler) writeStasusSuccess(ctx context.Context, w http.ResponseWriter, r *http.Request, status courier.MsgStatus) error {
	courier.LogMsgStatusReceived(r, status)
	w.WriteHeader(200)
	_, err := fmt.Fprint(w, "000")
	return err
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	sendURL := dartmediaSendURL
	maxMsgLength := dartmediaMaxMsgLength
	channelType := msg.Channel().ChannelType().String()

	if channelType == "H9" {
		sendURL = hub9SendURL
		maxMsgLength = hub9MaxMsgLength
	}

	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for %s channel", channelType)
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for %s channel", channelType)
	}

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	parts := handlers.SplitMsg(courier.GetTextAndAttachments(msg), maxMsgLength)
	for _, part := range parts {
		form := url.Values{
			"userid":     []string{username},
			"password": []string{password},
			"sendto":       []string{strings.TrimPrefix(msg.URN().Path(), "+")},
			"original":     []string{strings.TrimPrefix(msg.Channel().Address(), "+")},
			"messageid": []string{msg.ID().String()},
			"udhl":   []string{"0"},
			"dcs":   []string{"0"},
			"message":  []string{part},
		}

		encodedForm := form.Encode()

		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s?%s", sendURL, encodedForm), nil)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr, err := utils.MakeHTTPRequest(req)

		// record our status and log
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr)
		status.AddLog(log)
		if err != nil {
			log.WithError("Message Send Error", err)
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
			log.WithError("Message Send Error", fmt.Errorf(errorMessage))
			return status, nil
		}

		status.SetStatus(courier.MsgWired)

	}
	return status, nil
}