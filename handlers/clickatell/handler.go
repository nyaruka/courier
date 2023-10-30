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
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, handlers.JSONPayload(h, h.receiveMessage))
	s.AddHandlerRoute(h, http.MethodPost, "status", courier.ChannelLogTypeMsgStatus, handlers.JSONPayload(h, h.receiveStatus))
	return nil
}

type statusPayload struct {
	MessageID  string `name:"messageId"`
	StatusCode int    `name:"statusCode"`
}

var statusMapping = map[int]courier.MsgStatus{
	1:  courier.MsgStatusFailed, // incorrect msg id
	2:  courier.MsgStatusWired,  // queued
	3:  courier.MsgStatusSent,   // delivered to upstream gateway
	4:  courier.MsgStatusSent,   // delivered to upstream gateway
	5:  courier.MsgStatusFailed, // error in message
	6:  courier.MsgStatusFailed, // terminated by user
	7:  courier.MsgStatusFailed, // error delivering
	8:  courier.MsgStatusWired,  // msg received
	9:  courier.MsgStatusFailed, // error routing
	10: courier.MsgStatusFailed, // expired
	11: courier.MsgStatusWired,  // delayed but queued
	12: courier.MsgStatusFailed, // out of credit
	14: courier.MsgStatusFailed, // too long
}

// receiveStatus is our HTTP handler function for status updates
func (h *handler) receiveStatus(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *statusPayload, clog *courier.ChannelLog) ([]courier.Event, error) {
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
	status := h.Backend().NewStatusUpdateByExternalID(channel, payload.MessageID, msgStatus, clog)
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
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *moPayload, clog *courier.ChannelLog) ([]courier.Event, error) {
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
	msg := h.Backend().NewIncomingMsg(channel, urn, text, payload.MessageID, clog).WithReceivedOn(date.UTC())

	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
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

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	apiKey := msg.Channel().StringConfigForKey(courier.ConfigAPIKey, "")
	if apiKey == "" {
		return nil, fmt.Errorf("no api_key set for CT channel")
	}

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)
	parts := handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength)
	for _, part := range parts {
		form := url.Values{
			"apiKey":  []string{apiKey},
			"from":    []string{strings.TrimPrefix(msg.Channel().Address(), "+")},
			"to":      []string{strings.TrimPrefix(msg.URN().Path(), "+")},
			"content": []string{part},
		}

		partSendURL, _ := url.Parse(sendURL)
		partSendURL.RawQuery = form.Encode()

		req, err := http.NewRequest(http.MethodGet, partSendURL.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 != 2 {
			return status, nil
		}

		// try to read out our message id, if we can't then this was a failure
		externalID, err := jsonparser.GetString(respBody, "messages", "[0]", "apiMessageId")
		if err != nil {
			clog.Error(courier.ErrorResponseValueMissing("apiMessageId"))
		} else {
			status.SetStatus(courier.MsgStatusWired)
			status.SetExternalID(externalID)
		}
	}

	return status, nil
}
