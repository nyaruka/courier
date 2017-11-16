package infobip

import (
	"fmt"
	"net/http"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/urns"
)

/* no logs */

func init() {
	courier.RegisterHandler(NewHandler())
}

type handler struct {
	handlers.BaseHandler
}

// NewHandler returns a new Infobip handler
func NewHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("IB"), "Infobip")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	err := s.AddReceiveMsgRoute(h, "POST", "receive", h.ReceiveMessage)
	if err != nil {
		return err
	}
	return s.AddUpdateStatusRoute(h, "POST", "delivered", h.StatusMessage)
}

// StatusMessage is our HTTP handler function for status updates
func (h *handler) StatusMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.MsgStatus, error) {
	ibStatusEnvelop := &ibStatusEnvelop{}
	err := handlers.DecodeAndValidateJSON(ibStatusEnvelop, r)
	if err != nil {
		return nil, courier.WriteError(w, r, err)
	}

	msgStatus, found := infobipStatusMapping[ibStatusEnvelop.Results[0].Status.GroupName]
	if !found {
		return nil, courier.WriteError(w, r, fmt.Errorf("unknown status '%s', must be one of PENDING, DELIVERED, EXPIRED, REJECTED or UNDELIVERABLE", ibStatusEnvelop.Results[0].Status.GroupName))
	}

	// write our status
	status := h.Backend().NewMsgStatusForID(channel, courier.NewMsgID(ibStatusEnvelop.Results[0].MessageID), msgStatus)
	err = h.Backend().WriteMsgStatus(status)
	if err != nil {
		return nil, err
	}

	return []courier.MsgStatus{status}, courier.WriteStatusSuccess(w, r, []courier.MsgStatus{status})
}

var infobipStatusMapping = map[string]courier.MsgStatusValue{
	"PENDING":       courier.MsgSent,
	"EXPIRED":       courier.MsgSent,
	"DELIVERED":     courier.MsgDelivered,
	"REJECTED":      courier.MsgFailed,
	"UNDELIVERABLE": courier.MsgFailed,
}

type ibStatusEnvelop struct {
	Results []ibStatus `validate:"required" json:"results"`
}
type ibStatus struct {
	MessageID int64 `validate:"required" json:"messageId"`
	Status    struct {
		GroupName string `validate:"required" json:"groupName"`
	} `validate:"required" json:"status"`
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *handler) ReceiveMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.ReceiveEvent, error) {
	ie := &infobipEnvelope{}
	err := handlers.DecodeAndValidateJSON(ie, r)
	if err != nil {
		return nil, courier.WriteError(w, r, err)
	}

	if ie.MessageCount == 0 {
		return nil, courier.WriteIgnored(w, r, "ignoring request, no message")
	}

	msgs := []courier.Msg{}
	for _, infobipMessage := range ie.Results {
		messageID := infobipMessage.MessageID
		text := infobipMessage.Text
		dateString := infobipMessage.ReceivedAt

		if text == "" {
			continue
		}

		date := time.Now()
		if dateString != "" {
			date, err = time.Parse("2006-01-02T15:04:05.999999999-0700", dateString)
			if err != nil {
				return nil, courier.WriteError(w, r, err)
			}
		}

		// create our URN
		urn := urns.NewTelURNForCountry(infobipMessage.From, channel.Country())

		// build our infobipMessage
		msg := h.Backend().NewIncomingMsg(channel, urn, text).WithReceivedOn(date).WithExternalID(messageID)

		// and write it
		err = h.Backend().WriteMsg(msg)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, msg)

	}

	if len(msgs) == 0 {
		return nil, courier.WriteIgnored(w, r, "ignoring request, no message")
	}

	return []courier.ReceiveEvent{msgs[0]}, courier.WriteMsgSuccess(w, r, msgs)
}

type infobipMessage struct {
	MessageID  string `json:"messageId"`
	From       string `json:"from" validate:"required"`
	Text       string `json:"text"`
	ReceivedAt string `json:"receivedAt"`
}

// {
// 	"results": [
// 	  {
// 		"messageId": "817790313235066447",
// 		"from": "385916242493",
// 		"to": "385921004026",
// 		"text": "QUIZ Correct answer is Paris",
// 		"cleanText": "Correct answer is Paris",
// 		"keyword": "QUIZ",
// 		"receivedAt": "2016-10-06T09:28:39.220+0000",
// 		"smsCount": 1,
// 		"price": {
// 		  "pricePerMessage": 0,
// 		  "currency": "EUR"
// 		},
// 		"callbackData": "callbackData"
// 	  }
// 	],
// 	"messageCount": 1,
// 	"pendingMessageCount": 0
// }
type infobipEnvelope struct {
	PendingMessageCount int              `json:"pendingMessageCount"`
	MessageCount        int              `json:"messageCount"`
	Results             []infobipMessage `validate:"required" json:"results"`
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(msg courier.Msg) (courier.MsgStatus, error) {
	return nil, nil
}
