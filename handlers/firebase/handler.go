package firebase

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/urns"
	"google.golang.org/api/option"
)

const (
	configTitle           = "FCM_TITLE"
	configNotification    = "FCM_NOTIFICATION"
	configKey             = "FCM_KEY"
	configCredentialsFile = "FCM_CREDENTIALS_JSON"
)

var (
	maxMsgLength = 1024
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler

	fetchTokenMutex sync.Mutex
}

func newHandler() courier.ChannelHandler {
	return &handler{
		BaseHandler:     handlers.NewBaseHandler(courier.ChannelType("FCM"), "Firebase", handlers.WithRedactConfigKeys(configKey)),
		fetchTokenMutex: sync.Mutex{},
	}
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
	urn, err := urns.New(urns.Firebase, form.From)
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
	urn, err := urns.New(urns.Firebase, form.URN)
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

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	title := msg.Channel().StringConfigForKey(configTitle, "")

	fcmClient, projectID, err := h.GetFCMClient(ctx, msg.Channel(), clog)
	if err != nil {
		return err
	}

	configNotification := msg.Channel().ConfigForKey(configNotification, false)
	notification, _ := configNotification.(bool)

	msgParts := make([]string, 0)
	if msg.Text() != "" {
		msgParts = handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength)
	}

	for _, part := range msgParts {
		payload := messaging.Message{}

		payload.Data = map[string]string{"type": "rapidpro", "title": title, "message": part, "message_id": msg.ID().String(), "session_status": msg.SessionStatus()}

		payload.Token = msg.URNAuth()
		payload.Android = &messaging.AndroidConfig{Priority: "high"}

		if notification {
			payload.Notification = &messaging.Notification{
				Title: title,
				Body:  part,
			}
		}

		result, err := fcmClient.Send(ctx, &payload)
		if err != nil {
			return courier.ErrResponseUnexpected
		}

		if !strings.Contains(result, fmt.Sprintf("projects/%s/messages/", projectID)) {
			return courier.ErrResponseUnexpected
		}
		externalID := strings.TrimLeft(result, fmt.Sprintf("projects/%s/messages/", projectID))
		if externalID == "" {
			return courier.ErrResponseUnexpected
		}

		res.AddExternalID(externalID)

	}

	return nil
}

type FCMClient interface {
	Send(ctx context.Context, message *messaging.Message) (string, error)
}

func (h *handler) GetFCMClient(ctx context.Context, channel courier.Channel, clog *courier.ChannelLog) (FCMClient, string, error) {
	credentialsFile := channel.StringConfigForKey(configCredentialsFile, "")
	if credentialsFile == "" {
		return nil, "", courier.ErrChannelConfig
	}

	var credentialsFileJSON map[string]string

	err := json.Unmarshal([]byte(credentialsFile), &credentialsFileJSON)
	if err != nil {
		return nil, "", courier.ErrChannelConfig
	}

	app, err := firebase.NewApp(ctx, nil, option.WithCredentialsJSON([]byte(credentialsFile)))
	if err != nil {
		return nil, "", err
	}

	fcmClient, err := app.Messaging(ctx)

	return fcmClient, credentialsFileJSON["project_id"], err
}
