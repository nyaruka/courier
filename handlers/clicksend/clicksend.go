package clicksend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/pkg/errors"
)

var (
	maxMsgLength = 1224
	sendURL      = "https://rest.clicksend.com/v3/sms/send"
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("CS"), "ClickSend")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", handlers.NewTelReceiveHandler(&h.BaseHandler, "from", "body"))
	return nil
}

// {
// 	"messages": [
// 	  {
// 		"to": "+61411111111",
// 		"source": "sdk",
// 		"body": "body"
// 	  },
// 	  {
// 		"list_id": 0,
// 		"source": "sdk",
// 		"body": "body"
// 	  }
// 	]
// }
type mtPayload struct {
	Messages [1]struct {
		To     string `json:"to"`
		From   string `json:"from"`
		Body   string `json:"body"`
		Source string `json:"source"`
	} `json:"messages"`
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("Missing 'username' config for CS channel")
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("Missing 'password' config for CS channel")
	}

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	parts := handlers.SplitMsg(handlers.GetTextAndAttachments(msg), maxMsgLength)
	for _, part := range parts {
		payload := &mtPayload{}
		payload.Messages[0].To = msg.URN().Path()
		payload.Messages[0].From = msg.Channel().Address()
		payload.Messages[0].Body = part
		payload.Messages[0].Source = "courier"

		requestBody := &bytes.Buffer{}
		json.NewEncoder(requestBody).Encode(payload)

		// build our request
		req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf(sendURL, msg.Channel().Address()), requestBody)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.SetBasicAuth(username, password)

		rr, err := utils.MakeHTTPRequest(req)
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
		status.AddLog(log)
		if err != nil {
			return status, nil
		}

		// first read our status
		s, err := jsonparser.GetString(rr.Body, "data", "messages", "[0]", "status")
		if s != "SUCCESS" {
			log.WithError("Message Send Error", errors.Errorf("received non SUCCESS status: %s", s))
			return status, nil
		}

		// then get our external id
		id, err := jsonparser.GetString(rr.Body, "data", "messages", "[0]", "message_id")
		if err != nil {
			log.WithError("Message Send Error", errors.Errorf("unable to get message_id for message"))
			return status, nil
		}

		status.SetExternalID(id)
		status.SetStatus(courier.MsgWired)
	}

	return status, nil
}
