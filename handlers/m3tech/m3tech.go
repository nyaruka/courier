package m3tech

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"strings"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/gsm7"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
)

var sendURL = "https://secure.m3techservice.com/GenericServiceRestAPI/api/SendSMS"
var maxLength = 160

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
	receiveHandler := handlers.NewTelQueryReceiveHandler(h.BaseHandler, "from", "text")
	return s.AddHandlerRoute(h, http.MethodPost, "receive", receiveHandler)
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for M3 channel")
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for M3 channel")
	}

	// figure out if we need to send as unicode (encoding 7)
	text := gsm7.ReplaceSubstitutions(handlers.GetTextAndAttachments(msg))
	encoding := "0"
	if !gsm7.IsValid(text) {
		encoding = "7"
	}

	// send our message
	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	for _, part := range handlers.SplitMsg(text, maxLength) {
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
			return nil, err
		}

		rr, err := utils.MakeHTTPRequest(req)
		status.AddLog(courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err))
		if err != nil {
			break
		}

		// all went well, set ourselves to wired
		status.SetStatus(courier.MsgWired)
	}

	return status, nil
}
