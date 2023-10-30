package plivo

/*
POST /handlers/plivo/status/uuid
Status=delivered&From=13342031111&ParentMessageUUID=83876bdb-2033-4876-bfaf-0ff8693705af&TotalRate=0.0025&MCC=405&PartInfo=1+of+1&ErrorCode=&To=918553651111&Units=1&TotalAmount=0.0025&MNC=803&MessageUUID=83876bdb-2033-4876-bfaf-0ff8693705af

POST /api/v1/plivo/receive/uuid
To=4759440448&From=4795961111&TotalRate=0&Units=1&Text=Msg&TotalAmount=0&Type=sms&MessageUUID=7a242edc-2f57-11e7-98c9-06ab0bf64327
*/

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/buger/jsonparser"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/httpx"
)

var (
	sendURL      = "https://api.plivo.com/v1/Account/%s/Message/"
	maxMsgLength = 1600
)

const (
	configPlivoAuthID    = "PLIVO_AUTH_ID"
	configPlivoAuthToken = "PLIVO_AUTH_TOKEN"
	configPlivoAPPID     = "PLIVO_APP_ID"
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("PL"), "Plivo")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "status", courier.ChannelLogTypeMsgStatus, h.receiveStatus)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
	return nil
}

type statusForm struct {
	From              string `name:"From"               validate:"required"`
	To                string `name:"To"                 validate:"required"`
	MessageUUID       string `name:"MessageUUID"        validate:"required"`
	Status            string `name:"Status"             validate:"required"`
	ParentMessageUUID string `name:"ParentMessageUUID"`
}

var statusMapping = map[string]courier.MsgStatus{
	"queued":      courier.MsgStatusWired,
	"delivered":   courier.MsgStatusDelivered,
	"undelivered": courier.MsgStatusSent,
	"sent":        courier.MsgStatusSent,
	"rejected":    courier.MsgStatusFailed,
}

// receiveStatus is our HTTP handler function for status updates
func (h *handler) receiveStatus(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	form := &statusForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	msgStatus, found := statusMapping[form.Status]
	if !found {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, fmt.Sprintf("ignoring unknown status '%s'", form.Status))
	}

	if strings.TrimPrefix(channel.Address(), "+") != strings.TrimPrefix(form.From, "+") {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("invalid to number [%s], expecting [%s]", strings.TrimPrefix(form.From, "+"), strings.TrimPrefix(channel.Address(), "+")))
	}

	externalID := form.MessageUUID
	if form.ParentMessageUUID != "" {
		externalID = form.ParentMessageUUID
	}

	// write our status
	status := h.Backend().NewStatusUpdateByExternalID(channel, externalID, msgStatus, clog)
	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

type moForm struct {
	From        string `name:"From"        validate:"required"`
	To          string `name:"To"          validate:"required"`
	MessageUUID string `name:"MessageUUID" validate:"required"`
	Text        string `name:"Text"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	form := &moForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	if strings.TrimPrefix(channel.Address(), "+") != strings.TrimPrefix(form.To, "+") {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("invalid to number [%s], expecting [%s]", strings.TrimPrefix(form.To, "+"), strings.TrimPrefix(channel.Address(), "+")))
	}

	// create our URN
	urn, err := handlers.StrictTelForCountry(form.From, channel.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// create and write the message
	msg := h.Backend().NewIncomingMsg(channel, urn, form.Text, form.MessageUUID, clog)
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
}

type mtPayload struct {
	Src    string `json:"src"`
	Dst    string `json:"dst"`
	Text   string `json:"text"`
	URL    string `json:"url"`
	Method string `json:"method"`
}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	authID := msg.Channel().StringConfigForKey(configPlivoAuthID, "")
	authToken := msg.Channel().StringConfigForKey(configPlivoAuthToken, "")
	plivoAppID := msg.Channel().StringConfigForKey(configPlivoAPPID, "")
	if authID == "" || authToken == "" || plivoAppID == "" {
		return nil, fmt.Errorf("missing auth_id, auth_token, app_id for PL channel")
	}

	callbackDomain := msg.Channel().CallbackDomain(h.Server().Config().Domain)
	statusURL := fmt.Sprintf("https://%s/c/pl/%s/status", callbackDomain, msg.Channel().UUID())

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)
	parts := handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength)
	for i, part := range parts {
		payload := &mtPayload{
			Src:    strings.TrimPrefix(msg.Channel().Address(), "+"),
			Dst:    strings.TrimPrefix(msg.URN().Path(), "+"),
			Text:   part,
			URL:    statusURL,
			Method: "POST",
		}

		requestBody := &bytes.Buffer{}
		json.NewEncoder(requestBody).Encode(payload)

		// build our request
		req, err := http.NewRequest(http.MethodPost, fmt.Sprintf(sendURL, authID), requestBody)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.SetBasicAuth(authID, authToken)

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 != 2 {
			return status, nil
		}

		externalID, err := jsonparser.GetString(respBody, "message_uuid", "[0]")
		if err != nil {
			return status, fmt.Errorf("unable to parse response body from Plivo")
		}

		// set external id on first part
		if i == 0 {
			status.SetExternalID(externalID)
		}
	}

	status.SetStatus(courier.MsgStatusWired)
	return status, nil
}

func (h *handler) RedactValues(ch courier.Channel) []string {
	return []string{
		httpx.BasicAuth(ch.StringConfigForKey(configPlivoAuthID, ""), ch.StringConfigForKey(configPlivoAuthToken, "")),
	}
}
