package justcall

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/jsonx"
)

var (
	sendURL      = "https://api.justcall.io/v1/texts/new"
	maxMsgLength = 160
)

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("JCL"), "JustCall")}
}

func init() {
	courier.RegisterHandler(newHandler())
}

// Initialize implements courier.ChannelHandler
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, handlers.JSONPayload(h, h.receiveMessage))
	s.AddHandlerRoute(h, http.MethodPost, "status", courier.ChannelLogTypeMsgStatus, handlers.JSONPayload(h, h.statusMessage))
	return nil
}

//	{
//	  "data": {
//	    "type": "sms",
//	    "direction": "0",
//	    "justcall_number": "192XXXXXXXX",
//	    "contact_name": "Sushant Tripathi",
//	    "contact_number": "+91810XXXXXXX",
//	    "contact_email": "customer@gmail.com",
//	    "is_contact": 1,
//	    "content": "Hey !",
//	    "signature": "35e89fc56b497xxxxxxxxxx8f7b27fe49d",
//	    "datetime": "2020-12-03 13:35:13",
//	    "delivery_status": "sent",
//	    "requestid": "1229153",
//	    "messageid": 26523491,
//	    "is_mms": "1",
//	    "mms": [
//	      {
//	        "media_url": "https://www.filepicker.io/api/file/p6j9ExQNWMCCYOQvHI",
//	        "content_type": "image/jpeg"
//	      },
//	      {
//	        "media_url": "https://www.filepicker.io/api/file/axNH43SFm7inN3iKDz",
//	        "content_type": "image/png"
//	      },
//	      {
//	        "media_url": "https://www.filepicker.io/api//file/cN95JZSM2ScSXGamlh",
//	        "content_type": "image/jpeg"
//	      }
//	    ],
//	    "agent_name": "Sales JustCall",
//	    "agent_id": 10636
//	  }
//	}
type moPayload struct {
	Data struct {
		Type      string `json:"type"`
		Direction string `json:"direction"`
		To        string `json:"justcall_number"`
		From      string `json:"contact_number"`
		Name      string `json:"contact_name"`
		Content   string `json:"content"`
		Datetime  string `json:"datetime"`
		Status    string `json:"delivery_status"`
		MessageID int32  `json:"messageid"`
		MMS       []struct {
			MediaURL    string `json:"media_url"`
			ContentType string `json:"content_type"`
		} `json:"mms"`
	} `json:"data"`
}

func (h *handler) receiveMessage(ctx context.Context, c courier.Channel, w http.ResponseWriter, r *http.Request, payload *moPayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	if payload.Data.Type != "sms" || payload.Data.Direction != "I" {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, c, w, r, "Ignoring request, no message")
	}

	dateString := payload.Data.Datetime
	date := time.Now()
	var err error
	if dateString != "" {
		date, err = time.Parse("2006-01-02 15:04:05", dateString)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, errors.New("invalid date format, must be RFC 3339"))
		}
		date = date.UTC()
	}

	urn, err := handlers.StrictTelForCountry(payload.Data.From, c.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(c, urn, payload.Data.Content, fmt.Sprint(payload.Data.MessageID), clog).WithReceivedOn(date)

	if len(payload.Data.MMS) > 0 {
		msg.WithAttachment(payload.Data.MMS[0].MediaURL)
	}

	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
}

var statusMapping = map[string]courier.MsgStatus{
	"delivered":   courier.MsgStatusDelivered,
	"sent":        courier.MsgStatusSent,
	"undelivered": courier.MsgStatusErrored,
	"failed":      courier.MsgStatusFailed,
}

func (h *handler) statusMessage(ctx context.Context, c courier.Channel, w http.ResponseWriter, r *http.Request, payload *moPayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	if payload.Data.Type != "sms" || payload.Data.Direction != "O" {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, c, w, r, "Ignoring request, no message")
	}

	msgStatus, found := statusMapping[payload.Data.Status]
	if !found {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, fmt.Errorf("unknown status '%s', must be one of send, delivered, undelivered, failed", payload.Data.Status))
	}
	// write our status
	status := h.Backend().NewStatusUpdateByExternalID(c, fmt.Sprint(payload.Data.MessageID), msgStatus, clog)
	return handlers.WriteMsgStatusAndResponse(ctx, h, c, status, w, r)
}

type mtPayload struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Body     string `json:"body"`
	MediaURL string `json:"media_url,omitempty"`
}

// Send implements courier.ChannelHandler
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	apiKey := msg.Channel().StringConfigForKey(courier.ConfigAPIKey, "")
	if apiKey == "" {
		return nil, fmt.Errorf("no API key set for JCL channel")
	}

	apiSecret := msg.Channel().StringConfigForKey(courier.ConfigSecret, "")
	if apiSecret == "" {
		return nil, fmt.Errorf("no API secret set for JCL channel")
	}

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)
	mediaURLs := make([]string, 0, 5)
	text := msg.Text()

	if len(msg.Attachments()) <= 5 {
		for _, a := range msg.Attachments() {
			_, url := handlers.SplitAttachment(a)
			mediaURLs = append(mediaURLs, url)
		}
	} else {
		text = handlers.GetTextAndAttachments(msg)
	}

	payload := mtPayload{From: msg.Channel().Address(), To: msg.URN().Path(), Body: text}
	if len(mediaURLs) > 0 {
		payload.MediaURL = strings.Join(mediaURLs, ",")
	}

	req, err := http.NewRequest(http.MethodPost, sendURL, bytes.NewReader(jsonx.MustMarshal(payload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("%s:%s", apiKey, apiSecret))

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return status, nil
	}

	respStatus, err := jsonparser.GetString(respBody, "status")
	if err != nil {
		clog.Error(courier.ErrorResponseValueMissing("status"))
		return status, h.Backend().WriteChannelLog(ctx, clog)
	}
	if respStatus != "success" {
		return status, nil

	}

	externalID, err := jsonparser.GetInt(respBody, "id")
	if err != nil {
		clog.Error(courier.ErrorResponseValueMissing("id"))
		return status, h.Backend().WriteChannelLog(ctx, clog)
	}

	if externalID != 0 {
		status.SetExternalID(fmt.Sprintf("%d", externalID))
	}

	status.SetStatus(courier.MsgStatusWired)
	return status, nil

}
