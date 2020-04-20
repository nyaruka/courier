package clickatell

import (
	"bytes"
	"context"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
)

var (
	maxMsgLength = 640
	sendURL      = "https://platform.clickatell.com/messages/http/send"
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("CT"), "Clickatell")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodPost, "status", h.receiveStatus)
	return nil
}

type statusPayload struct {
	MessageID  string `name:"messageId"`
	StatusCode int    `name:"statusCode"`
}

var statusMapping = map[int]courier.MsgStatusValue{
	1:  courier.MsgFailed, // incorrect msg id
	2:  courier.MsgWired,  // queued
	3:  courier.MsgSent,   // delivered to upstream gateway
	4:  courier.MsgSent,   // delivered to upstream gateway
	5:  courier.MsgFailed, // error in message
	6:  courier.MsgFailed, // terminated by user
	7:  courier.MsgFailed, // error delivering
	8:  courier.MsgWired,  // msg received
	9:  courier.MsgFailed, // error routing
	10: courier.MsgFailed, // expired
	11: courier.MsgWired,  // delayed but queued
	12: courier.MsgFailed, // out of credit
	14: courier.MsgFailed, // too long
}

// receiveStatus is our HTTP handler function for status updates
func (h *handler) receiveStatus(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	payload := &statusPayload{}
	err := handlers.DecodeAndValidateJSON(payload, r)

	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	if payload.MessageID == "" || payload.StatusCode == 0 {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r,
			fmt.Errorf("missing one of 'messageId' or 'statusCode' in request parameters"))
	}

	msgStatus, found := statusMapping[payload.StatusCode]
	if !found {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r,
			fmt.Errorf("unknown status '%d', must be one of 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 14", payload.StatusCode))
	}

	// write our status
	status := h.Backend().NewMsgStatusForExternalID(channel, payload.MessageID, msgStatus)
	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

type moPayload struct {
	MessageID  string `name:"messageId"`
	FromNumber string `name:"fromNumber"`
	ToNumber   string `name:"toNumber"`
	Timestamp  int64  `name:"timestamp"`
	Text       string `name:"text"`
	Charset    string `name:"charset"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	payload := &moPayload{}
	err := handlers.DecodeAndValidateJSON(payload, r)

	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	if payload.FromNumber == "" || payload.MessageID == "" || payload.Text == "" || payload.Timestamp == 0 {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r,
			fmt.Errorf("missing one of 'messageId', 'fromNumber', 'text' or 'timestamp' in request body"))
	}

	date := time.Unix(0, payload.Timestamp*1000000)

	text := payload.Text
	if payload.Charset == "UTF-16BE" {
		// unescape the JSON
		text, _ = url.QueryUnescape(text)

		// then decode from UTF16
		textBytes := []byte{}
		for _, textByte := range text {
			textBytes = append(textBytes, byte(textByte))
		}
		text, _ = decodeUTF16BE(textBytes)
	}

	// clickatell URL encodes escapes ISO 8859 escape sequences
	if payload.Charset == "ISO-8859-1" {
		// unescape the JSON
		text, _ = url.QueryUnescape(text)

		// then decode from 8859
		text = mime.BEncoding.Encode("ISO-8859-1", text)
		text, _ = new(mime.WordDecoder).DecodeHeader(text)
	}

	// create our URN
	urn, err := handlers.StrictTelForCountry(payload.FromNumber, channel.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}
	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, utils.CleanString(text)).WithReceivedOn(date.UTC()).WithExternalID(payload.MessageID)

	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r)
}

// utility method to decode crazy clickatell 16 bit format
func decodeUTF16BE(b []byte) (string, error) {
	if len(b)%2 != 0 {
		return "", fmt.Errorf("byte slice must be of even length: %v", b)
	}
	u16s := make([]uint16, 1)
	ret := &bytes.Buffer{}
	b8buf := make([]byte, 4)

	lb := len(b)
	for i := 0; i < lb; i += 2 {
		u16s[0] = uint16(b[i+1]) + (uint16(b[i]) << 8)
		r := utf16.Decode(u16s)
		n := utf8.EncodeRune(b8buf, r[0])
		ret.Write(b8buf[:n])
	}
	return ret.String(), nil
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	apiKey := msg.Channel().StringConfigForKey(courier.ConfigAPIKey, "")
	if apiKey == "" {
		return nil, fmt.Errorf("no api_key set for CT channel")
	}

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	parts := handlers.SplitMsg(handlers.GetTextAndAttachments(msg), maxMsgLength)
	for _, part := range parts {
		form := url.Values{
			"apiKey":  []string{apiKey},
			"from":    []string{strings.TrimPrefix(msg.Channel().Address(), "+")},
			"to":      []string{strings.TrimPrefix(msg.URN().Path(), "+")},
			"content": []string{part},
		}

		partSendURL, _ := url.Parse(sendURL)
		partSendURL.RawQuery = form.Encode()

		req, _ := http.NewRequest(http.MethodGet, partSendURL.String(), nil)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		rr, err := utils.MakeHTTPRequest(req)

		// record our status and log
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Send Error", err)
		status.AddLog(log)
		if err != nil {
			return status, nil
		}

		// try to read out our message id, if we can't then this was a failure
		externalID, err := jsonparser.GetString(rr.Body, "messages", "[0]", "apiMessageId")
		if err != nil {
			log.WithError("Send Error", err)
		} else {
			status.SetStatus(courier.MsgWired)
			status.SetExternalID(externalID)
		}
	}

	return status, nil
}
