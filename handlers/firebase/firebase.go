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
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
	"github.com/pkg/errors"
)

const (
	configTitle        = "FCM_TITLE"
	configNotification = "FCM_NOTIFICATION"
	configKey          = "FCM_KEY"
)

var (
	sendURL    = "https://fcm.googleapis.com/fcm/send"
	maxMsgSize = 1024
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("FCM"), "Firebase")}
}

func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodPost, "register", h.registerContact)
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
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
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

	// build our msg
	dbMsg := h.Backend().NewIncomingMsg(channel, urn, form.Msg).WithReceivedOn(date).WithContactName(form.Name).WithURNAuth(form.FCMToken)

	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{dbMsg}, w, r)
}

type registerForm struct {
	URN      string `name:"urn"       validate:"required"`
	FCMToken string `name:"fcm_token" validate:"required"`
	Name     string `name:"name"`
}

// registerContact is our HTTP handler function for when a contact is registered (or renewed)
func (h *handler) registerContact(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
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
	contact, err := h.Backend().GetContact(ctx, channel, urn, form.FCMToken, form.Name)
	if err != nil {
		return nil, err
	}

	// return our contact UUID
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(map[string]string{"contact_uuid": contact.UUID().String()})
	return nil, err
}

type mtPayload struct {
	Data struct {
		Type      string `json:"type"`
		Title     string `json:"title"`
		Message   string `json:"message"`
		MessageID int64  `json:"message_id"`
	} `json:"data"`
	Notification     *mtNotification `json:"notification,omitempty"`
	QuickReplies     []mtQuickReply  `json:"quick_replies,omitempty"`
	ContentAvailable bool            `json:"content_available"`
	To               string          `json:"to"`
	Priority         string          `json:"priority"`
}

type mtNotification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type mtQuickReply struct {
	Title   string `json:"title"`
	Payload string `json:"payload"`
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
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

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	for i, part := range handlers.SplitMsg(handlers.GetTextAndAttachments(msg), maxMsgSize) {
		payload := mtPayload{}

		payload.Data.Type = "rapidpro"
		payload.Data.Title = title
		payload.Data.Message = part
		payload.Data.MessageID = int64(msg.ID())

		payload.To = msg.URNAuth()
		payload.Priority = "high"

		if notification {
			payload.Notification = &mtNotification{
				Title: title,
				Body:  part,
			}
			payload.ContentAvailable = true
		}

		if len(msg.QuickReplies()) > 0 {
			quickReplies := make([]mtQuickReply, len(msg.QuickReplies()))
			for i, qr := range msg.QuickReplies() {
				quickReplies[i].Title = qr
				quickReplies[i].Payload = qr
			}
			payload.QuickReplies = quickReplies
		}

		jsonPayload, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}

		req, _ := http.NewRequest(http.MethodPost, sendURL, bytes.NewReader(jsonPayload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("key=%s", fcmKey))
		rr, err := utils.MakeHTTPRequest(req)
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
		status.AddLog(log)
		if err != nil {
			return status, nil
		}

		// was this successful
		success, _ := jsonparser.GetInt(rr.Body, "success")
		if success != 1 {
			log.WithError("Message Send Error", errors.Errorf("received non-1 value for success in response"))
			return status, nil
		}

		// grab the id if this is our first part
		if i == 0 {
			externalID, err := jsonparser.GetInt(rr.Body, "multicast_id")
			if err != nil {
				log.WithError("Message Send Error", errors.Errorf("unable to get multicast_id from response"))
				return status, nil
			}
			status.SetExternalID(fmt.Sprintf("%d", externalID))
		}
	}

	status.SetStatus(courier.MsgWired)
	return status, nil
}
