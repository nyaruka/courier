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

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/gocommon/stringsx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
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
func NewHandler(channelType string, name string, sendURL string, maxLength int) channels.Handler {
	return &handler{
		handlers.NewBaseHandler(models.ChannelType(channelType), name),
		sendURL,
		maxLength,
	}
}

func init() {
	channels.RegisterHandler(NewHandler("DA", "DartMedia", sendURL, maxMsgLength))
}

// Initialize registers the routes this handler serves
func (h *handler) Initialize(r *channels.Routes) error {
	r.AddReceive(h, http.MethodGet, "receive", models.ChannelLogTypeMsgReceive, handlers.FormPayload(h.receiveMessage))
	r.AddReceive(h, http.MethodGet, "delivered", models.ChannelLogTypeMsgStatus, handlers.FormPayload(h.receiveStatus))
	return nil
}

type moForm struct {
	Message   string `name:"message"`
	Original  string `name:"original"`
	SendTo    string `name:"sendto"`
	MessageID string `name:"messageid"`
}

// receiveMessage is our receive function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel *models.Channel, r *http.Request, form *moForm, in *channels.Received, clog *models.ChannelLog) error {
	if form.Original == "" || form.SendTo == "" {
		return fmt.Errorf("missing required parameters original and sendto")
	}

	// create our URN
	urn, err := urns.ParsePhone(form.Original, channel.Country(), true, false)
	if err != nil {
		urn, err = urns.New(urns.External, form.Original)
		if err != nil {
			return err
		}
	}

	// build our msg
	msg := models.NewIncomingMsg(channel, urn, form.Message, form.MessageID, clog)

	in.Msg(msg)
	return nil
}

type statusForm struct {
	MessageID string `name:"messageid"`
	Status    string `name:"status"`
}

// receiveStatus is our receive function for status updates
func (h *handler) receiveStatus(ctx context.Context, channel *models.Channel, r *http.Request, form *statusForm, in *channels.Received, clog *models.ChannelLog) error {
	if form.Status == "" || form.MessageID == "" {
		return fmt.Errorf("parameters messageid and status should not be empty")
	}

	statusInt, err := strconv.Atoi(form.Status)
	if err != nil {
		return fmt.Errorf("parsing failed: status '%s' is not an integer", form.Status)
	}

	msgStatus := models.MsgStatusSent
	if statusInt >= 10 && statusInt <= 12 {
		msgStatus = models.MsgStatusDelivered
	}

	if statusInt > 20 {
		msgStatus = models.MsgStatusFailed
	}

	msgUUID := strings.Split(form.MessageID, ".")[0]
	if !uuids.Is(msgUUID) {
		return fmt.Errorf("parsing failed: messageid '%s' is not a UUID", form.MessageID)
	}

	status := models.NewStatusUpdate(channel, models.MsgUUID(msgUUID), msgStatus, clog)
	in.Status(status)
	return nil
}

// DartMedia expects "000" from a status request
func (h *handler) RespondStatuses(ctx context.Context, w http.ResponseWriter, statuses []*models.StatusUpdate) error {
	w.WriteHeader(200)
	_, err := fmt.Fprint(w, "000")
	return err
}

// DartMedia expects "000" from a message receive request
func (h *handler) RespondMsgs(ctx context.Context, w http.ResponseWriter, msgs []*models.MsgIn) error {
	w.WriteHeader(200)
	_, err := fmt.Fprint(w, "000")
	return err
}

func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	username := msg.Channel().StringConfigForKey(models.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(models.ConfigPassword, "")
	if username == "" || password == "" {
		return channels.ErrChannelConfig
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

		messageid := string(msg.UUID())
		if i > 0 {
			messageid = fmt.Sprintf("%s.%d", msg.UUID(), i+1)
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
		if err := handlers.ErrorFromResponse(resp, err); err != nil {
			return err
		}

		responseCode := stringsx.Truncate(string(respBody), 3)
		if responseCode != "000" {
			return channels.ErrFailedWithReason(responseCode, errorCodes[responseCode])
		}

	}

	return nil
}
