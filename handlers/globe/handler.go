package globe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/gocommon/urns"
)

var (
	maxMsgLength = 160
	sendURL      = "https://devapi.globelabs.com.ph/smsmessaging/v1/outbound/%s/requests"
)

const (
	configPassphrase = "passphrase"
	configAppSecret  = "app_secret"
	configAppID      = "app_id"
)

func init() {
	channels.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() channels.Handler {
	return &handler{handlers.NewBaseHandler(models.ChannelType("GL"), "Globe Labs", handlers.WithRedactConfigKeys(configPassphrase, configAppSecret))}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(r *channels.Routes) error {
	r.Add(h, http.MethodPost, "receive", models.ChannelLogTypeMsgReceive, handlers.JSONPayload(h, h.receiveMessage))
	return nil
}

//	{
//		"inboundSMSMessageList":{
//			"inboundSMSMessage":[
//			   {
//				  "dateTime":"Fri Nov 22 2013 12:12:13 GMT+0000 (UTC)",
//				  "destinationAddress":"tel:21581234",
//				  "messageId":null,
//				  "message":"Hello",
//				  "resourceURL":null,
//				  "senderAddress":"tel:+639171234567"
//			   }
//			 ],
//			 "numberOfMessagesInThisBatch":1,
//			 "resourceURL":null,
//			 "totalNumberOfPendingMessages":null
//		 }
//	}
type moPayload struct {
	InboundSMSMessageList struct {
		InboundSMSMessage []struct {
			DateTime           string `json:"dateTime"`
			DestinationAddress string `json:"destinationAddress"`
			MessageID          string `json:"messageId"`
			Message            string `json:"message"`
			SenderAddress      string `json:"senderAddress"`
		} `json:"inboundSMSMessage"`
	} `json:"inboundSMSMessageList"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, c *models.Channel, w http.ResponseWriter, r *http.Request, payload *moPayload, clog *models.ChannelLog) ([]channels.Event, error) {
	if len(payload.InboundSMSMessageList.InboundSMSMessage) == 0 {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, c, w, r, "no messages, ignored")
	}

	msgs := make([]*models.MsgIn, 0, 1)

	// parse each inbound message
	for _, glMsg := range payload.InboundSMSMessageList.InboundSMSMessage {
		// parse our date from format: "Fri Nov 22 2013 12:12:13 GMT+0000 (UTC)"
		date, err := time.Parse("Mon Jan 2 2006 15:04:05 GMT+0000 (UTC)", glMsg.DateTime)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
		}

		if !strings.HasPrefix(glMsg.SenderAddress, "tel:") {
			return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, fmt.Errorf("invalid 'senderAddress' parameter"))
		}

		urn, err := urns.ParsePhone(glMsg.SenderAddress[4:], c.Country(), true, false)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
		}

		msg := models.NewIncomingMsg(c, urn, glMsg.Message, glMsg.MessageID, clog).WithReceivedOn(date)
		msgs = append(msgs, msg)
	}

	return handlers.WriteMsgsAndResponse(ctx, h, msgs, w, r, clog)
}

//	{
//		  "address": "250788383383",
//	   "message": "hello world",
//	   "passphrase": "my passphrase",
//	   "app_id": "my app id",
//	   "app_secret": "my app secret"
//	}
type mtPayload struct {
	Address    string `json:"address"`
	Message    string `json:"message"`
	Passphrase string `json:"passphrase"`
	AppID      string `json:"app_id"`
	AppSecret  string `json:"app_secret"`
}

func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	appID := msg.Channel().StringConfigForKey(configAppID, "")
	appSecret := msg.Channel().StringConfigForKey(configAppSecret, "")
	passphrase := msg.Channel().StringConfigForKey(configPassphrase, "")

	if appID == "" || appSecret == "" || passphrase == "" {
		return channels.ErrChannelConfig
	}

	parts := handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength)
	for _, part := range parts {
		payload := &mtPayload{}
		payload.Address = strings.TrimPrefix(msg.URN().Path(), "+")
		payload.Message = part
		payload.Passphrase = passphrase
		payload.AppID = appID
		payload.AppSecret = appSecret

		requestBody := &bytes.Buffer{}
		json.NewEncoder(requestBody).Encode(payload)

		// build our request
		req, err := http.NewRequest(http.MethodPost, fmt.Sprintf(sendURL, msg.Channel().Address()), requestBody)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, _, err := h.RequestHTTP(req, clog)
		if err := handlers.ErrorFromResponse(resp, err); err != nil {
			return err
		}
	}
	return nil
}
