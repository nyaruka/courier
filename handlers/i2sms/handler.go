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
	"github.com/nyaruka/gocommon/urns"
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
	urn, err := urns.ParsePhone(from, c.Country(), true, false)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(ctx, c, urn, body, "", clog).WithReceivedOn(time.Now().UTC())
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
	ErrorDesc string `json:"error_desc"`
}

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	channelHash := msg.Channel().StringConfigForKey(configChannelHash, "")
	if username == "" || password == "" || channelHash == "" {
		return courier.ErrChannelConfig
	}

	for _, part := range handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength) {
		form := url.Values{
			"action":  []string{"send_single"},
			"mobile":  []string{strings.TrimLeft(msg.URN().Path(), "+")},
			"channel": []string{channelHash},
			"message": []string{part},
		}

		req, err := http.NewRequest(http.MethodPost, sendURL, strings.NewReader(form.Encode()))
		if err != nil {
			return err
		}
		req.SetBasicAuth(username, password)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 == 5 {
			return courier.ErrConnectionFailed
		} else if resp.StatusCode/100 != 2 {
			return courier.ErrResponseStatus
		}

		// parse our response as JSON
		response := &mtResponse{}
		err = json.Unmarshal(respBody, response)
		if err != nil {
			return courier.ErrResponseUnparseable
		}

		// we always get 00 on success
		if response.ErrorCode == "00" {
			res.AddExternalID(response.Result.SessionID)
		} else {
			return courier.ErrFailedWithReason(response.ErrorCode, response.ErrorDesc)
		}
	}

	return nil
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
