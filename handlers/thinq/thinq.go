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
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
)

const configAccountID = "account_id"
const configAPITokenUser = "api_token_user"
const configAPIToken = "api_token"
const maxMsgLength = 1600

var sendURL = "https://api.thinq.com/account/%s/product/origination/sms/send"
var sendMMSURL = "https://api.thinq.com/account/%s/product/origination/mms/send"

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("TQ"), "ThinQ")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodPost, "status", h.receiveStatus)
	return nil
}

// from: Source DID
// to: Destination DID
// type: sms|mms
// message: Content of the message
type moForm struct {
	From    string `validate:"required" name:"from"`
	To      string `validate:"required" name:"to"`
	Type    string `validate:"required" name:"type"`
	Message string `name:"message"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	// get our params
	form := &moForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// create our URN
	urn, err := handlers.StrictTelForCountry(form.From, channel.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	var msg courier.Msg
	if form.Type == "sms" {
		msg = h.Backend().NewIncomingMsg(channel, urn, form.Message)
	} else if form.Type == "mms" {
		msg = h.Backend().NewIncomingMsg(channel, urn, "").WithAttachment(form.Message)
	} else {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unknown message type: %s", form.Type))
	}
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r)
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
	GUID   string `validate:"required" name:"guid"`
	Status string `validate:"required" name:"status"`
}

var statusMapping = map[string]courier.MsgStatusValue{
	"DELIVRD": courier.MsgDelivered,
	"EXPIRED": courier.MsgErrored,
	"DELETED": courier.MsgFailed,
	"UNDELIV": courier.MsgFailed,
	"UNKNOWN": courier.MsgFailed,
	"REJECTD": courier.MsgFailed,
}

// receiveStatus is our HTTP handler function for status updates
func (h *handler) receiveStatus(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
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
	status := h.Backend().NewMsgStatusForExternalID(channel, form.GUID, msgStatus)
	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

type mtMessage struct {
	FromDID string `json:"from_did"`
	ToDID   string `json:"to_did"`
	Message string `json:"message"`
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(_ context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	accountID := msg.Channel().StringConfigForKey(configAccountID, "")
	if accountID == "" {
		return nil, fmt.Errorf("no account id set for TQ channel")
	}

	tokenUser := msg.Channel().StringConfigForKey(configAPITokenUser, "")
	if tokenUser == "" {
		return nil, fmt.Errorf("no token user set for TQ channel")
	}

	token := msg.Channel().StringConfigForKey(configAPIToken, "")
	if token == "" {
		return nil, fmt.Errorf("no token set for TQ channel")
	}

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)

	// we send attachments first so that text appears below
	for _, a := range msg.Attachments() {
		_, u := handlers.SplitAttachment(a)

		data := bytes.NewBuffer(nil)
		form := multipart.NewWriter(data)
		form.WriteField("from_did", strings.TrimLeft(msg.Channel().Address(), "+")[1:])
		form.WriteField("to_did", strings.TrimLeft(msg.URN().Path(), "+")[1:])
		form.WriteField("media_url", u)
		form.Close()

		req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf(sendMMSURL, accountID), data)
		req.Header.Set("Content-Type", form.FormDataContentType())
		req.Header.Set("Accept", "application/json")
		req.SetBasicAuth(tokenUser, token)
		rr, err := utils.MakeHTTPRequest(req)

		// record our status and log
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
		status.AddLog(log)
		if err != nil {
			return status, nil
		}

		// try to get our external id
		externalID, err := jsonparser.GetString([]byte(rr.Body), "guid")
		if err != nil {
			log.WithError("Unable to read external ID", err)
			return status, nil
		}
		status.SetStatus(courier.MsgWired)
		status.SetExternalID(externalID)
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
			req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf(sendURL, accountID), bytes.NewBuffer(bodyJSON))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json")
			req.SetBasicAuth(tokenUser, token)
			rr, err := utils.MakeHTTPRequest(req)

			// record our status and log
			log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
			status.AddLog(log)
			if err != nil {
				return status, nil
			}

			// get our external id
			externalID, err := jsonparser.GetString([]byte(rr.Body), "guid")
			if err != nil {
				log.WithError("Unable to read external ID from guid field", err)
				return status, nil
			}

			status.SetStatus(courier.MsgWired)
			status.SetExternalID(externalID)
		}
	}

	return status, nil
}
