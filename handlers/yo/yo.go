package yo

/*
GET /handlers/yo/received/uuid?account=12345&dest=8500&message=Msg&sender=256778021111
*/

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
	"github.com/pkg/errors"
)

var sendURL1 = "http://smgw1.yo.co.ug:9100/sendsms"
var sendURL2 = "http://41.220.12.201:9100/sendsms"
var sendURL3 = "http://164.40.148.210:9100/sendsms"

func init() {
	courier.RegisterHandler(NewHandler())
}

type handler struct {
	handlers.BaseHandler
}

// NewHandler returns a new Yo! handler
func NewHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("YO"), "YO!")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	err := s.AddHandlerRoute(h, "GET", "receive", h.ReceiveMessage)
	if err != nil {
		return err
	}

	return nil
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *handler) ReceiveMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	yoMessage := &yoMessage{}
	handlers.DecodeAndValidateQueryParams(yoMessage, r)

	// if this is a post, also try to parse the form body
	if r.Method == http.MethodPost {
		handlers.DecodeAndValidateForm(yoMessage, r)
	}

	// validate whether our required fields are present
	err := handlers.Validate(yoMessage)
	if err != nil {
		return nil, err
	}

	// must have one of from or sender set, error if neither
	sender := yoMessage.Sender
	if sender == "" {
		sender = yoMessage.From
	}
	if sender == "" {
		return nil, errors.New("must have one of 'sender' or 'from' set")
	}

	// if we have a date, parse it
	dateString := yoMessage.Date
	if dateString == "" {
		dateString = yoMessage.Time
	}

	date := time.Now()
	if dateString != "" {
		date, err = time.Parse(time.RFC3339Nano, dateString)
		if err != nil {
			return nil, errors.New("invalid date format, must be RFC 3339")
		}
	}

	// create our URN
	urn := urns.NewTelURNForCountry(sender, channel.Country())

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, yoMessage.Text).WithReceivedOn(date)

	// and write it
	err = h.Backend().WriteMsg(msg)
	if err != nil {
		return nil, err
	}

	return []courier.Event{msg}, courier.WriteMsgSuccess(w, r, []courier.Msg{msg})
}

type yoMessage struct {
	From   string `name:"from"`
	Sender string `name:"sender"`
	Text   string `validate:"required" name:"text"`
	Date   string `name:"date"`
	Time   string `name:"time"`
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(msg courier.Msg) (courier.MsgStatus, error) {

	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for YO channel")
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for YO channel")
	}

	// build our request
	form := url.Values{
		"origin":       []string{strings.TrimPrefix(msg.Channel().Address(), "+")},
		"sms_content":  []string{courier.GetTextAndAttachments(msg)},
		"destinations": []string{strings.TrimPrefix(msg.URN().Path(), "+")},
		"ybsacctno":    []string{username},
		"password":     []string{password},
	}

	var status courier.MsgStatus
	encodedForm := form.Encode()
	sendURLs := []string{sendURL1, sendURL2, sendURL3}

	for _, sendURL := range sendURLs {
		failed := false
		sendURL := fmt.Sprintf("%s?%s", sendURL, encodedForm)

		req, err := http.NewRequest(http.MethodGet, sendURL, nil)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr, err := utils.MakeHTTPRequest(req)

		if err != nil {
			failed = true
		}
		// record our status and log
		status = h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
		status.AddLog(log)

		if err != nil {
			return status, nil
		}

		responseQS, err := url.ParseQuery(string(rr.Body))

		if err != nil {
			failed = true
		}

		if !failed && rr.StatusCode != 200 && rr.StatusCode != 201 {
			failed = true
		}

		ybsAutocreateStatus, ok := responseQS["ybs_autocreate_status"]
		if !ok {
			ybsAutocreateStatus = []string{""}
		}

		if !failed && ybsAutocreateStatus[0] != "OK" {
			failed = true
		}

		ybsAutocreateMessage, ok := responseQS["ybs_autocreate_message"]

		if !ok {
			ybsAutocreateMessage = []string{""}
		}

		if failed && strings.Contains(ybsAutocreateMessage[0], "BLACKLISTED") {
			status.SetStatus(courier.MsgFailed)
			h.Backend().StopMsgContact(msg)
			return status, nil
		}

		if !failed {
			status.SetStatus(courier.MsgWired)
			return status, nil
		}

	}

	return status, errors.Errorf("received error from Yo! API")
}
