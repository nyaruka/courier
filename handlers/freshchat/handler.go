package freshchat

/*
 * Handler for FreshChat
 */
import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
)

var (
	apiURL          = "https://api.freshchat.com/v2"
	signatureHeader = "X-FreshChat-Signature"
)

func init() {
	courier.RegisterHandler(newHandler("FC", "FreshChat", true))
}

type handler struct {
	handlers.BaseHandler
	validateSignatures bool
}

func newHandler(channelType courier.ChannelType, name string, validateSignatures bool) courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("FC"), "FreshChat"), validateSignatures}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, handlers.JSONPayload(h, h.receiveMessage))
	return nil
}
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *moPayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	err := h.validateSignature(channel, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// no message? ignore this
	if payload.Data.Message == nil || payload.Data.Message.ActorID == "" {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "Ignoring request, no message")
	}

	// something we sent? ignore this
	if payload.Data.Message.ActorType == "agent" {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "Ignoring request, Agent Message")
	}

	// create our date from the timestamp
	date := payload.Data.Message.CreatedTime

	// create our URN
	urn := urns.NilURN
	urnstring := fmt.Sprintf("%s/%s", payload.Data.Message.ChannelID, payload.Data.Message.ActorID)
	urn, err = urns.NewURNFromParts("freshchat", urnstring, "", "")
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}
	text := ""
	mediaURL := ""
	// our text is either "text" or "image"
	for _, data := range payload.Data.Message.MessageParts {
		if data.Text != nil {
			text = data.Text.Content
		}
		if data.Image != nil {
			mediaURL = string(data.Image.URL)
		}
	}
	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, text, payload.Data.Message.ID, clog).WithReceivedOn(date)

	//add image
	if mediaURL != "" {
		msg.WithAttachment(mediaURL)
	}
	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
}

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {

	agentID := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if agentID == "" {
		return nil, fmt.Errorf("missing config 'username' for FC channel")
	}

	authToken := msg.Channel().StringConfigForKey(courier.ConfigAuthToken, "")
	if authToken == "" {
		return nil, fmt.Errorf("missing config 'auth_token' for FC channel")
	}

	user := strings.Split(msg.URN().Path(), "/")
	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)
	url := apiURL + "/conversations"

	// create base payload
	payload := &messagePayload{
		Messages: []Messages{
			{
				MessageParts: []MessageParts{},
				ActorID:      agentID,
				ActorType:    "agent",
			}},
		ChannelID: user[0],
		Users: []Users{
			{
				ID: user[1],
			},
		},
	}
	// build message payload

	if len(msg.Text()) > 0 {
		text := msg.Text()
		msgtext := &MessageParts{}
		msgtext.Text = &Text{Content: text}
		payload.Messages[0].MessageParts = append(payload.Messages[0].MessageParts, *msgtext)
	}
	for _, attachment := range msg.Attachments() {
		mediaType, mediaURL := handlers.SplitAttachment(attachment)
		switch strings.Split(mediaType, "/")[0] {
		case "image":
			var msgimage = new(MessageParts)
			msgimage.Image = &Image{URL: mediaURL}
			payload.Messages[0].MessageParts = append(payload.Messages[0].MessageParts, *msgimage)
		default:
			clog.Error(courier.ErrorMediaUnsupported(mediaType))
		}
	}

	jsonBody := jsonx.MustMarshal(payload)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(jsonBody))

	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	var bearer = "Bearer " + authToken
	req.Header.Set("Authorization", bearer)

	resp, _, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return status, nil
	}

	status.SetStatus(courier.MsgStatusWired)

	return status, nil
}

func (h *handler) validateSignature(c courier.Channel, r *http.Request) error {
	if !h.validateSignatures {
		return nil
	}
	key := c.StringConfigForKey(courier.ConfigSecret, "")
	if key == "" {
		return fmt.Errorf("missing config 'secret' for FC channel")
	}
	//x509 parser needs newlines for valid key- RP stores config strings without them.
	// this puts them back in
	key = strings.Replace(key, "- ", "-\n", 1)
	key = strings.Replace(key, " -", "\n-", 1)
	var rsaPubKey = []byte(key)

	actual := r.Header.Get(signatureHeader)
	if actual == "" {
		return fmt.Errorf("missing request signature")
	}
	buf, _ := io.ReadAll(r.Body)
	rdr1 := io.NopCloser(bytes.NewBuffer(buf))
	rdr2 := io.NopCloser(bytes.NewBuffer(buf))
	token, err := io.ReadAll(rdr1)
	if err != nil {
		return fmt.Errorf("unable to read Body, %s", err.Error())
	}
	r.Body = rdr2

	var b64Sig = []byte(actual)
	block, _ := pem.Decode(rsaPubKey)
	if err != nil {
		return fmt.Errorf("failed to decode public key, %s", err.Error())
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse DER encoded public key, %s", err.Error())
	}
	hash := sha256.New()
	if _, err := bytes.NewReader(token).WriteTo(hash); err != nil {
		return fmt.Errorf("unable to hash signed token, %s", err.Error())
	}
	decodedSig, err := base64.StdEncoding.DecodeString(string(b64Sig))
	if err != nil {
		return fmt.Errorf("unable to decode base64 signature, %s", err.Error())
	}

	if err := rsa.VerifyPKCS1v15(pub.(*rsa.PublicKey), crypto.SHA256, hash.Sum(nil), decodedSig); err != nil {
		return fmt.Errorf("unable to verify signature, %s", err.Error())
	}

	return nil
}

type messagePayload struct {
	Messages  []Messages `json:"messages"`
	Status    string     `json:"status,omitempty"`
	ChannelID string     `json:"channel_id"`
	Users     []Users    `json:"users"`
}
type Messages struct {
	MessageParts []MessageParts `json:"message_parts"`
	ActorID      string         `json:"actor_id"`
	ActorType    string         `json:"actor_type"`
}

type Users struct {
	ID string `json:"id"`
}
type moPayload struct {
	Actor      Actor     `json:"actor"`
	Action     string    `json:"action"`
	ActionTime time.Time `json:"action_time"`
	Data       Data      `json:"data"`
}
type Actor struct {
	ActorType string `json:"actor_type"`
	ActorID   string `json:"actor_id"`
}
type Text struct {
	Content string `json:"content,omitempty"`
}
type MessageParts struct {
	Text  *Text  `json:"text,omitempty"`
	Image *Image `json:"image,omitempty"`
}
type Message struct {
	MessageParts   []MessageParts `json:"message_parts"`
	AppID          string         `json:"app_id"`
	ActorID        string         `json:"actor_id"`
	ID             string         `json:"id"`
	ChannelID      string         `json:"channel_id"`
	ConversationID string         `json:"conversation_id"`
	MessageType    string         `json:"message_type"`
	ActorType      string         `json:"actor_type"`
	CreatedTime    time.Time      `json:"created_time"`
}
type Data struct {
	Message *Message `json:"message,omitempty"`
}
type Image struct {
	URL string `json:"url,omitempty"`
}
