package viber

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
	"github.com/pkg/errors"
)

/*
POST /handlers/viber_public/uuid?sig=sig
{"event":"delivered","timestamp":1493817791212,"message_token":504054678623710111,"user_id":"Iul/YIu1tJwyRWKkx7Pxyw=="}

POST /handlers/viber_public/uuid?sig=sig
{"event":"message","timestamp":1493823965629,"message_token":50405727362920111,"sender":{"id":"7nulzrc62mo4kiirIg==","name":"User name","avatar":"https://avatar.jpg","language":"th","country":"HK","api_version":2}

POST /handlers/viber_public/uuid?sig=sig
{"event":"message","timestamp":1493814248770,"message_token":50405319809731111,"sender":{"id":"iu7u0ekVY01115lOIg==","name":"User name","avatar":"https://avatar.jpg","language":"en","country":"PK","api_version":2},"message":{"text":"Msg","type":"text","tracking_data":"579777865"},"silent":false}
*/

var viberSignatureHeader = "X-Viber-Content-Signature"
var sendURL = "https://chatapi.viber.com/pa/send_message"

func init() {
	courier.RegisterHandler(NewHandler())
}

type handler struct {
	handlers.BaseHandler
}

// NewHandler returns a new Infobip handler
func NewHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("VP"), "Viber")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	return s.AddReceiveMsgRoute(h, "POST", "receive", h.ReceiveMessage)
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *handler) ReceiveMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.ReceiveEvent, error) {

	err := h.validateSignature(channel, r)
	if err != nil {
		return nil, err
	}

	viberMsg := &viberMessage{}

	err = handlers.DecodeAndValidateJSON(viberMsg, r)
	if err != nil {
		return nil, courier.WriteError(w, r, err)
	}

	event := viberMsg.Event
	switch event {
	case "webhook":
		return nil, courier.WriteIgnored(w, r, "webhook valid.")
	case "conversation_started":
		return nil, courier.WriteIgnored(w, r, "ignored conversation start")
	case "subscribed":
		viberID := viberMsg.User.ID
		ContactName := viberMsg.User.Name

		// build the URN
		urn := urns.NewURNFromParts(urns.ViberScheme, viberID, "")

		// build the channel event
		channelEvent := h.Backend().NewChannelEvent(channel, courier.NewConversation, urn).WithContactName(ContactName)

		err := h.Backend().WriteChannelEvent(channelEvent)
		if err != nil {
			return nil, err
		}

		return []courier.ReceiveEvent{channelEvent}, courier.WriteChannelEventSuccess(w, r, channelEvent)
	case "unsubscribed":
		viberID := viberMsg.User.ID

		// build the URN
		urn := urns.NewURNFromParts(urns.ViberScheme, viberID, "")

		// build the channel event
		channelEvent := h.Backend().NewChannelEvent(channel, courier.StopContact, urn)

		err := h.Backend().WriteChannelEvent(channelEvent)
		if err != nil {
			return nil, err
		}

		return []courier.ReceiveEvent{channelEvent}, courier.WriteChannelEventSuccess(w, r, channelEvent)
	case "failed":
		msgStatus := h.Backend().NewMsgStatusForExternalID(channel, string(viberMsg.MessageToken), courier.MsgFailed)

		err = h.Backend().WriteMsgStatus(msgStatus)
		if err != nil {
			return nil, err
		}

		return nil, courier.WriteStatusSuccess(w, r, []courier.MsgStatus{msgStatus})

	case "delivered":
		msgStatus := h.Backend().NewMsgStatusForExternalID(channel, fmt.Sprintf("%d", viberMsg.MessageToken), courier.MsgDelivered)

		err = h.Backend().WriteMsgStatus(msgStatus)
		if err != nil {
			return nil, err
		}

		return nil, courier.WriteStatusSuccess(w, r, []courier.MsgStatus{msgStatus})

	case "message":
		sender := viberMsg.Sender.ID
		contactName := viberMsg.Sender.Name

		// create our URN
		urn := urns.NewURNFromParts(urns.ViberScheme, sender, "")

		text := viberMsg.Message.Text
		mediaURL := ""

		// process any attached media
		messageType := viberMsg.Message.Type
		switch messageType {
		case "picture":
			mediaURL = viberMsg.Message.Media
		case "video":
			mediaURL = viberMsg.Message.Media
		case "contact":
			text = fmt.Sprintf("%s: %s", viberMsg.Message.Contact.Name, viberMsg.Message.Contact.PhoneNumber)
		case "url":
			text = viberMsg.Message.Media
		case "location":
			mediaURL = fmt.Sprintf("geo:%f,%f", viberMsg.Message.Location.Latitude, viberMsg.Message.Location.Longitude)
		case "text":
			text = viberMsg.Message.Text
		default:
			return nil, courier.WriteError(w, r, fmt.Errorf("unknown message type: %s", messageType))
		}

		if text == "" && mediaURL == "" {
			return nil, courier.WriteError(w, r, fmt.Errorf("missing text or media in message in request body"))
		}

		// build our msg
		msg := h.Backend().NewIncomingMsg(channel, urn, text).WithExternalID(fmt.Sprintf("%d", viberMsg.MessageToken)).WithContactName(contactName)

		if mediaURL != "" {
			msg.WithAttachment(mediaURL)
		}

		// and finally queue our message
		err = h.Backend().WriteMsg(msg)
		if err != nil {
			return nil, err
		}

		return []courier.ReceiveEvent{msg}, courier.WriteMsgSuccess(w, r, []courier.Msg{msg})
	}

	return nil, courier.WriteError(w, r, fmt.Errorf("not handled, unknown event: %s", event))
}

