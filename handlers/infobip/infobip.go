package infobip

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
	"github.com/pkg/errors"
)

var sendURL = "https://api.infobip.com/sms/1/text/advanced"

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("IB"), "Infobip")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	err := s.AddHandlerRoute(h, "POST", "receive", h.ReceiveMessage)
	if err != nil {
		return err
	}
	return s.AddHandlerRoute(h, "POST", "delivered", h.StatusMessage)
}

// StatusMessage is our HTTP handler function for status updates
func (h *handler) StatusMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	ibStatusEnvelope := &ibStatusEnvelope{}
	err := handlers.DecodeAndValidateJSON(ibStatusEnvelope, r)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	data := make([]interface{}, len(ibStatusEnvelope.Results))
	statuses := make([]courier.Event, len(ibStatusEnvelope.Results))
	for _, s := range ibStatusEnvelope.Results {
		msgStatus, found := infobipStatusMapping[s.Status.GroupName]
		if !found {
			return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf("unknown status '%s', must be one of PENDING, DELIVERED, EXPIRED, REJECTED or UNDELIVERABLE", s.Status.GroupName))
		}

		// write our status
		status := h.Backend().NewMsgStatusForExternalID(channel, s.MessageID, msgStatus)
		err = h.Backend().WriteMsgStatus(ctx, status)
		if err == courier.ErrMsgNotFound {
			data = append(data, courier.NewInfoData(fmt.Sprintf("ignoring status update message id: %s, not found", s.MessageID)))
			continue
		}

		if err != nil {
			return nil, err
		}
		data = append(data, courier.NewStatusData(status))
		statuses = append(statuses, status)
	}

	return statuses, courier.WriteDataResponse(ctx, w, http.StatusOK, "statuses handled", data)
}

var infobipStatusMapping = map[string]courier.MsgStatusValue{
	"PENDING":       courier.MsgSent,
	"EXPIRED":       courier.MsgSent,
	"DELIVERED":     courier.MsgDelivered,
	"REJECTED":      courier.MsgFailed,
	"UNDELIVERABLE": courier.MsgFailed,
}

type ibStatusEnvelope struct {
	Results []ibStatus `validate:"required" json:"results"`
}
type ibStatus struct {
	MessageID string `validate:"required" json:"messageId"`
	Status    struct {
		GroupName string `validate:"required" json:"groupName"`
	} `validate:"required" json:"status"`
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *handler) ReceiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	ie := &infobipEnvelope{}
	err := handlers.DecodeAndValidateJSON(ie, r)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	if ie.MessageCount == 0 {
		return nil, courier.WriteAndLogRequestIgnored(ctx, w, r, channel, "ignoring request, no message")
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
				return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
			}
		}

		// create our URN
		urn := urns.NewTelURNForCountry(infobipMessage.From, channel.Country())

		// build our infobipMessage
		msg := h.Backend().NewIncomingMsg(channel, urn, text).WithReceivedOn(date).WithExternalID(messageID)

		// and write it
		err = h.Backend().WriteMsg(ctx, msg)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, msg)

	}

	if len(msgs) == 0 {
		return nil, courier.WriteAndLogRequestIgnored(ctx, w, r, channel, "ignoring request, no message")
	}

	return []courier.Event{msgs[0]}, courier.WriteMsgSuccess(ctx, w, r, msgs)
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
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {

	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for IB channel")
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for IB channel")
	}

	callbackDomain := msg.Channel().CallbackDomain(h.Server().Config().Domain)
	statusURL := fmt.Sprintf("https://%s%s%s/delivered", callbackDomain, "/c/ib/", msg.Channel().UUID())

	ibMsg := ibOutgoingEnvelope{
		Messages: []ibOutgoingMessage{
			ibOutgoingMessage{
				From: msg.Channel().Address(),
				Destinations: []ibDestination{
					ibDestination{
						To:        strings.TrimLeft(msg.URN().Path(), "+"),
						MessageID: msg.ID().String(),
					},
				},
				Text:               handlers.GetTextAndAttachments(msg),
				NotifyContentType:  "application/json",
				IntermediateReport: true,
				NotifyURL:          statusURL,
			},
		},
	}

	requestBody := &bytes.Buffer{}
	err := json.NewEncoder(requestBody).Encode(ibMsg)
	if err != nil {
		return nil, err
	}

	// build our request
	req, err := http.NewRequest(http.MethodPost, sendURL, requestBody)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(username, password)
	rr, err := utils.MakeHTTPRequest(req)

	// record our status and log
	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr)
	status.AddLog(log)
	if err != nil {
		log.WithError("Message Send Error", err)
		return status, nil
	}

	groupID, err := jsonparser.GetInt(rr.Body, "messages", "[0]", "status", "groupId")
	if err != nil || (groupID != 1 && groupID != 3) {
		log.WithError("Message Send Error", errors.Errorf("received error status: '%d'", groupID))
		return status, nil
	}

	externalID, err := jsonparser.GetString(rr.Body, "messages", "[0]", "messageId")
	if externalID != "" {
		status.SetExternalID(externalID)
	}

	status.SetStatus(courier.MsgWired)
	return status, nil
}

// {
// 	"bulkId":"BULK-ID-123-xyz",
// 	"messages":[
// 	  {
// 		"from":"InfoSMS",
// 		"destinations":[
// 		  {
// 			"to":"41793026727",
// 			"messageId":"MESSAGE-ID-123-xyz"
// 		  },
// 		  {
// 			"to":"41793026731"
// 		  }
// 		],
// 		"text":"Artık Ulusal Dil Tanımlayıcısı ile Türkçe karakterli smslerinizi rahatlıkla iletebilirsiniz.",
// 		"flash":false,
// 		"language":{
// 		  "languageCode":"TR"
// 		},
// 		"transliteration":"TURKISH",
// 		"intermediateReport":true,
// 		"notifyUrl":"http://www.example.com/sms/advanced",
// 		"notifyContentType":"application/json",
// 		"callbackData":"DLR callback data",
// 		"validityPeriod": 720
// 	  }
// 	]
// }
//
// API docs from https://dev.infobip.com/docs/fully-featured-textual-message

type ibOutgoingEnvelope struct {
	Messages []ibOutgoingMessage `json:"messages"`
}

type ibOutgoingMessage struct {
	From               string          `json:"from"`
	Destinations       []ibDestination `json:"destinations"`
	Text               string          `json:"text"`
	NotifyContentType  string          `json:"notifyContentType"`
	IntermediateReport bool            `json:"intermediateReport"`
	NotifyURL          string          `json:"notifyUrl"`
}

type ibDestination struct {
	To        string `json:"to"`
	MessageID string `json:"messageId"`
}
