package line

import (
	"context"
	"fmt"
	"github.com/nyaruka/gocommon/urns"
	"net/http"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
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
	return s.AddHandlerRoute(h, http.MethodPost, "receive", h.ReceiveMessage)
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
type moMsg struct {
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

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *handler) ReceiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	lineRequest := &moMsg{}
	err := handlers.DecodeAndValidateJSON(lineRequest, r)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	msgs := []courier.Msg{}
	for _, lineEvent := range lineRequest.Events {
		if (lineEvent.Source.Type == "" && lineEvent.Source.UserID == "") || (lineEvent.Message.Type == "" && lineEvent.Message.ID == "" && lineEvent.Message.Text == "") || lineEvent.Message.Type != "text" {

			continue
		}

		// create our date from the timestamp (they give us millis, arg is nanos)
		date := time.Unix(0, lineEvent.Timestamp*1000000).UTC()

		urn := urns.NewURNFromParts(urns.LineScheme, lineEvent.Source.UserID, "")

		msg := h.Backend().NewIncomingMsg(channel, urn, lineEvent.Message.Text).WithReceivedOn(date)

		// and write it
		err = h.Backend().WriteMsg(ctx, msg)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, msg)
	}

	if len(msgs) == 0 {
		if len(lineRequest.Events) > 0 {
			return nil, courier.WriteAndLogRequestIgnored(ctx, w, r, channel, "ignoring request, no message")
		}
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf("missing message, source or type in the event"))

	}

	return []courier.Event{msgs[0]}, courier.WriteMsgSuccess(ctx, w, r, msgs)

}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	return nil, fmt.Errorf("LN sending via courier not yet implemented")
}
