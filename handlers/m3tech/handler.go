package m3tech

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
	"github.com/nyaruka/gocommon/urns"
)

var (
	sendURL      = "https://secure.m3techservice.com/GenericServiceRestAPI/api/SendSMS"
	maxMsgLength = 160
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("M3"), "M3Tech")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
	return nil
}

// receiveMessage takes care of handling incoming messages
func (h *handler) receiveMessage(ctx context.Context, c courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
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
	msg := h.Backend().NewIncomingMsg(ctx, c, urn, body, "", clog).WithReceivedOn(time.Now().UTC())
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
}

// WriteMsgSuccessResponse writes a success response for the messages
func (h *handler) WriteMsgSuccessResponse(ctx context.Context, w http.ResponseWriter, msgs []courier.MsgIn) error {
	w.Header().Set("Content-Type", "application/json")
	_, err := fmt.Fprintf(w, "SMS Accepted: %d", msgs[0].ID())
	return err
}

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if username == "" || password == "" {
		return courier.ErrChannelConfig
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
			"MsgId":       []string{msg.ID().String()},
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
		if err != nil || resp.StatusCode/100 == 5 {
			return courier.ErrConnectionFailed
		} else if resp.StatusCode/100 != 2 {
			return courier.ErrResponseStatus
		}
	}

	return nil
}
