package bandwidth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/utils"
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
	channels.RegisterHandler(newHandler)
}

type handler struct {
	handlers.BaseHandler
}

func newHandler(rt *runtime.Runtime, r *channels.Routes) channels.Handler {
	h := &handler{handlers.NewBaseHandler(rt, models.ChannelType("BW"), "Bandwidth")}

	r.AddReceive(h, http.MethodPost, "receive", channels.ReceiveKindMsg, h.receiveMessage)
	r.AddReceive(h, http.MethodPost, "status", channels.ReceiveKindStatus, h.receiveStatus)
	return h
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

// receiveMessage is our receive function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel *models.Channel, r *http.Request, in *channels.Received, clog *models.ChannelLog) error {
	var payload []moMessageData

	body, err := handlers.ReadBody(r, 1000000)
	if err != nil {
		return err
	}

	err = json.Unmarshal(body, &payload)
	if err != nil {
		return err
	}

	if len(payload) == 0 {
		return channels.Ignore("no messages, ignored")
	}

	err = utils.Validate(payload[0])
	if err != nil {
		return err
	}

	messagePayload := payload[0]

	// create our date from the timestamp
	// 2017-05-03T06:04:45Z
	date, err := time.Parse("2006-01-02T15:04:05Z", messagePayload.Message.Time)
	if err != nil {
		return fmt.Errorf("invalid date format: %s", messagePayload.Message.Time)
	}

	// create our URN
	urn, err := urns.ParsePhone(messagePayload.Message.From, channel.Country(), true, false)
	if err != nil {
		return err
	}
	// build our msg
	msg := models.NewIncomingMsg(channel, urn, messagePayload.Message.Text, messagePayload.Message.ID, clog).WithReceivedOn(date)

	for _, attURL := range messagePayload.Message.Media {
		msg.WithAttachment(attURL)
	}

	in.Msg(msg)
	return nil
}

type moStatusData struct {
	Type        string `json:"type" validate:"required"`
	ErrorCode   int    `json:"errorCode"`
	Description string `json:"description"`
	Message     struct {
		ID string `json:"id" validate:"required"`
	} `json:"message" validate:"required"`
}

var statusMapping = map[string]models.MsgStatus{
	"message-sending":   models.MsgStatusSent,
	"message-delivered": models.MsgStatusDelivered,
	"message-failed":    models.MsgStatusFailed,
}

// receiveStatus is our receive function for status updates
func (h *handler) receiveStatus(ctx context.Context, channel *models.Channel, r *http.Request, in *channels.Received, clog *models.ChannelLog) error {
	var payload []moStatusData
	body, err := handlers.ReadBody(r, 1000000)
	if err != nil {
		return err
	}

	err = json.Unmarshal(body, &payload)
	if err != nil {
		return err
	}

	if len(payload) == 0 {
		return channels.Ignore("no messages, ignored")
	}

	err = utils.Validate(payload[0])
	if err != nil {
		return err
	}

	statusPayload := payload[0]
	msgStatus, found := statusMapping[statusPayload.Type]
	if !found {
		return handlers.UnknownStatusError(statusMapping, statusPayload.Type)
	}

	if statusPayload.ErrorCode != 0 {
		clog.Error(models.ErrorExternal(strconv.Itoa(statusPayload.ErrorCode), statusPayload.Description))
	}

	status := models.NewStatusUpdateByExternalID(channel, statusPayload.Message.ID, msgStatus, clog)
	in.Status(status)
	return nil
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

func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	username := msg.Channel().StringConfigForKey(models.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(models.ConfigPassword, "")
	accountID := msg.Channel().StringConfigForKey(configAccountID, "")
	applicationID := msg.Channel().StringConfigForKey(configMsgApplicationID, "")
	if applicationID == "" {
		applicationID = msg.Channel().StringConfigForKey(oldApplicationID, "")
	}

	if username == "" || password == "" || accountID == "" || applicationID == "" {
		return channels.ErrChannelConfig
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
			return channels.ErrConnectionFailed
		}
		if handlers.IsThrottled(resp) {
			return channels.ErrConnectionThrottled
		}

		response := &mtResponse{}
		if err = json.Unmarshal(respBody, response); err != nil {
			return channels.ErrResponseUnparseable
		}

		if resp.StatusCode/100 != 2 {
			return channels.ErrFailedWithReason(response.Type, response.Description)
		}

		if response.ID != "" {
			res.AddExternalID(response.ID)
		}
	}

	return nil
}

// BuildAttachmentRequest to download media for message attachment with Basic auth set
func (h *handler) BuildAttachmentRequest(ctx context.Context, channel *models.Channel, attachmentURL string, clog *models.ChannelLog) (*http.Request, error) {
	username := channel.StringConfigForKey(models.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for BW channel")
	}

	password := channel.StringConfigForKey(models.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for BW channel")
	}

	req, _ := http.NewRequest(http.MethodGet, attachmentURL, nil)
	req.SetBasicAuth(username, password)

	return req, nil
}

func (h *handler) RedactValues(ch *models.Channel) []string {
	return []string{
		httpx.BasicAuth(ch.StringConfigForKey(models.ConfigUsername, ""), ch.StringConfigForKey(models.ConfigPassword, "")),
	}
}
