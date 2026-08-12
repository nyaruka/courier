package m3tech

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/gocommon/gsm7"
	"github.com/nyaruka/gocommon/urns"
)

var (
	sendURL      = "https://secure.m3techservice.com/GenericServiceRestAPI/api/SendSMS"
	maxMsgLength = 160
)

func init() {
	channels.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() channels.Handler {
	return &handler{handlers.NewBaseHandler(models.ChannelType("M3"), "M3Tech")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(r *channels.Routes) error {
	r.Add(h, http.MethodPost, "receive", models.ChannelLogTypeMsgReceive, h.receiveMessage)
	return nil
}

// receiveMessage takes care of handling incoming messages
func (h *handler) receiveMessage(ctx context.Context, c *models.Channel, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
	err := r.ParseForm()
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
	}

	body := r.Form.Get("text")
	from := r.Form.Get("from")
	if from == "" {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, fmt.Errorf("missing required field 'from'"))
	}

	// create our URN
	urn, err := urns.ParsePhone(from, c.Country(), true, false)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
	}

	// create and write the message
	msg := models.NewIncomingMsg(c, urn, body, "", clog).WithReceivedOn(time.Now().UTC())
	return handlers.WriteMsgsAndResponse(ctx, h, []*models.MsgIn{msg}, w, r, clog)
}

// WriteMsgSuccessResponse writes a success response for the messages
func (h *handler) WriteMsgSuccessResponse(ctx context.Context, w http.ResponseWriter, msgs []*models.MsgIn) error {
	w.Header().Set("Content-Type", "application/json")
	_, err := fmt.Fprintf(w, "SMS Accepted: %s", msgs[0].UUID())
	return err
}

func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	username := msg.Channel().StringConfigForKey(models.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(models.ConfigPassword, "")
	if username == "" || password == "" {
		return channels.ErrChannelConfig
	}

	// figure out if we need to send as unicode (encoding 7)
	text := gsm7.ReplaceSubstitutions(handlers.GetTextAndAttachments(msg))
	encoding := "0"
	if !gsm7.IsValid(text) {
		encoding = "7"
	}

	for _, part := range handlers.SplitMsgByChannel(msg.Channel(), text, maxMsgLength) {
		// build our request
		params := url.Values{
			"AuthKey":     []string{"m3-Tech"},
			"UserId":      []string{username},
			"Password":    []string{password},
			"SMS":         []string{part},
			"SMSType":     []string{encoding},
			"MobileNo":    []string{strings.TrimPrefix(msg.URN().Path(), "+")},
			"MsgId":       []string{string(msg.UUID())},
			"MsgHeader":   []string{strings.TrimPrefix(msg.Channel().Address(), "+")},
			"HandsetPort": []string{"0"},
			"SMSChannel":  []string{"0"},
			"Telco":       []string{"0"},
		}

		msgURL, _ := url.Parse(sendURL)

		msgURL.RawQuery = params.Encode()
		req, err := http.NewRequest(http.MethodGet, msgURL.String(), nil)
		if err != nil {
			return err
		}

		resp, _, err := h.RequestHTTP(req, clog)
		if err := handlers.ErrorFromResponse(resp, err); err != nil {
			return err
		}
	}

	return nil
}
