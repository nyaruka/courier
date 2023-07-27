package messagebird

/*
 * Handler for MessageBird
 */
import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"

	"encoding/json"

	"fmt"

	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/urns"
)

var (
	smsURL                    = "https://rest.messagebird.com/messages"
	mmsURL                    = "https://rest.messagebird.com/mms"
	signatureHeader           = "Messagebird-Signature-Jwt"
	maxRequestBodyBytes int64 = 1024 * 1024
)

func init() {
	courier.RegisterHandler(newHandler("MBD", "Messagebird", true))
}

type handler struct {
	handlers.BaseHandler
	validateSignatures bool
}

func newHandler(channelType courier.ChannelType, name string, validateSignatures bool) courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("MBD"), "Messagebird"), validateSignatures}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
	return nil
}

func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	err := h.validateSignature(channel, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	payload := &ReceivedMessage{}
	err = handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// no message? ignore this
	if payload.Body == "" && !payload.Mms {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "Ignoring request, no message")
	}

	// create our date from the timestamp
	date := payload.CreatedDatetime

	// create our URN
	urn, err := urns.NewTelURNForCountry(payload.Originator, "US")
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}
	text := payload.Body

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, text, clog).WithReceivedOn(date.UTC()).WithExternalID(payload.ID)

	// process any attached media
	if payload.Mms {
		for i := 0; i < len(payload.MediaUrls); i++ {
			msg.WithAttachment(payload.MediaUrls[i])
			println("this is the media url", payload.MediaUrls[i])
		}
	}
	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r, clog)
}

func (h *handler) Send(ctx context.Context, msg courier.Msg, clog *courier.ChannelLog) (courier.MsgStatus, error) {

	authToken := msg.Channel().StringConfigForKey(courier.ConfigAuthToken, "")
	if authToken == "" {
		return nil, fmt.Errorf("missing config 'auth_token' for Messagebird channel")
	}

	user := msg.URN().Path()
	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored, clog)

	// create base payload
	payload := &Message{
		Recipients: []string{user},
		Originator: msg.Channel().Address(),
	}
	// build message payload

	if len(msg.Text()) > 0 {
		payload.Body = msg.Text()
	}
	sendUrl := ""
	if len(msg.Attachments()) > 0 {
		sendUrl = mmsURL
	} else {
		sendUrl = smsURL
	}
	for _, attachment := range msg.Attachments() {
		mediaType, mediaURL := handlers.SplitAttachment(attachment)
		switch strings.Split(mediaType, "/")[0] {
		case "image":
			payload.MediaUrls = append(payload.MediaUrls, mediaURL)
		default:
			clog.Error(courier.ErrorMediaUnsupported(mediaType))
		}
	}

	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, sendUrl, bytes.NewReader(jsonBody))

	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	var bearer = "AccessKey " + authToken
	req.Header.Set("Authorization", bearer)

	resp, _, err := handlers.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return status, nil
	}

	status.SetStatus(courier.MsgWired)

	return status, nil
}

func verifyToken(tokenString string, secret string) (jwt.MapClaims, error) {
	// Parse the token with the provided secret to get the claims
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Validate the signing method
		// We only allow HS256
		// ref: https://auth0.com/blog/critical-vulnerabilities-in-json-web-token-libraries/
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}
		// Return the secret used to sign the token
		return []byte(secret), nil
	})

	if err != nil {
		return nil, err
	}

	// Check if the token is valid
	if token.Valid {
		tokenClaims := token.Claims.(jwt.MapClaims)
		return tokenClaims, nil
	}

	return nil, fmt.Errorf("Invalid token or missing payload_hash claim")
}

func (h *handler) validateSignature(c courier.Channel, r *http.Request) error {
	if !h.validateSignatures {
		return nil
	}
	headerSignature := r.Header.Get(signatureHeader)
	if headerSignature == "" {
		return fmt.Errorf("missing request signature")
	}
	configsecret := c.StringConfigForKey(courier.ConfigSecret, "")
	if configsecret == "" {
		return fmt.Errorf("missing configsecret")
	}
	verifiedToken, err := verifyToken(headerSignature, configsecret)
	if err != nil {
		return err
	}
	payloadHash := verifiedToken["payload_hash"].(string)

	body, err := handlers.ReadBody(r, maxRequestBodyBytes)
	if err != nil {
		return fmt.Errorf("unable to read request body: %s", err)
	}

	preHashSignature := sha256.Sum256(body)
	expectedSignature := hex.EncodeToString(preHashSignature[:])
	if !hmac.Equal([]byte(expectedSignature), []byte(payloadHash)) {
		return fmt.Errorf("invalid request signature, signature expected: %s got: %s for body: '%s'", expectedSignature, payloadHash, string(body))
	}
	return nil
}

type Message struct {
	Recipients []string `json:"recipients"`
	Originator string   `json:"originator"`
	Subject    string   `json:"subject,omitempty"`
	Body       string   `json:"body,omitempty"`
	MediaUrls  []string `json:"mediaUrls,omitempty"`
}

type ReceivedMessage struct {
	Receiver          string    `json:"receiver"`
	Sender            string    `json:"sender"`
	Message           string    `json:"message"`
	Date              int       `json:"date"`
	DateUtc           int       `json:"date_utc"`
	Reference         string    `json:"reference"`
	ID                string    `json:"id"`
	MessageID         string    `json:"message_id"`
	Recipient         string    `json:"recipient"`
	Originator        string    `json:"originator"`
	Body              string    `json:"body"`
	CreatedDatetime   time.Time `json:"createdDatetime"`
	MediaUrls         []string  `json:"mediaUrls"`
	MediaContentTypes []string  `json:"mediaContentTypes"`
	Subject           string    `json:"subject"`
	Mms               bool      `json:"mms"`
}
