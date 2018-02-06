package mtarget

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
)

var sendURL = "https://api-public.mtarget.fr/api-sms.json"
var maxLength = 765
var statuses = map[string]courier.MsgStatusValue{
	"0": courier.MsgWired,
	"1": courier.MsgWired,
	"2": courier.MsgSent,
	"3": courier.MsgDelivered,
	"4": courier.MsgFailed,
	"6": courier.MsgFailed,
}

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("MT"), "Mtarget")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)

	err := s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMsg)
	if err != nil {
		return nil
	}

	statusHandler := handlers.NewExternalIDQueryStatusHandler(h.BaseHandler, statuses, "MsgId", "Status")
	return s.AddHandlerRoute(h, http.MethodPost, "status", statusHandler)
}

// ReceiveMsg handles both MO messages and Stop commands
func (h *handler) receiveMsg(ctx context.Context, c courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	text := r.URL.Query().Get("Content")
	from := r.URL.Query().Get("Msisdn")
	keyword := r.URL.Query().Get("Keyword")

	if from == "" {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, c, fmt.Errorf("missing required field 'Msisdn'"))
	}

	// create our URN
	urn := urns.NewTelURNForCountry(from, c.Country())

	// if this a stop command, shortcut stopping that contact
	if keyword == "Stop" {
		stop := h.Backend().NewChannelEvent(c, courier.StopContact, urn)
		err := h.Backend().WriteChannelEvent(ctx, stop)
		if err != nil {
			return nil, err
		}
		return []courier.Event{stop}, courier.WriteChannelEventSuccess(ctx, w, r, stop)
	}

	// otherwise, create our incoming message and write that
	msg := h.Backend().NewIncomingMsg(c, urn, text).WithReceivedOn(time.Now().UTC())
	err := h.Backend().WriteMsg(ctx, msg)
	if err != nil {
		return nil, err
	}
	return []courier.Event{msg}, courier.WriteMsgSuccess(ctx, w, r, []courier.Msg{msg})
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for MT channel")
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for MT channel")
	}

	// send our message
	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	for _, part := range handlers.SplitMsg(handlers.GetTextAndAttachments(msg), maxLength) {
		// build our request
		params := url.Values{
			"username": []string{username},
			"password": []string{password},
			"msisdn":   []string{msg.URN().Path()},
			"msg":      []string{part},
		}

		msgURL, _ := url.Parse(sendURL)
		msgURL.RawQuery = params.Encode()
		req, err := http.NewRequest(http.MethodPost, msgURL.String(), nil)
		if err != nil {
			return nil, err
		}

		rr, err := utils.MakeHTTPRequest(req)
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
		status.AddLog(log)
		if err != nil {
			break
		}

		// parse our response for our status code and ticket (external id)
		// {
		//	"results": [{
		//		"msisdn": "+447xxxxxxxx",
		//		"smscount": "1",
		//		"code": "0",
		//		"reason": "ACCEPTED",
		//		"ticket": "760eeaa0-5034-11e7-bb92-00000a0a643a"
		//  }]
		// }
		code, _ := jsonparser.GetString(rr.Body, "results", "[0]", "code")
		externalID, _ := jsonparser.GetString(rr.Body, "results", "[0]", "ticket")
		if code == "0" && externalID != "" {
			// all went well, set ourselves to wired
			status.SetStatus(courier.MsgWired)
			status.SetExternalID(externalID)
		} else {
			status.SetStatus(courier.MsgFailed)
			log.WithError("Message Send Error", fmt.Errorf("Error status code, failing permanently"))
			break
		}
	}

	return status, nil
}
