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
	"github.com/nyaruka/gocommon/httpx"
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
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, handlers.NewTelReceiveHandler(h, "from", "body"))
	return nil
}

//	{
//		"messages": [
//		  {
//			"to": "+61411111111",
//			"source": "sdk",
//			"body": "body"
//		  },
//		  {
//			"list_id": 0,
//			"source": "sdk",
//			"body": "body"
//		  }
//		]
//	}
type mtPayload struct {
	Messages [1]struct {
		To     string `json:"to"`
		From   string `json:"from"`
		Body   string `json:"body"`
		Source string `json:"source"`
	} `json:"messages"`
}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("Missing 'username' config for CS channel")
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("Missing 'password' config for CS channel")
	}

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)
	parts := handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength)
	for _, part := range parts {
		payload := &mtPayload{}
		payload.Messages[0].To = msg.URN().Path()
		payload.Messages[0].From = msg.Channel().Address()
		payload.Messages[0].Body = part
		payload.Messages[0].Source = "courier"

		requestBody := &bytes.Buffer{}
		json.NewEncoder(requestBody).Encode(payload)

		// build our request
		req, err := http.NewRequest(http.MethodPost, sendURL, requestBody)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.SetBasicAuth(username, password)

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 != 2 {
			return status, nil
		}

		// first read our status
		s, _ := jsonparser.GetString(respBody, "data", "messages", "[0]", "status")
		if s != "SUCCESS" {
			clog.Error(courier.ErrorResponseValueUnexpected("status", "SUCCESS"))
			return status, nil
		}

		// then get our external id
		id, err := jsonparser.GetString(respBody, "data", "messages", "[0]", "message_id")
		if err != nil {
			clog.Error(courier.ErrorResponseValueMissing("message_id"))
			return status, nil
		}

		status.SetExternalID(id)
		status.SetStatus(courier.MsgStatusWired)
	}

	return status, nil
}

func (h *handler) RedactValues(ch courier.Channel) []string {
	return []string{
		httpx.BasicAuth(ch.StringConfigForKey(courier.ConfigUsername, ""), ch.StringConfigForKey(courier.ConfigPassword, "")),
	}
}
