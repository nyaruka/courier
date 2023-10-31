package i2sms

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/httpx"
)

const (
	configChannelHash = "channel_hash"
)

var (
	sendURL      = "https://mx2.i2sms.net/mxapi.php"
	maxMsgLength = 640
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("I2"), "I2SMS", handlers.WithRedactConfigKeys(courier.ConfigPassword, configChannelHash))}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, h.receive)
	return nil
}

// receive is our handler for MO messages
func (h *handler) receive(ctx context.Context, c courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	err := r.ParseForm()
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
	}

	body := r.Form.Get("message")
	from := r.Form.Get("mobile")
	if from == "" {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, fmt.Errorf("missing required field 'mobile'"))
	}

	// create our URN
	urn, err := handlers.StrictTelForCountry(from, c.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(c, urn, body, "", clog).WithReceivedOn(time.Now().UTC())
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
}

//	{
//		 "​result​":{
//		   "submit_result":"OK",
//	    "session_id":"5b8fc97d58795484819426",
//	    "status_code":"00",
//	    "status_message":"Submitted ok"
//	  },
//	  "​error_code​":"00",
//	  "error_desc​":"Completed OK"
//	}
type mtResponse struct {
	Result struct {
		SessionID string `json:"session_id"`
	} `json:"result"`
	ErrorCode string `json:"error_code"`
}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for I2 channel")
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for I2 channel")
	}

	channelHash := msg.Channel().StringConfigForKey(configChannelHash, "")
	if channelHash == "" {
		return nil, fmt.Errorf("no channel_hash set for I2 channel")
	}

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)
	for _, part := range handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength) {
		form := url.Values{
			"action":  []string{"send_single"},
			"mobile":  []string{strings.TrimLeft(msg.URN().Path(), "+")},
			"channel": []string{channelHash},
			"message": []string{part},
		}

		req, err := http.NewRequest(http.MethodPost, sendURL, strings.NewReader(form.Encode()))
		if err != nil {
			return nil, err
		}
		req.SetBasicAuth(username, password)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 != 2 {
			return status, nil
		}

		// parse our response as JSON
		response := &mtResponse{}
		err = json.Unmarshal(respBody, response)
		if err != nil {
			clog.Error(courier.ErrorResponseUnparseable("JSON"))
			break
		}

		// we always get 00 on success
		if response.ErrorCode == "00" {
			status.SetStatus(courier.MsgStatusWired)
			status.SetExternalID(response.Result.SessionID)
		} else {
			status.SetStatus(courier.MsgStatusFailed)
			clog.Error(courier.ErrorResponseValueUnexpected("error_code", "00"))
			break
		}
	}

	return status, nil
}

func (h *handler) RedactValues(ch courier.Channel) []string {
	return []string{
		httpx.BasicAuth(ch.StringConfigForKey(courier.ConfigUsername, ""), ch.StringConfigForKey(courier.ConfigPassword, "")),
		ch.StringConfigForKey(configChannelHash, ""),
	}
}

// WriteMsgSuccessResponse writes a success response for the messages, i2SMS expects an empty body in our response
func (h *handler) WriteMsgSuccessResponse(ctx context.Context, w http.ResponseWriter, msgs []courier.MsgIn) error {
	w.Header().Add("Content-type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte{})
	return err
}
