package firebase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
)

const (
	configTitle        = "FCM_TITLE"
	configNotification = "FCM_NOTIFICATION"
	configKey          = "FCM_KEY"
)

var (
	sendURL      = "https://fcm.googleapis.com/fcm/send"
	maxMsgLength = 1024
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("FCM"), "Firebase", handlers.WithRedactConfigKeys(configKey))}
}

func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodPost, "register", courier.ChannelLogTypeEventReceive, h.registerContact)
	return nil
}

type receiveForm struct {
	From     string `name:"from"       validate:"required"`
	Msg      string `name:"msg"`
	FCMToken string `name:"fcm_token"`
	Date     string `name:"date"`
	Name     string `name:"name"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	form := &receiveForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	date := time.Now().UTC()
	if form.Date != "" {
		date, err = time.Parse("2006-01-02T15:04:05.000", form.Date)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unable to parse date: %s", form.Date))
		}
	}

	// create our URN
	urn, err := urns.NewFirebaseURN(form.From)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// if a new auth token was provided, record that
	var authTokens map[string]string
	if form.FCMToken != "" {
		authTokens = map[string]string{"default": form.FCMToken}
	}

	// build our msg
	dbMsg := h.Backend().NewIncomingMsg(channel, urn, form.Msg, "", clog).WithReceivedOn(date).WithContactName(form.Name).WithURNAuthTokens(authTokens)

	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{dbMsg}, w, r, clog)
}

type registerForm struct {
	URN      string `name:"urn"       validate:"required"`
	FCMToken string `name:"fcm_token" validate:"required"`
	Name     string `name:"name"`
}

// registerContact is our HTTP handler function for when a contact is registered (or renewed)
func (h *handler) registerContact(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	form := &registerForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// create our URN
	urn, err := urns.NewFirebaseURN(form.URN)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// create our contact
	contact, err := h.Backend().GetContact(ctx, channel, urn, map[string]string{"default": form.FCMToken}, form.Name, clog)
	if err != nil {
		return nil, err
	}

	// return our contact UUID
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(map[string]string{"contact_uuid": string(contact.UUID())})
	return nil, err
}

type mtPayload struct {
	Data struct {
		Type          string   `json:"type"`
		Title         string   `json:"title"`
		Message       string   `json:"message"`
		MessageID     int64    `json:"message_id"`
		SessionStatus string   `json:"session_status"`
		QuickReplies  []string `json:"quick_replies,omitempty"`
	} `json:"data"`
	Notification     *mtNotification `json:"notification,omitempty"`
	ContentAvailable bool            `json:"content_available"`
	To               string          `json:"to"`
	Priority         string          `json:"priority"`
}

type mtNotification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	title := msg.Channel().StringConfigForKey(configTitle, "")
	if title == "" {
		return nil, fmt.Errorf("no FCM_TITLE set for FCM channel")
	}

	fcmKey := msg.Channel().StringConfigForKey(configKey, "")
	if fcmKey == "" {
		return nil, fmt.Errorf("no FCM_KEY set for FCM channel")
	}

	configNotification := msg.Channel().ConfigForKey(configNotification, false)
	notification, _ := configNotification.(bool)

	msgParts := make([]string, 0)
	if msg.Text() != "" {
		msgParts = handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength)
	}

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)
	for i, part := range msgParts {
		payload := mtPayload{}

		payload.Data.Type = "rapidpro"
		payload.Data.Title = title
		payload.Data.Message = part
		payload.Data.MessageID = int64(msg.ID())
		payload.Data.SessionStatus = msg.SessionStatus()

		// include any quick replies on the last piece we send
		if i == len(msgParts)-1 {
			payload.Data.QuickReplies = msg.QuickReplies()
		}

		payload.To = msg.URNAuth()
		payload.Priority = "high"

		if notification {
			payload.Notification = &mtNotification{
				Title: title,
				Body:  part,
			}
			payload.ContentAvailable = true
		}

		jsonPayload := jsonx.MustMarshal(payload)

		req, err := http.NewRequest(http.MethodPost, sendURL, bytes.NewReader(jsonPayload))
		if err != nil {
			return nil, err
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("key=%s", fcmKey))

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 != 2 {
			return status, nil
		}

		// was this successful
		success, _ := jsonparser.GetInt(respBody, "success")
		if success != 1 {
			clog.Error(courier.ErrorResponseValueUnexpected("success", "1"))
			return status, nil
		}

		// grab the id if this is our first part
		if i == 0 {
			externalID, err := jsonparser.GetInt(respBody, "multicast_id")
			if err != nil {
				clog.Error(courier.ErrorResponseValueMissing("multicast_id"))
				return status, nil
			}
			status.SetExternalID(fmt.Sprintf("%d", externalID))
		}
	}

	status.SetStatus(courier.MsgStatusWired)
	return status, nil
}
