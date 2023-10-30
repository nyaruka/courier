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
	"log/slog"
	"strconv"

	"fmt"

	"net/http"
	"time"

	"github.com/buger/jsonparser"
	"github.com/golang-jwt/jwt/v5"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
)

var (
	smsURL                    = "https://rest.messagebird.com/messages"
	mmsURL                    = "https://rest.messagebird.com/mms"
	signatureHeader           = "Messagebird-Signature-Jwt"
	maxRequestBodyBytes int64 = 1024 * 1024
	// error code messagebird returns when a contact has sent "stop"
	errorStopped = 103
)

type Message struct {
	Recipients []string `json:"recipients"`
	Reference  string   `json:"reference,omitempty"`
	Originator string   `json:"originator"`
	Subject    string   `json:"subject,omitempty"`
	Body       string   `json:"body,omitempty"`
	MediaURLs  []string `json:"mediaUrls,omitempty"`
}

type ReceivedStatus struct {
	ID              string    `schema:"id"`
	Reference       string    `schema:"reference"`
	Recipient       string    `schema:"recipient,required"`
	Status          string    `schema:"status,required"`
	StatusReason    string    `schema:"statusReason"`
	StatusDatetime  time.Time `schema:"statusDatetime"`
	StatusErrorCode int       `schema:"statusErrorCode"`
}

var statusMapping = map[string]courier.MsgStatus{
	"scheduled":       courier.MsgStatusSent,
	"delivery_failed": courier.MsgStatusFailed,
	"sent":            courier.MsgStatusSent,
	"buffered":        courier.MsgStatusSent,
	"delivered":       courier.MsgStatusDelivered,
	"expired":         courier.MsgStatusFailed,
}

type ReceivedMessage struct {
	ID              string   `json:"id"`
	Recipient       string   `json:"recipient"`
	Originator      string   `json:"originator"`
	Body            string   `json:"body"`
	CreatedDatetime string   `json:"createdDatetime"`
	MediaURLs       []string `json:"mediaUrls"`
	MMS             bool     `json:"mms"`
}

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
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, handlers.JSONPayload(h, h.receiveMessage))
	s.AddHandlerRoute(h, http.MethodGet, "status", courier.ChannelLogTypeMsgStatus, h.receiveStatus)

	return nil
}

func (h *handler) receiveStatus(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {

	// get our params
	receivedStatus := &ReceivedStatus{}
	err := handlers.DecodeAndValidateForm(receivedStatus, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "no msg status, ignoring")
	}

	msgStatus, found := statusMapping[receivedStatus.Status]
	if !found {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unknown status '%s', must be one of 'queued', 'failed', 'sent', 'delivered', or 'undelivered'", receivedStatus.Status))
	}

	// if the message id was passed explicitely, use that
	var status courier.StatusUpdate
	if receivedStatus.Reference != "" {
		msgID, err := strconv.ParseInt(receivedStatus.Reference, 10, 64)
		if err != nil {
			slog.Error("error converting Messagebird status id to integer", "error", err, "id", receivedStatus.Reference)
		} else {
			status = h.Backend().NewStatusUpdate(channel, courier.MsgID(msgID), msgStatus, clog)
		}
	}

	// if we have no status, then build it from the external (messagebird) id
	if status == nil {
		status = h.Backend().NewStatusUpdateByExternalID(channel, receivedStatus.ID, msgStatus, clog)
	}

	if receivedStatus.StatusErrorCode == errorStopped {
		urn, err := urns.NewTelURNForCountry(receivedStatus.Recipient, "")
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
		}
		// create a stop channel event
		channelEvent := h.Backend().NewChannelEvent(channel, courier.EventTypeStopContact, urn, clog)
		err = h.Backend().WriteChannelEvent(ctx, channelEvent, clog)
		if err != nil {
			return nil, err
		}
		clog.Error(courier.ErrorExternal(fmt.Sprint(receivedStatus.StatusErrorCode), "Contact has sent 'stop'"))
	}

	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *ReceivedMessage, clog *courier.ChannelLog) ([]courier.Event, error) {
	err := h.validateSignature(channel, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// no message? ignore this
	if payload.Body == "" && !payload.MMS {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "Ignoring request, no message")
	}

	// create our date from the timestamp
	standardDateLayout := "2006-01-02T15:04:05+00:00"
	date, err := time.Parse(standardDateLayout, payload.CreatedDatetime)
	if err != nil {
		//try shortcode format
		shortCodeDateLayout := "20060102150405"
		date, err = time.Parse(shortCodeDateLayout, payload.CreatedDatetime)
		if err != nil {
			return nil, fmt.Errorf("unable to parse date '%s': %v", payload.CreatedDatetime, err)
		}
	}

	// create our URN
	urn, err := handlers.StrictTelForCountry(payload.Originator, channel.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}
	text := payload.Body

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, text, payload.ID, clog).WithReceivedOn(date.UTC())

	// process any attached media
	if payload.MMS {
		for _, mediaURL := range payload.MediaURLs {
			msg.WithAttachment(mediaURL)
		}
	}
	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
}

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {

	authToken := msg.Channel().StringConfigForKey(courier.ConfigAuthToken, "")
	if authToken == "" {
		return nil, fmt.Errorf("missing config 'auth_token' for Messagebird channel")
	}

	user := msg.URN().Path()
	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)

	// create base payload
	payload := &Message{
		Recipients: []string{user},
		Originator: msg.Channel().Address(),
		Reference:  msg.ID().String(),
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
		_, mediaURL := handlers.SplitAttachment(attachment)
		payload.MediaURLs = append(payload.MediaURLs, mediaURL)
	}

	jsonBody := jsonx.MustMarshal(payload)

	req, err := http.NewRequest(http.MethodPost, sendUrl, bytes.NewReader(jsonBody))

	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	var bearer = "AccessKey " + authToken
	req.Header.Set("Authorization", bearer)

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		return status, nil
	}
	status.SetStatus(courier.MsgStatusWired)

	externalID, err := jsonparser.GetString(respBody, "id")
	if err != nil {
		clog.Error(courier.ErrorResponseUnparseable("JSON"))
		return status, nil
	}
	status.SetExternalID(externalID)
	return status, nil
}

func verifyToken(tokenString string, secret string) (jwt.MapClaims, error) {
	// Parse the token with the provided secret to get the claims
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
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

func calculateSignature(body []byte) string {
	preHashSignature := sha256.Sum256(body)
	return hex.EncodeToString(preHashSignature[:])
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
	CalledURL := fmt.Sprintf("https://%s%s", c.CallbackDomain(h.Server().Config().Domain), r.URL.Path)
	expectedURLHash := calculateSignature([]byte(CalledURL))
	URLHash := verifiedToken["url_hash"].(string)

	if !hmac.Equal([]byte(expectedURLHash), []byte(URLHash)) {
		return fmt.Errorf("invalid request signature, signature doesn't match expected signature for URL.")
	}

	if verifiedToken["payload_hash"] != nil {
		payloadHash := verifiedToken["payload_hash"].(string)

		body, err := handlers.ReadBody(r, maxRequestBodyBytes)
		if err != nil {
			return fmt.Errorf("unable to read request body: %s", err)
		}

		expectedSignature := calculateSignature(body)
		if !hmac.Equal([]byte(expectedSignature), []byte(payloadHash)) {
			return fmt.Errorf("invalid request signature, signature doesn't match expected signature for body.")
		}
	}

	return nil
}
