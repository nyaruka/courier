package clicksend

import (
	"bytes"
	"context"
	"net/http"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
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

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if username == "" || password == "" {
		return courier.ErrChannelConfig
	}

	parts := handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength)
	for _, part := range parts {
		payload := &mtPayload{}
		payload.Messages[0].To = msg.URN().Path()
		payload.Messages[0].From = msg.Channel().Address()
		payload.Messages[0].Body = part
		payload.Messages[0].Source = "courier"

		requestBody := jsonx.MustMarshal(payload)

		req, err := http.NewRequest(http.MethodPost, sendURL, bytes.NewReader(requestBody))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.SetBasicAuth(username, password)

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 == 5 {
			return courier.ErrConnectionFailed
		} else if resp.StatusCode/100 != 2 {
			return courier.ErrResponseStatus
		}

		s, _ := jsonparser.GetString(respBody, "data", "messages", "[0]", "status")
		if s != "SUCCESS" {
			return courier.ErrResponseContent
		}

		id, _ := jsonparser.GetString(respBody, "data", "messages", "[0]", "message_id")
		if id != "" {
			res.AddExternalID(id)
		} else {
			return courier.ErrResponseContent
		}
	}

	return nil
}

func (h *handler) RedactValues(ch courier.Channel) []string {
	return []string{
		httpx.BasicAuth(ch.StringConfigForKey(courier.ConfigUsername, ""), ch.StringConfigForKey(courier.ConfigPassword, "")),
	}
}
