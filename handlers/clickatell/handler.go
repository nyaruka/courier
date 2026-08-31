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
	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/gocommon/urns"
)

var (
	maxMsgLength = 640
	sendURL      = "https://platform.clickatell.com/messages/http/send"
)

func init() {
	channels.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() channels.Handler {
	return &handler{handlers.NewBaseHandler(models.ChannelType("CT"), "Clickatell")}
}

// Initialize registers the routes this handler serves
func (h *handler) Initialize(r *channels.Routes) error {
	r.AddReceive(h, http.MethodPost, "receive", channels.ReceiveKindMsg, handlers.JSONPayload(h.receiveMessage))
	r.AddReceive(h, http.MethodPost, "status", channels.ReceiveKindStatus, handlers.JSONPayload(h.receiveStatus))
	return nil
}

type statusPayload struct {
	MessageID  string `json:"messageId"`
	StatusCode int    `json:"statusCode"`
}

var statusMapping = map[int]models.MsgStatus{
	1:  models.MsgStatusFailed, // incorrect msg id
	2:  models.MsgStatusWired,  // queued
	3:  models.MsgStatusSent,   // delivered to upstream gateway
	4:  models.MsgStatusSent,   // delivered to upstream gateway
	5:  models.MsgStatusFailed, // error in message
	6:  models.MsgStatusFailed, // terminated by user
	7:  models.MsgStatusFailed, // error delivering
	8:  models.MsgStatusWired,  // msg received
	9:  models.MsgStatusFailed, // error routing
	10: models.MsgStatusFailed, // expired
	11: models.MsgStatusWired,  // delayed but queued
	12: models.MsgStatusFailed, // out of credit
	14: models.MsgStatusFailed, // too long
}

// receiveStatus is our receive function for status updates
func (h *handler) receiveStatus(ctx context.Context, channel *models.Channel, r *http.Request, payload *statusPayload, in *channels.Received, clog *models.ChannelLog) error {
	if payload.MessageID == "" || payload.StatusCode == 0 {
		return fmt.Errorf("missing one of 'messageId' or 'statusCode' in request parameters")
	}

	msgStatus, found := statusMapping[payload.StatusCode]
	if !found {
		return handlers.UnknownStatusError(statusMapping, payload.StatusCode)
	}

	status := models.NewStatusUpdateByExternalID(channel, payload.MessageID, msgStatus, clog)
	in.Status(status)
	return nil
}

type moPayload struct {
	MessageID  string `json:"messageId"`
	FromNumber string `json:"fromNumber"`
	ToNumber   string `json:"toNumber"`
	Timestamp  int64  `json:"timestamp"`
	Text       string `json:"text"`
	Charset    string `json:"charset"`
}

// receiveMessage is our receive function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel *models.Channel, r *http.Request, payload *moPayload, in *channels.Received, clog *models.ChannelLog) error {
	if payload.FromNumber == "" || payload.MessageID == "" || payload.Text == "" || payload.Timestamp == 0 {
		return fmt.Errorf("missing one of 'messageId', 'fromNumber', 'text' or 'timestamp' in request body")
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
	urn, err := urns.ParsePhone(payload.FromNumber, channel.Country(), true, false)
	if err != nil {
		return err
	}
	// build our msg
	msg := models.NewIncomingMsg(channel, urn, text, payload.MessageID, clog).WithReceivedOn(date.UTC())

	in.Msg(msg)
	return nil
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

func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	apiKey := msg.Channel().StringConfigForKey(models.ConfigAPIKey, "")
	if apiKey == "" {
		return channels.ErrChannelConfig
	}

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

		req, _ := http.NewRequest(http.MethodGet, partSendURL.String(), nil)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err := handlers.ErrorFromResponse(resp, err); err != nil {
			return err
		}

		// try to read out our message id, if we can't then this was a failure
		externalID, _ := jsonparser.GetString(respBody, "messages", "[0]", "apiMessageId")
		if externalID != "" {
			res.AddExternalID(externalID)
		} else {
			return channels.ErrResponseContent
		}
	}

	return nil
}
