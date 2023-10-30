package yo

/*
GET /handlers/yo/received/uuid?account=12345&dest=8500&message=Msg&sender=256778021111
*/

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/pkg/errors"
)

var (
	sendURLs = []string{
		"http://smgw1.yo.co.ug:9100/sendsms",
		"http://41.220.12.201:9100/sendsms",
		"http://164.40.148.210:9100/sendsms",
	}
	maxMsgLength = 1600
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("YO"), "YO!")}
}

func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodGet, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
	return nil
}

type moForm struct {
	From    string `name:"from"`
	Sender  string `name:"sender"`
	Message string `name:"message"`
	Date    string `name:"date"`
	Time    string `name:"time"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	form := &moForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// must have one of from or sender set, error if neither
	sender := form.Sender
	if sender == "" {
		sender = form.From
	}
	if sender == "" {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, errors.New("must have one of 'sender' or 'from'"))
	}

	// if we have a date, parse it
	dateString := form.Date
	if dateString == "" {
		dateString = form.Time
	}

	date := time.Now()
	if dateString != "" {
		date, err = time.Parse(time.RFC3339Nano, dateString)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, errors.New("invalid date format, must be RFC 3339"))
		}
	}

	// create our URN
	urn, err := handlers.StrictTelForCountry(sender, channel.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// build our msg
	dbMsg := h.Backend().NewIncomingMsg(channel, urn, form.Message, "", clog).WithReceivedOn(date)

	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{dbMsg}, w, r, clog)
}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for YO channel")
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for YO channel")
	}

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)
	var err error

	for _, part := range handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength) {
		form := url.Values{
			"origin":       []string{strings.TrimPrefix(msg.Channel().Address(), "+")},
			"sms_content":  []string{part},
			"destinations": []string{strings.TrimPrefix(msg.URN().Path(), "+")},
			"ybsacctno":    []string{username},
			"password":     []string{password},
		}

		for _, sendURL := range sendURLs {
			sendURL, _ := url.Parse(sendURL)
			sendURL.RawQuery = form.Encode()

			req, err := http.NewRequest(http.MethodGet, sendURL.String(), nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			resp, respBody, err := h.RequestHTTP(req, clog)
			if err != nil || resp.StatusCode/100 != 2 {
				return status, nil
			}

			responseQS, _ := url.ParseQuery(string(respBody))

			// check whether we were blacklisted
			createMessage := responseQS["ybs_autocreate_message"]
			if len(createMessage) > 0 && strings.Contains(createMessage[0], "BLACKLISTED") {
				status.SetStatus(courier.MsgStatusFailed)

				// create a stop channel event
				channelEvent := h.Backend().NewChannelEvent(msg.Channel(), courier.EventTypeStopContact, msg.URN(), clog)
				err = h.Backend().WriteChannelEvent(ctx, channelEvent, clog)
				if err != nil {
					return nil, err
				}

				return status, nil
			}

			// finally check that we were sent
			createStatus := responseQS["ybs_autocreate_status"]
			if len(createStatus) > 0 && createStatus[0] == "OK" {
				status.SetStatus(courier.MsgStatusWired)
				return status, nil
			}
		}
	}

	return status, err
}
