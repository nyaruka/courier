package whatsappcloudapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
)

var (
	signatureHeader = "X-Hub-Signature"

	graphURL = "https://graph.facebook.com/v13.0/"
)

func newHandler(channelType courier.ChannelType, name string, useUUIDRoutes bool) courier.ChannelHandler {
	return &handler{handlers.NewBaseHandlerWithParams(channelType, name, useUUIDRoutes)}
}

func init() {
	courier.RegisterHandler(newHandler("CWA", "Cloud API WhatsApp", false))
}

type handler struct {
	handlers.BaseHandler
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodGet, "receive", h.receiveVerify)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveEvent)
	return nil
}

var waStatusMapping = map[string]courier.MsgStatusValue{
	"sent":      courier.MsgSent,
	"delivered": courier.MsgDelivered,
	"read":      courier.MsgDelivered,
	"failed":    courier.MsgFailed,
}

var waIgnoreStatuses = map[string]bool{
	"deleted": true,
}

// components as seen on https://developers.facebook.com/docs/whatsapp/cloud-api/webhooks/components
type moPayload struct {
	Object string     `json:"object"`
	Entry  []cwaEntry `json:"entry"`
}

type cwaEntry struct {
	ID      string      `json:"id"`
	Changes []cwaChange `json:"changes"`
}

type cwaChange struct {
	Field string   `json:"field"`
	Value cwaValue `json:"value"`
}

type cwaValue struct {
	MessagingProduct string       `json:"messaging_product"`
	Metadata         cwaMetadata  `json:"metadata"`
	Contacts         []cwaContact `json:"contacts"`
	Messages         []cwaMessage `json:"messages"`
	Statuses         []cwaStatus  `json:"statuses"`
	Errors           []cwaError   `json:"errors"`
}

type cwaMetadata struct {
	DisplayPhoneNumber string `json:"display_phone_number"`
	PhoneNumberID      string `json:"phone_number_id"`
}

type cwaContact struct {
	Profile struct {
		Name string `json:"name"`
	} `json:"profile"`
	WaID string `json:"wa_id"`
}

type cwaMessage struct {
	ID        string      `json:"id"`
	From      string      `json:"from"`
	Timestamp string      `json:"timestamp"`
	Type      string      `json:"type"`
	Context   *cwaContext `json:"context"`
	Text      struct {
		Body string `json:"body"`
	} `json:"text"`
	Image    *cwaMedia    `json:"image"`
	Audio    *cwaMedia    `json:"audio"`
	Video    *cwaMedia    `json:"video"`
	Document *cwaMedia    `json:"document"`
	Voice    *cwaMedia    `json:"voice"`
	Location *cwaLocation `json:"location"`
	Button   *cwaButton   `json:"button"`
}

type cwaContext struct {
	Forwarded           bool   `json:"forwarded"`
	FrequentlyForwarded bool   `json:"frequently_forwarded"`
	From                string `json:"from"`
	ID                  string `json:"id"`
}

type cwaStatus struct {
	ID           string           `json:"id"`
	RecipientID  string           `json:"recipient_id"`
	Status       string           `json:"status"`
	Timestamp    string           `json:"timestamp"`
	Type         string           `json:"type"`
	Conversation *cwaConversation `json:"conversation"`
	Pricing      *cwaPricing      `json:"pricing"`
}

type cwaMedia struct {
	Caption  string `json:"caption"`
	Filename string `json:"filename"`
	ID       string `json:"id"`
	Mimetype string `json:"mime_type"`
	SHA256   string `json:"sha256"`
}

type cwaButton struct {
	Text    string `json:"text"`
	Payload string `json:"payload"`
}

type cwaLocation struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Name      string  `json:"name"`
	Address   string  `json:"address"`
}

type cwaError struct {
	Code  int    `json:"code"`
	Title string `json:"title"`
}

type cwaPricing struct {
	PricingModel string `json:"pricing_model"`
	Billable     bool   `json:"billable"`
	Category     string `json:"category"`
}

type cwaOrigin struct {
	Type string `json:"type"`
}

type cwaConversation struct {
	ID                  string     `json:"id"`
	Origin              *cwaOrigin `json:"origin"`
	ExpirationTimestamp int64      `json:"expiration_timestamp"`
}

