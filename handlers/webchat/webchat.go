package webchat

import (
	"bytes"
	"context"
	"encoding/json"
	. "github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
	"golang.org/x/text/language"
	"net/http"
	"strings"
	"time"
)

func init() {
	RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() ChannelHandler {
	return &handler{handlers.NewBaseHandler(ChannelType("WCH"), "WebChat")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "register", h.registerUser)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
	return nil
}

// registerUser is our HTTP handler function for register websocket contacts
func (h *handler) registerUser(ctx context.Context, channel Channel, w http.ResponseWriter, r *http.Request) ([]Event, error) {
	payload := &userPayload{}
	err := handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// no URN? ignore this
	if payload.URN == "" {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "Ignoring request, no identifier")
	}

	// the list of data we will return in our response
	data := make([]interface{}, 0, 2)

	// create our URN
	urn, errURN := urns.NewURNFromParts(channel.Schemes()[0], payload.URN, "", "")
	if errURN != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, errURN)
	}

	contact, errGetContact := h.Backend().GetContact(ctx, channel, urn, "", "")
	if errGetContact != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, errGetContact)
	}

	// Getting the language in ISO3
	tag := language.MustParse(payload.Language)
	languageBase, _ := tag.Base()

	_, errLang := h.Backend().AddLanguageToContact(ctx, channel, languageBase.ISO3(), contact)
	if errLang != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, errLang)
	}

	// build our response
	data = append(data, NewEventRegisteredContactData(contact.UUID()))

	return nil, WriteDataResponse(ctx, w, http.StatusOK, "Events Handled", data)
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel Channel, w http.ResponseWriter, r *http.Request) ([]Event, error) {
	payload := &msgPayload{}
	err := handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// no message? ignore this
	if payload.Text == "" && payload.AttachmentURL == "" {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "Ignoring request, no message or no attachment")
	}

	urn, errURN := urns.NewURNFromParts(channel.Schemes()[0], payload.From, "", "")
	if errURN != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, errURN)
	}
	text := payload.Text

	msg := h.Backend().NewIncomingMsg(channel, urn, text)

	if payload.AttachmentURL != "" {
		msg.WithAttachment(payload.AttachmentURL)
	}

	return handlers.WriteMsgsAndResponse(ctx, h, []Msg{msg}, w, r)
}

func (h *handler) sendMsgPart(msg Msg, apiURL string, payload *dataPayload) (string, *ChannelLog, error) {
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		log := NewChannelLog("unable to build JSON body", msg.Channel(), msg.ID(), "", "", NilStatusCode, "", "", time.Duration(0), err)
		return "", log, err
	}

	req, _ := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	rr, err := utils.MakeHTTPRequest(req)

	// build our channel log
	log := NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)

	return "", log, nil
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg Msg) (MsgStatus, error) {
	address := msg.Channel().Address()

	data := &dataPayload{
		ID:          msg.ID().String(),
		Text:        msg.Text(),
		To:          msg.URN().Path(),
		ToNoPlus:    strings.Replace(msg.URN().Path(), "+", "", 1),
		From:        address,
		FromNoPlus:  strings.Replace(address, "+", "", 1),
		Channel:     strings.Replace(address, "+", "", 1),
		Metadata:    nil,
		Attachments: nil,
	}

	metadata := make(map[string]interface{}, 0)

	if len(msg.QuickReplies()) > 0 {
		buildQuickReplies := make([]string, 0)
		for _, item := range msg.QuickReplies() {
			item = strings.ReplaceAll(item, "\\/", "/")
			item = strings.ReplaceAll(item, "\\\"", "\"")
			item = strings.ReplaceAll(item, "\\\\", "\\")
			buildQuickReplies = append(buildQuickReplies, item)
		}
		metadata["quick_replies"] = buildQuickReplies
	}

	if len(msg.Attachments()) > 0 {
		data.Attachments = msg.Attachments()
	}

	if msg.ReceiveAttachment() != "" {
		metadata["receive_attachment"] = msg.ReceiveAttachment()
	}

	data.Metadata = metadata

	// the status that will be written for this message
	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), MsgErrored)

	// whether we encountered any errors sending any parts
	hasError := true

	// if we have text, send that if we aren't sending it as a caption
	if msg.Text() != "" {
		externalID, log, err := h.sendMsgPart(msg, address, data)
		status.SetExternalID(externalID)
		hasError = err != nil
		status.AddLog(log)
	}

	if !hasError {
		status.SetStatus(MsgWired)
	}

	return status, nil
}

type userPayload struct {
	URN      string `json:"urn"`
	Language string `json:"language"`
}

type msgPayload struct {
	Text          string `json:"text"`
	From          string `json:"from"`
	AttachmentURL string `json:"attachment_url"`
}

type dataPayload struct {
	ID          string                 `json:"id"`
	Text        string                 `json:"text"`
	To          string                 `json:"to"`
	ToNoPlus    string                 `json:"to_no_plus"`
	From        string                 `json:"from"`
	FromNoPlus  string                 `json:"from_no_plus"`
	Channel     string                 `json:"channel"`
	Metadata    map[string]interface{} `json:"metadata"`
	Attachments []string               `json:"attachments"`
}
