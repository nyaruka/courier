package playmobile

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
)

const (
	configBaseURL          = "base_url"
	configUsername         = "username"
	configPassword         = "password"
	configIncomingPrefixes = "incoming_prefixes"
)

var (
	maxMsgLength = 640
	sendURL      = "%s/broker-api/send"
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("PM"), "Play Mobile")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
	return nil
}

// {
// 	"messages": [{
// 		"recipient": "999999999999",
// 		"message-id": "2018-10-26-09-27-34",
// 		"sms": {
// 			"originator": "1122",
// 			"content": {
// 				"text": "Hello World. Please send me an email if you received well!"
// 			}
// 		}
// 	}]
// }

type mtPayload struct {
	Messages []mtMessage `json:"messages"`
}

type mtMessage struct {
	Recipient string `json:"recipient"`
	MessageID string `json:"message-id"`
	SMS       struct {
		Originator string `json:"originator"`
		Content    struct {
			Text string `json:"text"`
		} `json:"content"`
	} `json:"sms"`
}

// <sms-request version="1.0">
//     <message id="1107962" msisdn="9989xxxxxxxx" submit-date="2016-11-22 15:10:32">
//         <content type="text/plain">SMS Response</content>
//     </message>
// </sms-request>

type mtResponse struct {
	XMLName xml.Name `xml:"sms-request"`
	Message []struct {
		ID         string `xml:"id,attr"`
		MSIDSN     string `xml:"msisdn,attr"`
		SubmitDate string `xml:"submit-date,attr"`
		Content    struct {
			Text string `xml:",chardata"`
		} `xml:"content"`
	} `xml:"message"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, c courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	payload := &mtResponse{}
	err := handlers.DecodeAndValidateXML(payload, r)

	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
	}

	if len(payload.Message) == 0 {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, c, w, r, "no messages, ignored")
	}

	msgs := make([]courier.MsgIn, 0, 1)

	// parse each inbound message
	for _, pmMsg := range payload.Message {
		if pmMsg.MSIDSN == "" || pmMsg.ID == "" {
			return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, fmt.Errorf("missing required fields msidsn or id"))
		}

		// create our URN
		urn, err := handlers.StrictTelForCountry(pmMsg.MSIDSN, c.Country())
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
		}

		// remove message prefix according to a list of possible prefixes, useful for free accounts
		incomingPrefixes := c.ConfigForKey(configIncomingPrefixes, []string{})
		if prefixes, ok := incomingPrefixes.([]string); ok {
			for _, prefix := range prefixes {
				text := pmMsg.Content.Text

				if strings.HasPrefix(strings.ToLower(text), strings.ToLower(prefix)) {
					text = strings.TrimSpace(text[len(prefix):])
					pmMsg.Content.Text = text
					break
				}
			}
		}

		// build our msg
		if pmMsg.Content.Text == "" {
			return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, errors.New("no text"))
		}
		msg := h.Backend().NewIncomingMsg(c, urn, pmMsg.Content.Text, pmMsg.ID, clog)
		msgs = append(msgs, msg)
	}

	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, msgs, w, r, clog)
}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	username := msg.Channel().StringConfigForKey(configUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for PM channel")
	}

	password := msg.Channel().StringConfigForKey(configPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for PM channel")
	}

	shortCode := msg.Channel().Address()
	if shortCode == "" {
		return nil, fmt.Errorf("no phone sender set for PM channel")
	}

	baseURL := msg.Channel().StringConfigForKey(configBaseURL, "")
	if baseURL == "" {
		return nil, fmt.Errorf("no base url set for PM channel")
	}

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)

	for i, part := range handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength) {
		payload := mtPayload{}
		message := mtMessage{}

		messageid := msg.ID().String()
		if i > 0 {
			messageid = fmt.Sprintf("%s.%d", msg.ID().String(), i+1)
		}
		message.MessageID = messageid
		message.Recipient = strings.TrimLeft(msg.URN().Path(), "+")
		message.SMS.Originator = shortCode
		message.SMS.Content.Text = part

		payload.Messages = append(payload.Messages, message)
		jsonBody := jsonx.MustMarshal(payload)

		req, err := http.NewRequest(http.MethodPost, fmt.Sprintf(sendURL, baseURL), bytes.NewReader(jsonBody))
		if err != nil {
			return nil, err
		}
		req.SetBasicAuth(username, password)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, _, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 != 2 {
			return status, nil
		}

		status.SetStatus(courier.MsgStatusWired)
	}

	return status, nil
}

func (h *handler) RedactValues(ch courier.Channel) []string {
	return []string{
		httpx.BasicAuth(ch.StringConfigForKey(courier.ConfigUsername, ""), ch.StringConfigForKey(courier.ConfigPassword, "")),
	}
}