type otherMoPayload struct {
	Object string `json:"object"`
	Entry  []struct {
		ID      string `json:"id"`
		Changes []struct {
			Field string `json:"field"`
			Value struct {
				MessagingProduct string `json:"messaging_product"`
				Metadata         *struct {
					DisplayPhoneNumber string `json:"display_phone_number"`
					PhoneNumberID      string `json:"phone_number_id"`
				} `json:"metadata"`
				Contacts []struct {
					Profile struct {
						Name string `json:"name"`
					} `json:"profile"`
					WaID string `json:"wa_id"`
				} `json:"contacts"`
				Messages []struct {
					ID        string `json:"id"`
					From      string `json:"from"`
					Timestamp string `json:"timestamp"`
					Type      string `json:"type"`
					Context   *struct {
						Forwarded           bool   `json:"forwarded"`
						FrequentlyForwarded bool   `json:"frequently_forwarded"`
						From                string `json:"from"`
						ID                  string `json:"id"`
					} `json:"context"`
					Text struct {
						Body string `json:"body"`
					} `json:"text"`
					Image    *cwaMedia `json:"image"`
					Audio    *cwaMedia `json:"audio"`
					Video    *cwaMedia `json:"video"`
					Document *cwaMedia `json:"document"`
					Voice    *cwaMedia `json:"voice"`
					Location *struct {
						Latitude  float64 `json:"latitude"`
						Longitude float64 `json:"longitude"`
						Name      string  `json:"name"`
						Address   string  `json:"address"`
					} `json:"location"`
					Button *struct {
						Text    string `json:"text"`
						Payload string `json:"payload"`
					} `json:"button"`
				} `json:"messages"`
				Statuses []struct {
					ID           string `json:"id"`
					RecipientID  string `json:"recipient_id"`
					Status       string `json:"status"`
					Timestamp    string `json:"timestamp"`
					Type         string `json:"type"`
					Conversation *struct {
						ID     string `json:"id"`
						Origin *struct {
							Type string `json:"type"`
						} `json:"origin"`
						ExpirationTimestamp int64 `json:"expiration_timestamp"`
					} `json:"conversation"`
					Pricing *struct {
						PricingModel string `json:"pricing_model"`
						Billable     bool   `json:"billable"`
						Category     string `json:"category"`
					} `json:"pricing"`
				} `json:"statuses"`
				Errors []struct {
					Code  int    `json:"code"`
					Title string `json:"title"`
				} `json:"errors"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

// GetChannel returns the channel
func (h *handler) GetChannel(ctx context.Context, r *http.Request) (courier.Channel, error) {

	if r.Method == http.MethodGet {
		return nil, nil
	}

	payload := &moPayload{}
	err := handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, err
	}
	if payload.Object != "whatsapp_business_account" {
		return nil, fmt.Errorf("object expected 'whatsapp_business_account', found %s", payload.Object)
	}
	// no entries? ignore this request
	if len(payload.Entry) == 0 {
		return nil, fmt.Errorf("no entries found")
	}

	if len(payload.Entry[0].Changes) == 0 {
		return nil, fmt.Errorf("no changes found")
	}

	channelAddress := payload.Entry[0].Changes[0].Value.Metadata.DisplayPhoneNumber
	if channelAddress == "" {
		return nil, fmt.Errorf("no channel adress found")
	}

	return h.Backend().GetChannelByAddress(ctx, courier.ChannelType("CWA"), courier.ChannelAddress(channelAddress))
}

// receiveVerify handles Facebook's webhook verification callback
func (h *handler) receiveVerify(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	mode := r.URL.Query().Get("hub.mode")

	// this isn't a subscribe verification, that's an error
	if mode != "subscribe" {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unknown request"))
	}

	// verify the token against our server facebook webhook secret, if the same return the challenge FB sent us
	secret := r.URL.Query().Get("hub.verify_token")
	if secret != h.Server().Config().FacebookWebhookSecret {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("token does not match secret"))
	}
	// and respond with the challenge token
	_, err := fmt.Fprint(w, r.URL.Query().Get("hub.challenge"))
	return nil, err
}

// receiveEvent is our HTTP handler function for incoming messages and status updates
func (h *handler) receiveEvent(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	err := h.validateSignature(r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	payload := &moPayload{}
	err = handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// is not a 'whatsapp_business_account' object? ignore it
	if payload.Object != "whatsapp_business_account" {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring request")
	}

	// no entries? ignore this request
	if len(payload.Entry) == 0 {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "ignoring request, no entries")
	}

	// the list of events we deal with
	events := make([]courier.Event, 0, 2)

	// the list of data we will return in our response
	data := make([]interface{}, 0, 2)

	var contactNames = make(map[string]string)

	// for each entry
	for _, entry := range payload.Entry {
		if len(entry.Changes) == 0 {
			continue
		}

		for _, change := range entry.Changes {

			for _, contact := range change.Value.Contacts {
				contactNames[contact.WaID] = contact.Profile.Name
			}

			for _, msg := range change.Value.Messages {
				// create our date from the timestamp
				ts, err := strconv.ParseInt(msg.Timestamp, 10, 64)
				if err != nil {
					return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("invalid timestamp: %s", msg.Timestamp))
				}
				date := time.Unix(ts, 0).UTC()

				urn, err := urns.NewWhatsAppURN(msg.From)
				if err != nil {
					return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
				}

				text := ""
				mediaURL := ""

				if msg.Type == "text" {
					text = msg.Text.Body
				} else if msg.Type == "audio" && msg.Audio != nil {
					text = msg.Audio.Caption
					mediaURL, err = resolveMediaURL(channel, msg.Audio.ID)
				} else if msg.Type == "voice" && msg.Voice != nil {
					text = msg.Voice.Caption
					mediaURL, err = resolveMediaURL(channel, msg.Voice.ID)
				} else if msg.Type == "button" && msg.Button != nil {
					text = msg.Button.Text
				} else if msg.Type == "document" && msg.Document != nil {
					text = msg.Document.Caption
					mediaURL, err = resolveMediaURL(channel, msg.Document.ID)
				} else if msg.Type == "image" && msg.Image != nil {
					text = msg.Image.Caption
					mediaURL, err = resolveMediaURL(channel, msg.Image.ID)
				} else if msg.Type == "video" && msg.Video != nil {
					text = msg.Video.Caption
					mediaURL, err = resolveMediaURL(channel, msg.Video.ID)
				} else if msg.Type == "location" && msg.Location != nil {
					mediaURL = fmt.Sprintf("geo:%f,%f", msg.Location.Latitude, msg.Location.Longitude)
				} else {
					// we received a message type we do not support.
					courier.LogRequestError(r, channel, fmt.Errorf("unsupported message type %s", msg.Type))
				}

				// create our message
				ev := h.Backend().NewIncomingMsg(channel, urn, text).WithReceivedOn(date).WithExternalID(msg.ID).WithContactName(contactNames[msg.From])
				event := h.Backend().CheckExternalIDSeen(ev)

				// we had an error downloading media
				if err != nil {
					courier.LogRequestError(r, channel, err)
				}

				if mediaURL != "" {
					event.WithAttachment(mediaURL)
				}

				err = h.Backend().WriteMsg(ctx, event)
				if err != nil {
					return nil, err
				}

				h.Backend().WriteExternalIDSeen(event)

				events = append(events, event)
				data = append(data, courier.NewMsgReceiveData(event))

			}

			for _, status := range change.Value.Statuses {

				msgStatus, found := waStatusMapping[status.Status]
				if !found {
					if waIgnoreStatuses[status.Status] {
						data = append(data, courier.NewInfoData(fmt.Sprintf("ignoring status: %s", status.Status)))
					} else {
						handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("unknown status: %s", status.Status))
					}
					continue
				}

				event := h.Backend().NewMsgStatusForExternalID(channel, status.ID, msgStatus)
				err := h.Backend().WriteMsgStatus(ctx, event)

				// we don't know about this message, just tell them we ignored it
				if err == courier.ErrMsgNotFound {
					data = append(data, courier.NewInfoData(fmt.Sprintf("message id: %s not found, ignored", status.ID)))
					continue
				}

				if err != nil {
					return nil, err
				}

				events = append(events, event)
				data = append(data, courier.NewStatusData(event))

			}

		}

	}

	return events, courier.WriteDataResponse(ctx, w, http.StatusOK, "Events Handled", data)
}

func setWhatsAppAuthHeader(header *http.Header, channel courier.Channel) {
	authToken := channel.StringConfigForKey(courier.ConfigAuthToken, "")
	header.Set("Authorization", fmt.Sprintf("Bearer %s", authToken))
}

func resolveMediaURL(channel courier.Channel, mediaID string) (string, error) {
	token := channel.StringConfigForKey(courier.ConfigAuthToken, "")
	if token == "" {
		return "", fmt.Errorf("missing token for WA channel")
	}

	base, _ := url.Parse(graphURL)
	path, _ := url.Parse(fmt.Sprintf("/%s", mediaID))
	retreiveURL := base.ResolveReference(path)

	// set the access token as the authorization header
	req, _ := http.NewRequest(http.MethodGet, retreiveURL.String(), nil)
	//req.Header.Set("User-Agent", utils.HTTPUserAgent)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := utils.MakeHTTPRequest(req)
	if err != nil {
		return "", err
	}

	mediaURL, err := jsonparser.GetString(resp.Body, "url")
	return mediaURL, err
}

// see https://developers.facebook.com/docs/graph-api/webhooks/getting-started/#verification-requests
func (h *handler) validateSignature(r *http.Request) error {
	headerSignature := r.Header.Get(signatureHeader)
	if headerSignature == "" {
		return fmt.Errorf("missing request signature")
	}
	appSecret := h.Server().Config().FacebookApplicationSecret

	body, err := handlers.ReadBody(r, 100000)
	if err != nil {
		return fmt.Errorf("unable to read request body: %s", err)
	}

	expectedSignature, err := fbCalculateSignature(appSecret, body)
	if err != nil {
		return err
	}

	signature := ""
	if len(headerSignature) == 45 && strings.HasPrefix(headerSignature, "sha1=") {
		signature = strings.TrimPrefix(headerSignature, "sha1=")
	}

	// compare signatures in way that isn't sensitive to a timing attack
	if !hmac.Equal([]byte(expectedSignature), []byte(signature)) {
		return fmt.Errorf("invalid request signature, expected: %s got: %s for body: '%s'", expectedSignature, signature, string(body))
	}

	return nil
}

func fbCalculateSignature(appSecret string, body []byte) (string, error) {
	var buffer bytes.Buffer
	buffer.Write(body)

	// hash with SHA1
	mac := hmac.New(sha1.New, []byte(appSecret))
	mac.Write(buffer.Bytes())

	return hex.EncodeToString(mac.Sum(nil)), nil
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	return nil, nil
}
