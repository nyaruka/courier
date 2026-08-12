package clicksend

import (
	"bytes"
	"context"
	"net/http"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
)

var (
	maxMsgLength = 1224
	sendURL      = "https://rest.clicksend.com/v3/sms/send"
)

func init() {
	channels.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() channels.Handler {
	return &handler{handlers.NewBaseHandler(models.ChannelType("CS"), "ClickSend")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(r *channels.Routes) error {
	r.Add(h, http.MethodPost, "receive", models.ChannelLogTypeMsgReceive, handlers.NewTelReceiveHandler(h, "from", "body"))
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

func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	username := msg.Channel().StringConfigForKey(models.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(models.ConfigPassword, "")
	if username == "" || password == "" {
		return channels.ErrChannelConfig
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
		if err := handlers.ErrorFromResponse(resp, err); err != nil {
			return err
		}

		s, _ := jsonparser.GetString(respBody, "data", "messages", "[0]", "status")
		if s != "SUCCESS" {
			return channels.ErrResponseContent
		}

		id, _ := jsonparser.GetString(respBody, "data", "messages", "[0]", "message_id")
		if id != "" {
			res.AddExternalID(id)
		} else {
			return channels.ErrResponseContent
		}
	}

	return nil
}

func (h *handler) RedactValues(ch *models.Channel) []string {
	return []string{
		httpx.BasicAuth(ch.StringConfigForKey(models.ConfigUsername, ""), ch.StringConfigForKey(models.ConfigPassword, "")),
	}
}
