package line

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
)

var (
	sendURL      = "https://api.line.me/v2/bot/message/push"
	maxMsgLength = 2000
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("LN"), "Line")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
	return nil
}

// {
// 	"events": [
// 	  {
// 		"replyToken": "nHuyWiB7yP5Zw52FIkcQobQuGDXCTA",
// 		"type": "message",
// 		"timestamp": 1462629479859,
// 		"source": {
// 		  "type": "user",
// 		  "userId": "U4af4980629..."
// 		},
// 		"message": {
// 		  "id": "325708",
// 		  "type": "text",
// 		  "text": "Hello, world"
// 		}
// 	  },
// 	  {
// 		"replyToken": "nHuyWiB7yP5Zw52FIkcQobQuGDXCTA",
// 		"type": "follow",
// 		"timestamp": 1462629479859,
// 		"source": {
// 		  "type": "user",
// 		  "userId": "U4af4980629..."
// 		}
// 	  }
// 	]
// }
type moPayload struct {
	Events []struct {
		Type      string `json:"type"`
		Timestamp int64  `json:"timestamp"`
		Source    struct {
			Type   string `json:"type"`
			UserID string `json:"userId"`
		} `json:"source"`
		Message struct {
			ID   string `json:"id"`
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"message"`
	} `json:"events"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	payload := &moPayload{}
	err := handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	msgs := []courier.Msg{}

	for _, lineEvent := range payload.Events {
		if (lineEvent.Source.Type == "" && lineEvent.Source.UserID == "") || (lineEvent.Message.Type == "" && lineEvent.Message.ID == "" && lineEvent.Message.Text == "") || lineEvent.Message.Type != "text" {

			continue
		}

		// create our date from the timestamp (they give us millis, arg is nanos)
		date := time.Unix(0, lineEvent.Timestamp*1000000).UTC()

		urn, err := urns.NewURNFromParts(urns.LineScheme, lineEvent.Source.UserID, "")
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}

		msg := h.Backend().NewIncomingMsg(channel, urn, lineEvent.Message.Text).WithReceivedOn(date)
		msgs = append(msgs, msg)
	}

	if len(msgs) == 0 {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring request, no message")
	}

	return handlers.WriteMsgsAndResponse(ctx, h, msgs, w, r)

}

type mtMsg struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mtPayload struct {
	To       string  `json:"to"`
	Messages []mtMsg `json:"messages"`
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	authToken := msg.Channel().StringConfigForKey(courier.ConfigAuthToken, "")
	if authToken == "" {
		return nil, fmt.Errorf("no auth token set for LN channel: %s", msg.Channel().UUID())
	}

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	parts := handlers.SplitMsg(handlers.GetTextAndAttachments(msg), maxMsgLength)
	for _, part := range parts {
		payload := mtPayload{
			To: msg.URN().Path(),
			Messages: []mtMsg{
				mtMsg{
					Type: "text",
					Text: part,
				},
			},
		}

		requestBody := &bytes.Buffer{}
		json.NewEncoder(requestBody).Encode(payload)

		// build our request
		req, _ := http.NewRequest(http.MethodPost, sendURL, requestBody)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authToken))

		rr, err := utils.MakeHTTPRequest(req)
		// record our status and log
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
		status.AddLog(log)

		if err != nil {
			return status, err
		}
		status.SetStatus(courier.MsgWired)
	}

	return status, nil

}
