package thinq

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

const configAccountID = "account_id"
const configAPITokenUser = "api_token_user"
const configAPIToken = "api_token"
const maxMsgLength = 1600

var sendURL = "https://api.thinq.com/account/%s/product/origination/sms/send"
var sendMMSURL = "https://api.thinq.com/account/%s/product/origination/mms/send"

func init() {
	channels.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() channels.Handler {
	return &handler{handlers.NewBaseHandler(models.ChannelType("TQ"), "ThinQ")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(r *channels.Routes) error {
	r.Add(h, http.MethodPost, "receive", models.ChannelLogTypeMsgReceive, h.receiveMessage)
	r.Add(h, http.MethodPost, "status", models.ChannelLogTypeMsgStatus, h.receiveStatus)
	return nil
}

// see https://apidocs.thinq.com/#829c8863-8a47-4273-80fb-d962aa64c901
// from: Source DID
// to: Destination DID
// type: sms|mms
// message: Content of the message
type moForm struct {
	From    string `name:"from"    validate:"required"`
	To      string `name:"to"      validate:"required"`
	Type    string `name:"type"    validate:"required"`
	Message string `name:"message"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel *models.Channel, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
	// get our params
	form := &moForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// create our URN
	urn, err := urns.ParsePhone(form.From, channel.Country(), true, false)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	var msg *models.MsgIn

	if form.Type == "sms" {
		msg = models.NewIncomingMsg(channel, urn, form.Message, "", clog)
	} else if form.Type == "mms" {
		if strings.HasPrefix(form.Message, "http://") || strings.HasPrefix(form.Message, "https://") {
			msg = models.NewIncomingMsg(channel, urn, "", "", clog).WithAttachment(form.Message)
		} else {
			msg = models.NewIncomingMsg(channel, urn, "", "", clog).WithAttachment("data:" + form.Message)
		}
	} else {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unknown message type: %s", form.Type))
	}
	return handlers.WriteMsgsAndResponse(ctx, h, []*models.MsgIn{msg}, w, r, clog)
}

// guid: thinQ guid returned when an outbound message is sent via our API
// account_id: Your thinQ account ID
// source_did: Source DID
// destination_did: Destination DID
// timestamp: Time the delivery notification was received
// send_status: User friendly version of the status (i.e.: delivered)
// status: System version of the status (i.e.: DELIVRD)
// error: Error code if any (i.e.: 000)
type statusForm struct {
	GUID   string `name:"guid"   validate:"required"`
	Status string `name:"status" validate:"required"`
}

var statusMapping = map[string]models.MsgStatus{
	"DELIVRD": models.MsgStatusDelivered,
	"EXPIRED": models.MsgStatusErrored,
	"DELETED": models.MsgStatusFailed,
	"UNDELIV": models.MsgStatusFailed,
	"UNKNOWN": models.MsgStatusFailed,
	"REJECTD": models.MsgStatusFailed,
}

// receiveStatus is our HTTP handler function for status updates
func (h *handler) receiveStatus(ctx context.Context, channel *models.Channel, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
	// get our params
	form := &statusForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	msgStatus, found := statusMapping[form.Status]
	if !found {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r,
			fmt.Errorf("unknown status: '%s'", form.Status))
	}

	// write our status
	status := models.NewStatusUpdateByExternalID(channel, form.GUID, msgStatus, clog)
	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

type mtMessage struct {
	FromDID string `json:"from_did"`
	ToDID   string `json:"to_did"`
	Message string `json:"message"`
}

func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	accountID := msg.Channel().StringConfigForKey(configAccountID, "")
	tokenUser := msg.Channel().StringConfigForKey(configAPITokenUser, "")
	token := msg.Channel().StringConfigForKey(configAPIToken, "")
	if accountID == "" || tokenUser == "" || token == "" {
		return channels.ErrChannelConfig
	}

	// we send attachments first so that text appears below
	for _, a := range msg.Attachments() {
		_, u := handlers.SplitAttachment(a)

		data := bytes.NewBuffer(nil)
		form := multipart.NewWriter(data)
		form.WriteField("from_did", strings.TrimLeft(msg.Channel().Address(), "+")[1:])
		form.WriteField("to_did", strings.TrimLeft(msg.URN().Path(), "+")[1:])
		form.WriteField("media_url", u)
		form.Close()

		req, err := http.NewRequest(http.MethodPost, fmt.Sprintf(sendMMSURL, accountID), data)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", form.FormDataContentType())
		req.Header.Set("Accept", "application/json")
		req.SetBasicAuth(tokenUser, token)

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err := handlers.ErrorFromResponse(resp, err); err != nil {
			return err
		}

		// try to get our external id
		externalID, err := jsonparser.GetString(respBody, "guid")
		if err != nil {
			clog.Error(models.ErrorResponseValueMissing("guid"))
			return channels.ErrResponseContent
		}

		res.AddExternalID(externalID)
	}
	// now send our text if we have any
	if msg.Text() != "" {
		parts := handlers.SplitMsgByChannel(msg.Channel(), msg.Text(), maxMsgLength)
		for _, part := range parts {
			body := mtMessage{
				FromDID: strings.TrimLeft(msg.Channel().Address(), "+")[1:],
				ToDID:   strings.TrimLeft(msg.URN().Path(), "+")[1:],
				Message: part,
			}
			bodyJSON, _ := json.Marshal(body)
			req, err := http.NewRequest(http.MethodPost, fmt.Sprintf(sendURL, accountID), bytes.NewBuffer(bodyJSON))
			if err != nil {
				return err
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json")
			req.SetBasicAuth(tokenUser, token)

			resp, respBody, err := h.RequestHTTP(req, clog)
			if err := handlers.ErrorFromResponse(resp, err); err != nil {
				return err
			}

			// get our external id
			externalID, err := jsonparser.GetString(respBody, "guid")
			if err != nil {
				clog.Error(models.ErrorResponseValueMissing("guid"))
				return channels.ErrResponseContent
			}

			res.AddExternalID(externalID)
		}
	}

	return nil
}

func (h *handler) RedactValues(ch *models.Channel) []string {
	return []string{
		httpx.BasicAuth(ch.StringConfigForKey(configAPITokenUser, ""), ch.StringConfigForKey(configAPIToken, "")),
	}
}
