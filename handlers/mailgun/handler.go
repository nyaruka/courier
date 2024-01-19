package mailgun

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

const (
	configDefaultSubject = "default_subject"
	configSigningKey     = "signing_key"
)

var (
	defaultAPIURL = "https://api.mailgun.net/v3"
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("MLG"), "Mailgun")}
}

func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, h.receive)
	return nil
}

type receiveRequest struct {
	Recipient       string `name:"recipient"     validate:"required"` // can be multiple addresses
	Sender          string `name:"sender"        validate:"required,email"`
	From            string `name:"From"`
	ReplyTo         string `name:"Reply-To"`
	MessageID       string `name:"Message-Id"    validate:"required"`
	Subject         string `name:"subject"       validate:"required"`
	PlainBody       string `name:"body-plain"`
	StrippedText    string `name:"stripped-text" validate:"required"`
	HTMLBody        string `name:"body-html"`
	Timestamp       string `name:"timestamp"     validate:"required"`
	Token           string `name:"token"         validate:"required"`
	Signature       string `name:"signature"     validate:"required"`
	AttachmentCount int    `name:"attachment-count"`
}

// see https://documentation.mailgun.com/en/latest/user_manual.html#securing-webhooks
func (r *receiveRequest) verify(signingKey string) bool {
	v := r.Timestamp + r.Token

	mac := hmac.New(sha256.New, []byte(signingKey))
	mac.Write([]byte(v))
	expectedMAC := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(r.Signature), []byte(expectedMAC))
}

// WriteRequestError writes the passed in error to our response writer
func (h *handler) WriteRequestError(ctx context.Context, w http.ResponseWriter, err error) error {
	return courier.WriteError(w, http.StatusNotAcceptable, err)
}

func (h *handler) receive(ctx context.Context, c courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	signingKey := c.StringConfigForKey(configSigningKey, "")
	if signingKey == "" {
		return nil, fmt.Errorf("missing signing key for %s channel", h.ChannelName())
	}

	request := &receiveRequest{}
	if err := handlers.DecodeAndValidateForm(request, r); err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
	}

	if !request.verify(signingKey) {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, errors.New("signature validation failed"))
	}

	// if the channel address isn't in the recipients then this email has probably been mis-routed
	if !strings.Contains(request.Recipient, c.Address()) {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, errors.New("invalid recipient"))
	}

	urn, err := urns.NewURNFromParts(urns.EmailScheme, request.Sender, "", "")
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
	}

	msg := h.Backend().NewIncomingMsg(c, urn, request.StrippedText, "", clog)

	// TODO attachments

	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
}

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	address := msg.Channel().Address()
	domain := strings.SplitN(address, "@", 2)[1]

	sendURL := fmt.Sprintf("%s/%s/messages", defaultAPIURL, domain)

	sendingKey := msg.Channel().StringConfigForKey(courier.ConfigAuthToken, "")
	if sendingKey == "" {
		return nil, fmt.Errorf("missing sending key for %s channel", h.ChannelName())
	}

	subject := msg.Channel().StringConfigForKey(configDefaultSubject, "Chat with TextIt")

	b := &bytes.Buffer{}
	w := multipart.NewWriter(b)
	w.WriteField("from", address)
	w.WriteField("to", msg.URN().Path())
	w.WriteField("subject", subject)
	w.WriteField("text", msg.Text())

	// TODO add attachments

	w.Close()

	req, _ := http.NewRequest("POST", sendURL, b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.SetBasicAuth("api", sendingKey)

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusWired, clog)

	resp, respBody, err := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 != 2 {
		status.SetStatus(courier.MsgStatusErrored)
	} else {
		id, _ := jsonparser.GetString(respBody, "id")
		status.SetExternalID(id)
	}

	return status, nil
}

func (h *handler) RedactValues(ch courier.Channel) []string {
	return []string{
		httpx.BasicAuth("api", ch.StringConfigForKey(courier.ConfigAuthToken, "")),
	}
}
