package playmobile

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
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
	channels.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() channels.Handler {
	return &handler{handlers.NewBaseHandler(models.ChannelType("PM"), "Play Mobile")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(r *channels.Routes) error {
	r.Add(h, http.MethodPost, "receive", models.ChannelLogTypeMsgReceive, h.receiveMessage)
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
func (h *handler) receiveMessage(ctx context.Context, c *models.Channel, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
	payload := &mtResponse{}
	err := handlers.DecodeAndValidateXML(payload, r)

	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
	}

	if len(payload.Message) == 0 {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, c, w, r, "no messages, ignored")
	}

	msgs := make([]*models.MsgIn, 0, 1)

	// parse each inbound message
	for _, pmMsg := range payload.Message {
		if pmMsg.MSIDSN == "" || pmMsg.ID == "" {
			return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, fmt.Errorf("missing required fields msidsn or id"))
		}

		// create our URN
		urn, err := urns.ParsePhone(pmMsg.MSIDSN, c.Country(), true, false)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
		}

		// remove message prefix according to a list of possible prefixes, useful for free accounts. Channel config is
		// JSON so the configured list arrives as []any of strings, not []string.
		prefixes, _ := c.ConfigForKey(configIncomingPrefixes, []any{}).([]any)
		for _, p := range prefixes {
			prefix, ok := p.(string)
			if !ok {
				continue
			}

			text := pmMsg.Content.Text

			if strings.HasPrefix(strings.ToLower(text), strings.ToLower(prefix)) {
				pmMsg.Content.Text = strings.TrimSpace(text[len(prefix):])
				break
			}
		}

		// build our msg
		if pmMsg.Content.Text == "" {
			return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, errors.New("no text"))
		}
		msg := models.NewIncomingMsg(c, urn, pmMsg.Content.Text, pmMsg.ID, clog)
		msgs = append(msgs, msg)
	}

	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, msgs, w, r, clog)
}

func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	username := msg.Channel().StringConfigForKey(configUsername, "")
	password := msg.Channel().StringConfigForKey(configPassword, "")
	shortCode := msg.Channel().Address()
	baseURL := msg.Channel().StringConfigForKey(configBaseURL, "")
	if username == "" || password == "" || shortCode == "" || baseURL == "" {
		return channels.ErrChannelConfig
	}

	for i, part := range handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength) {
		payload := mtPayload{}
		message := mtMessage{}

		messageid := string(msg.UUID())
		if i > 0 {
			messageid = fmt.Sprintf("%s.%d", msg.UUID(), i+1)
		}
		message.MessageID = messageid
		message.Recipient = strings.TrimLeft(msg.URN().Path(), "+")
		message.SMS.Originator = shortCode
		message.SMS.Content.Text = part

		payload.Messages = append(payload.Messages, message)
		jsonBody := jsonx.MustMarshal(payload)

		req, err := http.NewRequest(http.MethodPost, fmt.Sprintf(sendURL, baseURL), bytes.NewReader(jsonBody))
		if err != nil {
			return err
		}
		req.SetBasicAuth(username, password)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, _, err := h.RequestHTTP(req, clog)
		if err := handlers.ErrorFromResponse(resp, err); err != nil {
			return err
		}
	}
	return nil
}

func (h *handler) RedactValues(ch *models.Channel) []string {
	return []string{
		httpx.BasicAuth(ch.StringConfigForKey(models.ConfigUsername, ""), ch.StringConfigForKey(models.ConfigPassword, "")),
	}
}
