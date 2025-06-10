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
	"github.com/nyaruka/gocommon/stringsx"
	"github.com/nyaruka/gocommon/urns"
)

var (
	sendURL      = "http://202.43.169.11/APIhttpU/receive2waysms.php"
	maxMsgLength = 160

	errorCodes = map[string]string{
		"001": "Authentication error.",
		"101": "Account expired or invalid parameters.",
	}
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
	s.AddHandlerRoute(h, http.MethodGet, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodGet, "delivered", courier.ChannelLogTypeMsgStatus, h.receiveStatus)
	return nil
}

type moForm struct {
	Message   string `name:"message"`
	Original  string `name:"original"`
	SendTo    string `name:"sendto"`
	MessageID string `name:"messageid"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	form := &moForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	if form.Original == "" || form.SendTo == "" {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("missing required parameters original and sendto"))
	}

	// create our URN
	urn, err := urns.ParsePhone(form.Original, channel.Country(), true, false)
	if err != nil {
		urn, err = urns.New(urns.External, form.Original)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(ctx, channel, urn, form.Message, form.MessageID, clog)

	// and finally queue our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
}

type statusForm struct {
	MessageID string `name:"messageid"`
	Status    string `name:"status"`
}

// receiveStatus is our HTTP handler function for status updates
func (h *handler) receiveStatus(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	form := &statusForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	if form.Status == "" || form.MessageID == "" {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("parameters messageid and status should not be empty"))
	}

	statusInt, err := strconv.Atoi(form.Status)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("parsing failed: status '%s' is not an integer", form.Status))
	}

	msgStatus := courier.MsgStatusSent
	if statusInt >= 10 && statusInt <= 12 {
		msgStatus = courier.MsgStatusDelivered
	}

	if statusInt > 20 {
		msgStatus = courier.MsgStatusFailed
	}

	msgID, err := strconv.ParseInt(strings.Split(form.MessageID, ".")[0], 10, 64)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("parsing failed: messageid '%s' is not an integer", form.MessageID))
	}

	// write our status
	status := h.Backend().NewStatusUpdate(channel, courier.MsgID(msgID), msgStatus, clog)
	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

// DartMedia expects "000" from a message receive request
func (h *handler) WriteStatusSuccessResponse(ctx context.Context, w http.ResponseWriter, statuses []courier.StatusUpdate) error {
	w.WriteHeader(200)
	_, err := fmt.Fprint(w, "000")
	return err
}

// DartMedia expects "000" from a status request
func (h *handler) WriteMsgSuccessResponse(ctx context.Context, w http.ResponseWriter, msgs []courier.MsgIn) error {
	w.WriteHeader(200)
	_, err := fmt.Fprint(w, "000")
	return err
}

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if username == "" || password == "" {
		return courier.ErrChannelConfig
	}

	parts := handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), h.maxLength)
	for i, part := range parts {
		form := url.Values{
			"userid":   []string{username},
			"password": []string{password},
			"sendto":   []string{strings.TrimPrefix(msg.URN().Path(), "+")},
			"original": []string{strings.TrimPrefix(msg.Channel().Address(), "+")},
			"udhl":     []string{"0"},
			"dcs":      []string{"0"},
			"message":  []string{part},
		}

		messageid := msg.ID().String()
		if i > 0 {
			messageid = fmt.Sprintf("%s.%d", msg.ID().String(), i+1)
		}
		form["messageid"] = []string{messageid}

		partSendURL, _ := url.Parse(h.sendURL)
		partSendURL.RawQuery = form.Encode()

		req, err := http.NewRequest(http.MethodGet, partSendURL.String(), nil)
		if err != nil {
			return err
		}

		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 == 5 {
			return courier.ErrConnectionFailed
		} else if resp.StatusCode/100 != 2 {
			return courier.ErrResponseStatus
		}

		responseCode := stringsx.Truncate(string(respBody), 3)
		if responseCode != "000" {
			return courier.ErrFailedWithReason(responseCode, errorCodes[responseCode])
		}

	}

	return nil
}