type viberMessage struct {
	Event        string `json:"event" validate:"required"`
	Timestamp    int64  `json:"timestamp" validate:"required"`
	MessageToken int64  `json:"message_token" validate:"required"`
	Sender       struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"sender"`
	User struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"user"`
	Message struct {
		Text    string `json:"text"`
		Media   string `json:"media"`
		Contact struct {
			Name        string `json:"name"`
			PhoneNumber string `json:"phone_number"`
		}
		Location struct {
			Latitude  float64 `json:"lat"`
			Longitude float64 `json:"lon"`
		}
		Type         string `json:"type"`
		TrackingData string `json:"tracking_data"`
	} `json:"message"`
}

// see https://developers.viber.com/docs/api/rest-bot-api/#callbacks
func (h *handler) validateSignature(channel courier.Channel, r *http.Request) error {
	actual := r.Header.Get(viberSignatureHeader)
	if actual == "" {
		return fmt.Errorf("missing request signature")
	}

	confAuth := channel.ConfigForKey(courier.ConfigAuthToken, "")
	authToken, isStr := confAuth.(string)
	if !isStr || authToken == "" {
		return fmt.Errorf("invalid or missing auth token in config")
	}

	// read our body
	body, err := ioutil.ReadAll(r.Body)
	r.Body = ioutil.NopCloser(bytes.NewBuffer(body))
	expected, err := viberCalculateSignature(authToken, body)
	if err != nil {
		return err
	}

	// compare signatures in way that isn't sensitive to a timing attack
	if !hmac.Equal(expected, []byte(actual)) {
		return fmt.Errorf("invalid request signature")
	}

	return nil
}

func viberCalculateSignature(authToken string, contents []byte) ([]byte, error) {

	var buffer bytes.Buffer
	buffer.Write(contents)

	// hash with SHA256
	mac := hmac.New(sha256.New, []byte(authToken))
	mac.Write(buffer.Bytes())
	hash := mac.Sum(nil)

	return hash, nil
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(msg courier.Msg) (courier.MsgStatus, error) {
	confAuth := msg.Channel().ConfigForKey(courier.ConfigAuthToken, "")
	authToken, isStr := confAuth.(string)
	if !isStr || authToken == "" {
		return nil, fmt.Errorf("invalid auth token config")
	}

	viberMsg := viberOutgoingMessage{
		AuthToken:    authToken,
		Receiver:     msg.URN().Path(),
		Text:         courier.GetTextAndAttachments(msg),
		Type:         "text",
		TrackingData: msg.ID().String(),
	}

	requestBody := &bytes.Buffer{}
	err := json.NewEncoder(requestBody).Encode(viberMsg)
	if err != nil {
		return nil, err
	}

	// build our request
	req, err := http.NewRequest(http.MethodPost, sendURL, requestBody)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	rr, err := utils.MakeHTTPRequest(req)

	// record our status and log
	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr)
	status.AddLog(log)
	if err != nil {
		log.WithError("Message Send Error", err)
		return status, nil
	}

	responseStatus, err := jsonparser.GetInt([]byte(rr.Body), "status")
	if err != nil {
		log.WithError("Message Send Error", errors.Errorf("received invalid JSON response"))
		status.SetStatus(courier.MsgFailed)
		return status, nil
	}
	if responseStatus != 0 {
		log.WithError("Message Send Error", errors.Errorf("received non-0 status: '%d'", responseStatus))
		status.SetStatus(courier.MsgFailed)
		return status, nil
	}

	status.SetStatus(courier.MsgWired)
	return status, nil
}

type viberOutgoingMessage struct {
	AuthToken    string `json:"auth_token"`
	Receiver     string `json:"receiver"`
	Text         string `json:"text"`
	Type         string `json:"type"`
	TrackingData string `json:"tracking_data"`
}
