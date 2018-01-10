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
	courier.RegisterHandler(NewHandler())
}

type handler struct {
	handlers.BaseHandler
}

// NewHandler returns a new WhatsApp ready to be registered
func NewHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("WA"), "WhatsApp")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	err := s.AddHandlerRoute(h, http.MethodPost, "receive", h.ReceiveMessage)
	if err != nil {
		return err
	}
	return s.AddHandlerRoute(h, http.MethodPost, "status", h.UpdateStatus)
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
type waReceive struct {
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

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *handler) ReceiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	waReceive := &waReceive{}
	err := handlers.DecodeAndValidateJSON(waReceive, r)
	if err != nil {
		return nil, courier.WriteError(ctx, w, r, err)
	}

	// if this is an error, that's an erro
	if waReceive.Error {
		return nil, courier.WriteError(ctx, w, r, fmt.Errorf("received errored message"))
	}

	// create our date from the timestamp
	ts, err := strconv.ParseInt(waReceive.Payload.Timestamp, 10, 64)
	if err != nil {
		return nil, courier.WriteError(ctx, w, r, fmt.Errorf("invalid timestamp: %s", waReceive.Payload.Timestamp))
	}
	date := time.Unix(ts, 0).UTC()

	// create our URN
	urn, err := urns.NewWhatsAppURN(waReceive.Payload.From)
	if err != nil {
		return nil, courier.WriteError(ctx, w, r, err)
	}

	// ignore this if there's no message
	if waReceive.Payload.Message.Text == "" {
		return nil, courier.WriteAndLogRequestIgnored(ctx, w, r, channel, "ignoring, empty message")
	}

	// TODO: should we be hitting the API to look up contact information?
	// TODO: deal with media messages
	// TODO: deal with location messages

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, waReceive.Payload.Message.Text).WithReceivedOn(date).WithExternalID(waReceive.Payload.MessageID)

	// queue our message
	err = h.Backend().WriteMsg(ctx, msg)
	if err != nil {
		return nil, err
	}

	return []courier.Event{msg}, courier.WriteMsgSuccess(ctx, w, r, []courier.Msg{msg})
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
type waStatus struct {
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

// UpdateStatus is our HTTP handler function for status updates
func (h *handler) UpdateStatus(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	// get our params
	waStatus := &waStatus{}
	err := handlers.DecodeAndValidateJSON(waStatus, r)
	if err != nil {
		return nil, courier.WriteError(ctx, w, r, err)
	}

	msgStatus, found := waStatusMapping[waStatus.Payload.MessageStatus]
	if !found {
		return nil, courier.WriteError(
			ctx, w, r,
			fmt.Errorf("unknown status '%s', must be one of 'sending', 'sent', 'delivered', 'read' or 'failed'", waStatus.Payload.MessageStatus))
	}

	// if we have no status, then build it from the external (twilio) id
	status := h.Backend().NewMsgStatusForExternalID(channel, waStatus.Payload.MessageID, msgStatus)

	// write our status
	err = h.Backend().WriteMsgStatus(ctx, status)

	// we can receive read statuses for msgs we didn't trigger
	if err == courier.ErrMsgNotFound {
		return nil, courier.WriteAndLogStatusMsgNotFound(ctx, w, r, channel)
	}

	if err != nil {
		return nil, err
	}

	return []courier.Event{status}, courier.WriteStatusSuccess(ctx, w, r, []courier.MsgStatus{status})
}

// {
//   "payload": {
//     "to": "16315555555",
//     "body": "hello world"
//   }
// }
type waPayload struct {
	To   string `json:"to"    validate:"required"`
	Body string `json:"body"  validate:"required"`
}

// {
//   "payload": {
//     "message_id": "1015511",
//   },
//	 "error": false
// }
type waSendResponse struct {
	Payload struct {
		MessageID string `json:"message_id"  validate:"required"`
	} `json:"payload"`
	Error bool `json:"error"`
}

// whatsapp only allows messages up to 4096 chars
const maxMsgLength = 4096

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	// get our username and password
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for WA channel")
	}
	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for WA channel")
	}
	urlStr := msg.Channel().StringConfigForKey(courier.ConfigBaseURL, "")
	if urlStr == "" {
		return nil, fmt.Errorf("no base url set for WA channel")
	}

	url, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid base url set for WA channel: %s", err)
	}
	sendPath, _ := url.Parse("/api/rest_send.php")
	sendURL := url.ResolveReference(sendPath).String()

	// TODO: figure out sending media

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	parts := handlers.SplitMsg(msg.Text(), maxMsgLength)
	for i, part := range parts {
		payload := waPayload{
			To:   msg.URN().Path(),
			Body: part,
		}

		body := map[string]interface{}{"payload": payload}
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return status, err
		}

		req, err := http.NewRequest(http.MethodPost, sendURL, bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.SetBasicAuth(username, password)
		rr, err := utils.MakeHTTPRequest(req)

		// record our status and log
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
		status.AddLog(log)
		if err != nil {
			return status, nil
		}

		// was this an error?
		wasError, err := jsonparser.GetBoolean([]byte(rr.Body), "error")
		if err != nil || wasError {
			log.WithError("Message Send Error", errors.Errorf("received error from send endpoint"))
			return status, nil
		}

		// grab the id
		externalID, err := jsonparser.GetString([]byte(rr.Body), "payload", "message_id")
		if err != nil {
			log.WithError("Message Send Error", errors.Errorf("unable to get message_id from body"))
			return status, nil
		}

		// if this is our first message, record the external id
		if i == 0 {
			status.SetExternalID(externalID)
		}

		// this was wired successfully
		status.SetStatus(courier.MsgWired)
	}

	return status, nil
}
