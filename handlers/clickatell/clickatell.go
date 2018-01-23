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
	"github.com/nyaruka/gocommon/urns"
)

var maxMsgLength = 640
var sendURL = "https://platform.clickatell.com/messages/http/send"

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
	err := s.AddHandlerRoute(h, http.MethodPost, "receive", h.ReceiveMessage)
	if err != nil {
		return err
	}
	return s.AddHandlerRoute(h, http.MethodPost, "status", h.StatusMessage)
}

type statusReport struct {
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

// StatusMessage is our HTTP handler function for status updates
func (h *handler) StatusMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	statusReport := &statusReport{}
	err := handlers.DecodeAndValidateJSON(statusReport, r)

	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	if statusReport.MessageID == "" || statusReport.StatusCode == 0 {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel,
			fmt.Errorf("missing one of 'messageId' or 'statusCode' in request parameters"))
	}

	msgStatus, found := statusMapping[statusReport.StatusCode]
	if !found {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel,
			fmt.Errorf("unknown status '%d', must be one of 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 14", statusReport.StatusCode))
	}

	// write our status
	status := h.Backend().NewMsgStatusForExternalID(channel, statusReport.MessageID, msgStatus)
	err = h.Backend().WriteMsgStatus(ctx, status)
	if err == courier.ErrMsgNotFound {
		return []courier.Event{}, courier.WriteAndLogStatusMsgNotFound(ctx, w, r, channel)
	}
	if err != nil {
		return nil, err
	}

	return []courier.Event{status}, courier.WriteStatusSuccess(ctx, w, r, []courier.MsgStatus{status})
}

type clickatellIncomingMsg struct {
	MessageID  string `name:"messageId"`
	FromNumber string `name:"fromNumber"`
	ToNumber   string `name:"toNumber"`
	Timestamp  int64  `name:"timestamp"`
	Text       string `name:"text"`
	Charset    string `name:"charset"`
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *handler) ReceiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	ctMO := &clickatellIncomingMsg{}
	err := handlers.DecodeAndValidateJSON(ctMO, r)

	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	if ctMO.FromNumber == "" || ctMO.MessageID == "" || ctMO.Text == "" || ctMO.Timestamp == 0 {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel,
			fmt.Errorf("missing one of 'messageId', 'fromNumber', 'text' or 'timestamp' in request body"))
	}

	date := time.Unix(0, ctMO.Timestamp*1000000)

	text := ctMO.Text
	if ctMO.Charset == "UTF-16BE" {
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
	if ctMO.Charset == "ISO-8859-1" {
		// unescape the JSON
		text, _ = url.QueryUnescape(text)

		// then decode from 8859
		text = mime.BEncoding.Encode("ISO-8859-1", text)
		text, _ = new(mime.WordDecoder).DecodeHeader(text)
	}

	// create our URN
	urn := urns.NewTelURNForCountry(ctMO.FromNumber, channel.Country())

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, utils.CleanString(text)).WithReceivedOn(date.UTC()).WithExternalID(ctMO.MessageID)

	// and write it
	err = h.Backend().WriteMsg(ctx, msg)
	if err != nil {
		return nil, err
	}

	return []courier.Event{msg}, courier.WriteMsgSuccess(ctx, w, r, []courier.Msg{msg})
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
	parts := handlers.SplitMsg(courier.GetTextAndAttachments(msg), maxMsgLength)
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
