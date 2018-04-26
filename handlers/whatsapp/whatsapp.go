package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
	"github.com/pkg/errors"
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("WA"), "WhatsApp")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodPost, "status", h.receiveStatus)
	return nil
}

// {
//	 "meta": null,
//	 "payload": {
//	   "from": "16315555555",
//	   "message_id": "345b5e14775782",
//	   "timestamp": "1476225801",
//	   "message": {
//		"address": "1 Hacker Way, Menlo Park, CA 94025",
//		"latitude": 37.483253479003906,
//		"longitude": -122.14960479736328,
// 		"has_media": true,
// 		"text": "This is the media caption.",
//		"type": "image"
// 	   }
//	 },
//	 "error": false
// }
type moPayload struct {
	Payload struct {
		From      string `json:"from" validate:"required"`
		MessageID string `json:"message_id" validate:"required"`
		Timestamp string `json:"timestamp" validate:"required"`
		Message   struct {
			HasMedia  bool    `json:"has_media"`
			Address   string  `json:"address"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
			Text      string  `json:"text"`
			Type      string  `json:"type" validate:"required"`
		} `json:"message" validate:"required"`
	} `json:"payload"`
	Error bool `json:"error"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	payload := &moPayload{}
	err := handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// if this is an error, that's an erro
	if payload.Error {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("received errored message"))
	}

	// create our date from the timestamp
	ts, err := strconv.ParseInt(payload.Payload.Timestamp, 10, 64)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("invalid timestamp: %s", payload.Payload.Timestamp))
	}
	date := time.Unix(ts, 0).UTC()

	// create our URN
	urn, err := urns.NewWhatsAppURN(payload.Payload.From)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// TODO: should we be hitting the API to look up contact information?
	// TODO: deal with media messages
	// TODO: deal with location messages

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, payload.Payload.Message.Text).WithReceivedOn(date).WithExternalID(payload.Payload.MessageID)
	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r)
}

// {
//   "meta": null,
//   "payload": {
//     "message_id": "157b5e14568e8",
//     "to": "16315555555",
//     "timestamp": "1476225801",
//     "message_status": "read"
//   },
//   "error": false
// }
type statusPayload struct {
	Payload struct {
		MessageID     string `json:"message_id"      validate:"required"`
		To            string `json:"to"              validate:"required"`
		Timestamp     string `json:"timestamp"       validate:"required"`
		MessageStatus string `json:"message_status"  validate:"required"`
	} `json:"payload"`
}

var waStatusMapping = map[string]courier.MsgStatusValue{
	"sending":   courier.MsgWired,
	"sent":      courier.MsgSent,
	"delivered": courier.MsgDelivered,
	"read":      courier.MsgDelivered,
	"failed":    courier.MsgFailed,
}

// receiveStatus is our HTTP handler function for status updates
func (h *handler) receiveStatus(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	// get our params
	payload := &statusPayload{}
	err := handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	msgStatus, found := waStatusMapping[payload.Payload.MessageStatus]
	if !found {
		return nil, handlers.WriteAndLogRequestError(
			ctx, h, channel, w, r,
			fmt.Errorf("unknown status '%s', must be one of 'sending', 'sent', 'delivered', 'read' or 'failed'", payload.Payload.MessageStatus))
	}

	// if we have no status, then build it from the external (twilio) id
	status := h.Backend().NewMsgStatusForExternalID(channel, payload.Payload.MessageID, msgStatus)

	// write our status
	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

// {
//   "to": "16315555555",
//   "type": "text",
//   "text": {
//     "body": "text message"
//   }
// }
type mtPayload struct {
	To   string `json:"to"    validate:"required"`
	Type string `json:"type"`
	Text struct {
		Body string `json:"body"  validate:"required"`
	} `json:"text"`
}

// whatsapp only allows messages up to 4096 chars
const maxMsgLength = 4096

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	// get our token
	token := msg.Channel().StringConfigForKey(courier.ConfigAuthToken, "")
	if token == "" {
		return nil, fmt.Errorf("missing token for WA channel")
	}

	urlStr := msg.Channel().StringConfigForKey(courier.ConfigBaseURL, "")
	url, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid base url set for WA channel: %s", err)
	}
	sendPath, _ := url.Parse("/v1/messages")
	sendURL := url.ResolveReference(sendPath).String()

	// TODO: figure out sending media

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	parts := handlers.SplitMsg(msg.Text(), maxMsgLength)
	for i, part := range parts {
		payload := mtPayload{
			To:   msg.URN().Path(),
			Type: "text",
		}
		payload.Text.Body = part

		jsonBody, err := json.Marshal(payload)
		if err != nil {
			return status, err
		}

		req, _ := http.NewRequest(http.MethodPost, sendURL, bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
		rr, err := utils.MakeHTTPRequest(req)

		// record our status and log
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
		status.AddLog(log)
		if err != nil {
			return status, nil
		}

		// was this an error?
		errorTitle, _ := jsonparser.GetString([]byte(rr.Body), "errors", "[0]", "title")
		if errorTitle != "" {
			log.WithError("Message Send Error", errors.Errorf("received error from send endpoint"))
			return status, nil
		}

		// grab the id
		externalID, err := jsonparser.GetString([]byte(rr.Body), "messages", "[0]", "id")
		if err != nil {
			log.WithError("Message Send Error", errors.Errorf("unable to get messages.0.id from body"))
			return status, nil
		}

		// if this is our first message, record the external id
		if i == 0 {
			status.SetExternalID(externalID)
		}
	}

	status.SetStatus(courier.MsgWired)
	return status, nil
}
