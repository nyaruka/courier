package i2sms

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
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
	return &handler{handlers.NewBaseHandler(courier.ChannelType("I2"), "I2SMS")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	receiveHandler := handlers.NewTelReceiveHandler(&h.BaseHandler, "mobile", "message")
	s.AddHandlerRoute(h, http.MethodPost, "receive", receiveHandler)
	return nil
}

// {
//	 "​result​":{
//	   "submit_result":"OK",
//     "session_id":"5b8fc97d58795484819426",
//     "status_code":"00",
//     "status_message":"Submitted ok"
//   },
//   "​error_code​":"00",
//   "error_desc​":"Completed OK"
// }
type mtResponse struct {
	Result struct {
		SessionID string `json:"session_id"`
	} `json:"result"`
	ErrorCode string `json:"error_code"`
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(_ context.Context, msg courier.Msg) (courier.MsgStatus, error) {
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

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	for _, part := range handlers.SplitMsg(handlers.GetTextAndAttachments(msg), maxMsgLength) {
		form := url.Values{
			"action":  []string{"send_single"},
			"mobile":  []string{strings.TrimLeft(msg.URN().Path(), "+")},
			"channel": []string{channelHash},
			"message": []string{part},
		}

		req, _ := http.NewRequest(http.MethodPost, sendURL, strings.NewReader(form.Encode()))
		req.SetBasicAuth(username, password)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		rr, err := utils.MakeHTTPRequest(req)

		// record our status and log
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
		status.AddLog(log)
		if err != nil {
			return status, nil
		}

		// parse our response as JSON
		response := &mtResponse{}
		err = json.Unmarshal(rr.Body, response)
		if err != nil {
			log.WithError("Message Send Error", err)
			break
		}

		// we always get 00 on success
		fmt.Println(string(rr.Body))
		fmt.Printf("%++v\n", response)
		if response.ErrorCode == "00" {
			status.SetStatus(courier.MsgWired)
			status.SetExternalID(response.Result.SessionID)
		} else {
			status.SetStatus(courier.MsgFailed)
			log.WithError("Message Send Error", fmt.Errorf("Received invalid response code: %s", response.ErrorCode))
			break
		}
	}

	return status, nil
}
