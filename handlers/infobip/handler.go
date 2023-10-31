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
	"github.com/nyaruka/gocommon/httpx"
)

var sendURL = "https://api.infobip.com/sms/1/text/advanced"

const configTransliteration = "transliteration"

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
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, handlers.JSONPayload(h, h.receiveMessage))
	s.AddHandlerRoute(h, http.MethodPost, "delivered", courier.ChannelLogTypeMsgStatus, handlers.JSONPayload(h, h.statusMessage))
	return nil
}

var statusMapping = map[string]courier.MsgStatus{
	"PENDING":       courier.MsgStatusSent,
	"EXPIRED":       courier.MsgStatusSent,
	"DELIVERED":     courier.MsgStatusDelivered,
	"REJECTED":      courier.MsgStatusFailed,
	"UNDELIVERABLE": courier.MsgStatusFailed,
}

type statusPayload struct {
	Results []ibStatus `validate:"required" json:"results"`
}
type ibStatus struct {
	MessageID string `validate:"required" json:"messageId"`
	Status    struct {
		GroupName string `validate:"required" json:"groupName"`
	} `validate:"required" json:"status"`
}

// statusMessage is our HTTP handler function for status updates
func (h *handler) statusMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *statusPayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	data := make([]any, len(payload.Results))
	statuses := make([]courier.Event, len(payload.Results))
	for _, s := range payload.Results {
		msgStatus, found := statusMapping[s.Status.GroupName]
		if !found {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unknown status '%s', must be one of PENDING, DELIVERED, EXPIRED, REJECTED or UNDELIVERABLE", s.Status.GroupName))
		}

		// write our status
		status := h.Backend().NewStatusUpdateByExternalID(channel, s.MessageID, msgStatus, clog)
		err := h.Backend().WriteStatusUpdate(ctx, status)
		if err != nil {
			return nil, err
		}
		data = append(data, courier.NewStatusData(status))
		statuses = append(statuses, status)
	}

	return statuses, courier.WriteDataResponse(w, http.StatusOK, "statuses handled", data)
}

//	{
//		"results": [
//		  {
//			"messageId": "817790313235066447",
//			"from": "385916242493",
//			"to": "385921004026",
//			"text": "QUIZ Correct answer is Paris",
//			"cleanText": "Correct answer is Paris",
//			"keyword": "QUIZ",
//			"receivedAt": "2016-10-06T09:28:39.220+0000",
//			"smsCount": 1,
//			"price": {
//			  "pricePerMessage": 0,
//			  "currency": "EUR"
//			},
//			"callbackData": "callbackData"
//		  }
//		],
//		"messageCount": 1,
//		"pendingMessageCount": 0
//	}
type moPayload struct {
	PendingMessageCount int         `json:"pendingMessageCount"`
	MessageCount        int         `json:"messageCount"`
	Results             []moMessage `validate:"required" json:"results"`
}

type moMessage struct {
	MessageID  string `json:"messageId"`
	From       string `json:"from" validate:"required"`
	Text       string `json:"text"`
	ReceivedAt string `json:"receivedAt"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *moPayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	if payload.MessageCount == 0 {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring request, no message")
	}

	msgs := []courier.MsgIn{}
	for _, infobipMessage := range payload.Results {
		messageID := infobipMessage.MessageID
		text := infobipMessage.Text
		dateString := infobipMessage.ReceivedAt

		if text == "" {
			continue
		}

		date := time.Now()
		var err error
		if dateString != "" {
			date, err = time.Parse("2006-01-02T15:04:05.999999999-0700", dateString)
			if err != nil {
				return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
			}
		}

		// create our URN
		urn, err := handlers.StrictTelForCountry(infobipMessage.From, channel.Country())
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}

		// build our infobipMessage
		msg := h.Backend().NewIncomingMsg(channel, urn, text, messageID, clog).WithReceivedOn(date)
		msgs = append(msgs, msg)

	}

	if len(msgs) == 0 {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring request, no message")
	}

	return handlers.WriteMsgsAndResponse(ctx, h, msgs, w, r, clog)
}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for IB channel")
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for IB channel")
	}

	transliteration := msg.Channel().StringConfigForKey(configTransliteration, "")

	callbackDomain := msg.Channel().CallbackDomain(h.Server().Config().Domain)
	statusURL := fmt.Sprintf("https://%s%s%s/delivered", callbackDomain, "/c/ib/", msg.Channel().UUID())

	ibMsg := mtPayload{
		Messages: []mtMessage{
			{
				From: msg.Channel().Address(),
				Destinations: []mtDestination{
					{
						To:        strings.TrimLeft(msg.URN().Path(), "+"),
						MessageID: msg.ID().String(),
					},
				},
				Text:               handlers.GetTextAndAttachments(msg),
				NotifyContentType:  "application/json",
				IntermediateReport: true,
				NotifyURL:          statusURL,
				Transliteration:    transliteration,
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
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(username, password)

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return status, nil
	}

	groupID, err := jsonparser.GetInt(respBody, "messages", "[0]", "status", "groupId")
	if err != nil || (groupID != 1 && groupID != 3) {
		clog.Error(courier.ErrorResponseValueUnexpected("groupId", "1", "3"))
		return status, nil
	}

	externalID, _ := jsonparser.GetString(respBody, "messages", "[0]", "messageId")
	if externalID != "" {
		status.SetExternalID(externalID)
	}

	status.SetStatus(courier.MsgStatusWired)
	return status, nil
}

func (h *handler) RedactValues(ch courier.Channel) []string {
	return []string{
		httpx.BasicAuth(ch.StringConfigForKey(courier.ConfigUsername, ""), ch.StringConfigForKey(courier.ConfigPassword, "")),
	}
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

type mtPayload struct {
	Messages []mtMessage `json:"messages"`
}

type mtMessage struct {
	From               string          `json:"from"`
	Destinations       []mtDestination `json:"destinations"`
	Text               string          `json:"text"`
	NotifyContentType  string          `json:"notifyContentType"`
	IntermediateReport bool            `json:"intermediateReport"`
	NotifyURL          string          `json:"notifyUrl"`
	Transliteration    string          `json:"transliteration,omitempty"`
}

type mtDestination struct {
	To        string `json:"to"`
	MessageID string `json:"messageId"`
}
