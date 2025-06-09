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
	"errors"
	"fmt"
	"log/slog"
	"strconv"

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

type formMessage struct {
	ID              string   `name:"id"`
	MID             string   `name:"mid"`       //shortcode only
	Shortcode       string   `name:"shortcode"` //shortcode only
	Recipient       string   `name:"recipient"` //non-shortcode only
	Originator      string   `name:"originator" validate:"required"`
	Body            string   `name:"body"`
	MediaURLs       []string `name:"mediaUrls"`
	MessageBody     string   `name:"message"` //shortcode only
	CreatedDatetime string   `name:"createdDatetime"`
	ReceiveDatetime string   `name:"receive_datetime"` //shortcode only
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
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
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
		urn, err := urns.ParsePhone(receivedStatus.Recipient, "", true, false)
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

func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	err := h.validateSignature(channel, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	payload := &formMessage{}
	err = handlers.DecodeAndValidateForm(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	text := ""
	messageID := ""
	date := time.Time{}
	//chechk if shortcode or regular
	if payload.Shortcode != "" {
		text = payload.MessageBody
		messageID = payload.MID
		shortCodeDateLayout := "20060102150405"
		date, err = time.Parse(shortCodeDateLayout, payload.ReceiveDatetime)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unable to parse date '%s': %v", payload.ReceiveDatetime, err))
		}
	} else {
		text = payload.Body
		messageID = payload.ID
		standardDateLayout := "2006-01-02T15:04:05+00:00"
		date, err = time.Parse(standardDateLayout, payload.CreatedDatetime)
		if err != nil {
			return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unable to parse date '%s': %v", payload.CreatedDatetime, err))
		}
	}

	// no message? ignore this
	if text == "" && len(payload.MediaURLs) == 0 {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, errors.New("no text or media"))
	}

	// create our URN
	urn, err := urns.ParsePhone(payload.Originator, channel.Country(), true, false)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(ctx, channel, urn, text, messageID, clog).WithReceivedOn(date.UTC())

	// process any attached media
	if len(payload.MediaURLs) > 0 {
		for _, mediaURL := range payload.MediaURLs {
			msg.WithAttachment(mediaURL)
		}
	}
	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
}

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	authToken := msg.Channel().StringConfigForKey(courier.ConfigAuthToken, "")
	if authToken == "" {
		return courier.ErrChannelConfig
	}

	user := msg.URN().Path()
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
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	var bearer = "AccessKey " + authToken
	req.Header.Set("Authorization", bearer)

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 == 5 {
		return courier.ErrConnectionFailed
	} else if resp.StatusCode/100 != 2 {
		return courier.ErrResponseStatus
	}

	externalID, err := jsonparser.GetString(respBody, "id")
	if err != nil {
		clog.Error(courier.ErrorResponseValueMissing("id"))
	} else {
		res.AddExternalID(externalID)
	}

	return nil
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

// WriteMsgSuccessResponse writes a success response for the messages, MB expects an 'OK' body in our response
func (h *handler) WriteMsgSuccessResponse(ctx context.Context, w http.ResponseWriter, msgs []courier.MsgIn) error {
	w.Header().Add("Content-type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte("OK"))
	return err
}
