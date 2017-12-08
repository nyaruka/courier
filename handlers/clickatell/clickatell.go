package clickatell

import (
	"bytes"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/gsm7"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
	"github.com/pkg/errors"
)

/*
GET /api/v1/clickatell/receive/uuid?api_id=12345&from=263778181111&timestamp=2017-05-03+07%3A30%3A10&text=Msg&charset=ISO-8859-1&udh=&moMsgId=b1e4782a3c87339d706ab1343b4df1ce&to=33500
*/
var maxMsgLength = 420
var sendURL = "https://api.clickatell.com/http/sendmsg"

func init() {
	courier.RegisterHandler(NewHandler())
}

type handler struct {
	handlers.BaseHandler
}

// NewHandler returns a new Infobip handler
func NewHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("CT"), "Clickatell")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	err := s.AddReceiveMsgRoute(h, "GET", "receive", h.ReceiveMessage)
	if err != nil {
		return err
	}
	return nil
}

type clickatellIncomingMsg struct {
	From      string `name:"from"`
	Text      string `name:"text"`
	SmsID     string `name:"moMsgId"`
	Timestamp string `name:"timestamp"`
	APIID     string `name:"api_id"`
	Charset   string `name:"charset"`
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *handler) ReceiveMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.ReceiveEvent, error) {
	ctIncomingMessage := &clickatellIncomingMsg{}
	handlers.DecodeAndValidateQueryParams(ctIncomingMessage, r)

	// if this is a post, also try to parse the form body
	if r.Method == http.MethodPost {
		handlers.DecodeAndValidateForm(ctIncomingMessage, r)
	}

	if ctIncomingMessage.APIID != "" && ctIncomingMessage.APIID != channel.StringConfigForKey(courier.ConfigAPIID, "") {
		return nil, courier.WriteError(w, r, fmt.Errorf("invalid API id for message delivery: %s", ctIncomingMessage.APIID))
	}

	if ctIncomingMessage.From == "" || ctIncomingMessage.SmsID == "" || ctIncomingMessage.Text == "" || ctIncomingMessage.Timestamp == "" {
		return nil, courier.WriteIgnored(w, r, "missing one of 'from', 'text', 'moMsgId' or 'timestamp' in request parameters.")
	}

	dateString := ctIncomingMessage.Timestamp

	date := time.Now()
	var err error
	if dateString != "" {
		loc, _ := time.LoadLocation("Europe/Berlin")
		date, err = time.ParseInLocation("2006-01-02 15:04:05", dateString, loc).UTC()
		if err != nil {
			return nil, courier.WriteError(w, r, errors.New("invalid date format, must be YYYY-MM-DD HH:MM:SS"))
		}
	}

	// create our URN
	urn := urns.NewTelURNForCountry(ctIncomingMessage.From, channel.Country())

	text := ctIncomingMessage.Text
	if ctIncomingMessage.Charset == "UTF-16BE" {
		textBytes := []byte{}
		for _, textByte := range text {
			textBytes = append(textBytes, byte(textByte))
		}
		text, _ = decodeUTF16BE(textBytes)
	}

	if ctIncomingMessage.Charset == "ISO-8859-1" {
		text = mime.BEncoding.Encode("ISO-8859-1", text)
		text, _ = new(mime.WordDecoder).DecodeHeader(text)
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, utils.CleanString(text)).WithReceivedOn(date).WithExternalID(ctIncomingMessage.SmsID)

	// and write it
	err = h.Backend().WriteMsg(msg)
	if err != nil {
		return nil, err
	}

	return []courier.ReceiveEvent{msg}, courier.WriteMsgSuccess(w, r, []courier.Msg{msg})
}

func decodeUTF16BE(b []byte) (string, error) {
	if len(b)%2 != 0 {
		return "", fmt.Errorf("Must have even length byte slice")
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
func (h *handler) SendMsg(msg courier.Msg) (courier.MsgStatus, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for CT channel")
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for CT channel")
	}

	apiID := msg.Channel().StringConfigForKey(courier.ConfigAPIID, "")
	if apiID == "" {
		return nil, fmt.Errorf("no api_id set for CT channel")
	}

	unicodeSwitch := "0"
	text := courier.GetTextAndAttachments(msg)
	if !gsm7.IsGSM7(text) {
		replaced := gsm7.ReplaceNonGSM7Chars(text)
		if gsm7.IsGSM7(replaced) {
			text = replaced
		} else {
			unicodeSwitch = "1"
		}
	}

	re := regexp.MustCompile(`^ID: (.*)`)

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	parts := handlers.SplitMsg(text, maxMsgLength)
	for _, part := range parts {
		form := url.Values{
			"api_id":   []string{apiID},
			"user":     []string{username},
			"password": []string{password},
			"from":     []string{strings.TrimPrefix(msg.Channel().Address(), "+")},
			"concat":   []string{"3"},
			"callback": []string{"7"},
			"mo":       []string{"1"},
			"unicode":  []string{unicodeSwitch},
			"to":       []string{strings.TrimPrefix(msg.URN().Path(), "+")},
			"text":     []string{part},
		}

		encodedForm := form.Encode()
		partSendURL := fmt.Sprintf("%s?%s", sendURL, encodedForm)

		req, err := http.NewRequest(http.MethodGet, partSendURL, nil)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr, err := utils.MakeHTTPRequest(req)

		// record our status and log
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr)
		status.AddLog(log)
		if err != nil {
			log.WithError("Message Send Error", err)
			return status, nil
		}

		if rr.StatusCode != 200 && rr.StatusCode != 201 && rr.StatusCode != 202 {
			return status, errors.Errorf("Got non-200 response [%d] from API", rr.StatusCode)
		}

		status.SetStatus(courier.MsgWired)

		matched := re.FindAllStringSubmatch(string([]byte(rr.Body)), -1)
		if len(matched) > 0 && len(matched[0]) > 0 {
			status.SetExternalID(matched[0][1])
		}

	}

	return status, nil
}
