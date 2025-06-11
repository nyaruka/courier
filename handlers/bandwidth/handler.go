package bandwidth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
)

var (
	maxMsgLength = 2048
	sendURL      = "https://messaging.bandwidth.com/api/v2/users/%s/messages"
)

const (
	configAccountID        = "account_id"
	configMsgApplicationID = "messaging_application_id"

	oldApplicationID = "application_id"
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("BW"), "Bandwidth")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodPost, "status", courier.ChannelLogTypeMsgStatus, h.statusMessage)
	return nil
}

type moMessageData struct {
	Type    string `json:"type" validate:"required"`
	Message struct {
		ID    string   `json:"id" validate:"required"`
		Time  string   `json:"time"`
		From  string   `json:"from"`
		Text  string   `json:"text"`
		Media []string `json:"media"`
	} `json:"message" validate:"required"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	var payload []moMessageData

	body, err := handlers.ReadBody(r, 1000000)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	err = json.Unmarshal(body, &payload)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	if len(payload) == 0 {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "no messages, ignored")
	}

	err = utils.Validate(payload[0])
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	messagePayload := payload[0]

	// create our date from the timestamp
	// 2017-05-03T06:04:45Z
	date, err := time.Parse("2006-01-02T15:04:05Z", messagePayload.Message.Time)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("invalid date format: %s", messagePayload.Message.Time))
	}

	// create our URN
	urn, err := urns.ParsePhone(messagePayload.Message.From, channel.Country(), true, false)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}
	// build our msg
	msg := h.Backend().NewIncomingMsg(ctx, channel, urn, messagePayload.Message.Text, messagePayload.Message.ID, clog).WithReceivedOn(date)

	for _, attURL := range messagePayload.Message.Media {
		msg.WithAttachment(attURL)
	}

	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
}

type moStatusData struct {
	Type        string `json:"type" validate:"required"`
	ErrorCode   int    `json:"errorCode"`
	Description string `json:"description"`
	Message     struct {
		ID string `json:"id" validate:"required"`
	} `json:"message" validate:"required"`
}

var statusMapping = map[string]courier.MsgStatus{
	"message-sending":   courier.MsgStatusSent,
	"message-delivered": courier.MsgStatusDelivered,
	"message-failed":    courier.MsgStatusFailed,
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) statusMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	var payload []moStatusData
	body, err := handlers.ReadBody(r, 1000000)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	err = json.Unmarshal(body, &payload)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	if len(payload) == 0 {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "no messages, ignored")
	}

	err = utils.Validate(payload[0])
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	statusPayload := payload[0]
	msgStatus, found := statusMapping[statusPayload.Type]
	if !found {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r,
			fmt.Errorf("unknown status '%s', must be one of 'message-sending', 'message-delivered' or 'message-failed'", statusPayload.Type))
	}

	if statusPayload.ErrorCode != 0 {
		clog.Error(courier.ErrorExternal(strconv.Itoa(statusPayload.ErrorCode), statusPayload.Description))
	}

	// write our status
	status := h.Backend().NewStatusUpdateByExternalID(channel, statusPayload.Message.ID, msgStatus, clog)
	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

type mtPayload struct {
	ApplicationID string   `json:"applicationId"`
	To            []string `json:"to"`
	From          string   `json:"from"`
	Text          string   `json:"text"`
	Media         []string `json:"media,omitempty"`
}

type mtResponse struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	accountID := msg.Channel().StringConfigForKey(configAccountID, "")
	applicationID := msg.Channel().StringConfigForKey(configMsgApplicationID, "")
	if applicationID == "" {
		applicationID = msg.Channel().StringConfigForKey(oldApplicationID, "")
	}

	if username == "" || password == "" || accountID == "" || applicationID == "" {
		return courier.ErrChannelConfig
	}

	msgParts := make([]string, 0)
	if msg.Text() != "" {
		msgParts = handlers.SplitMsgByChannel(msg.Channel(), msg.Text(), maxMsgLength)
	} else {
		if len(msg.Attachments()) > 0 {
			msgParts = append(msgParts, "")
		}
	}

	for i, part := range msgParts {
		payload := &mtPayload{}
		payload.ApplicationID = applicationID
		payload.To = []string{msg.URN().Path()}
		payload.From = msg.Channel().Address()
		payload.Text = part

		if i == 0 && len(msg.Attachments()) > 0 {
			attachments := make([]string, 0)
			for _, attachment := range msg.Attachments() {
				_, url := handlers.SplitAttachment(attachment)
				attachments = append(attachments, url)
			}
			payload.Media = attachments
		}

		jsonBody := jsonx.MustMarshal(payload)

		// build our request
		req, err := http.NewRequest(http.MethodPost, fmt.Sprintf(sendURL, accountID), bytes.NewReader(jsonBody))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.SetBasicAuth(username, password)

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 == 5 {
			return courier.ErrConnectionFailed
		}

		response := &mtResponse{}
		if err = json.Unmarshal(respBody, response); err != nil {
			return courier.ErrResponseUnparseable
		}

		if resp.StatusCode/100 != 2 {
			return courier.ErrFailedWithReason(response.Type, response.Description)
		}

		if response.ID != "" {
			res.AddExternalID(response.ID)
		}
	}

	return nil
}

// BuildAttachmentRequest to download media for message attachment with Basic auth set
func (h *handler) BuildAttachmentRequest(ctx context.Context, b courier.Backend, channel courier.Channel, attachmentURL string, clog *courier.ChannelLog) (*http.Request, error) {
	username := channel.StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for BW channel")
	}

	password := channel.StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for BW channel")
	}

	req, _ := http.NewRequest(http.MethodGet, attachmentURL, nil)
	req.SetBasicAuth(username, password)

	return req, nil
}

func (h *handler) RedactValues(ch courier.Channel) []string {
	return []string{
		httpx.BasicAuth(ch.StringConfigForKey(courier.ConfigUsername, ""), ch.StringConfigForKey(courier.ConfigPassword, "")),
	}
}
