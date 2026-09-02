package twiml

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/web"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/core/events"
	"github.com/stretchr/testify/assert"
)

func addValidSignature(r *http.Request) {
	r.ParseForm()
	sig, _ := twCalculateSignature(fmt.Sprintf("%s://%s%s", r.URL.Scheme, r.Host, r.URL.RequestURI()), r.PostForm, "6789")
	r.Header.Set(signatureHeader, string(sig))
}

// signs over the path the request was forwarded from rather than the path it arrived on
func addForwardSignature(r *http.Request) {
	r.ParseForm()
	path := r.Header.Get(forwardedPathHeader)
	sig, _ := twCalculateSignature(fmt.Sprintf("%s://%s%s", r.URL.Scheme, r.Host, path), r.PostForm, "6789")
	r.Header.Set(signatureHeader, string(sig))
}

func TestIncoming(t *testing.T) {
	signed := &IncomingOptions{Sign: addValidSignature}

	phoneChannel := func(ctype string) []*models.Channel {
		return []*models.Channel{
			test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", ctype, "2020", "US", []string{urns.Phone.Prefix}, map[string]any{"auth_token": "6789"}),
		}
	}
	whatsappChannel := func(ctype string) []*models.Channel {
		return []*models.Channel{
			test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", ctype, "+12065551212", "US", []string{urns.WhatsApp.Prefix}, map[string]any{
				configAccountSID:       "accountSID",
				models.ConfigAuthToken: "6789",
			}),
		}
	}

	RunIncomingTests(t, phoneChannel("T"), newTWIMLHandler("T", "Twilio", true), "testdata/t_incoming.json", signed)
	RunIncomingTests(t, phoneChannel("TMS"), newTWIMLHandler("TMS", "Twilio Messaging Service", true), "testdata/tms_incoming.json", signed)
	RunIncomingTests(t, phoneChannel("TW"), newTWIMLHandler("TW", "TwiML API", true), "testdata/tw_incoming.json", signed)
	RunIncomingTests(t, phoneChannel("TW"), newTWIMLHandler("TW", "TwiML API", true), "testdata/tw_incoming_forwarded.json", &IncomingOptions{Sign: addForwardSignature})
	RunIncomingTests(t, phoneChannel("SW"), newTWIMLHandler("SW", "SignalWire", false), "testdata/sw_incoming.json", nil)
	RunIncomingTests(t, whatsappChannel("T"), newTWIMLHandler("T", "TwilioWhatsApp", true), "testdata/wa_incoming.json", signed)
	RunIncomingTests(t, whatsappChannel("TWA"), newTWIMLHandler("TWA", "Twilio WhatsApp", true), "testdata/twa_incoming.json", signed)
}

func TestOutgoing(t *testing.T) {
	maxMsgLength = 160
	opts := &OutgoingOptions{CheckRedacted: []string{httpx.BasicAuth("accountSID", "authToken")}}

	defaultChannel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "T", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			configAccountSID:       "accountSID",
			models.ConfigAuthToken: "authToken"})
	RunOutgoingTests(t, defaultChannel, newTWIMLHandler("T", "Twilio", true), "testdata/t_outgoing.json", opts)

	tmsChannel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56cd", "TMS", "", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			configMessagingServiceSID: "messageServiceSID",
			configAccountSID:          "accountSID",
			models.ConfigAuthToken:    "authToken"})
	RunOutgoingTests(t, tmsChannel, newTWIMLHandler("TMS", "Twilio Messaging Service", true), "testdata/tms_outgoing.json", opts)

	tmsShortenLinksChannel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56cd", "TMS", "", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			configLinkShortening:      true,
			configMessagingServiceSID: "messageServiceSID",
			configAccountSID:          "accountSID",
			models.ConfigAuthToken:    "authToken"})
	RunOutgoingTests(t, tmsShortenLinksChannel, newTWIMLHandler("TMS", "Twilio Messaging Service", true), "testdata/tms_outgoing_shorten_links.json", opts)

	twChannel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TW", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			configAccountSID:       "accountSID",
			models.ConfigAuthToken: "authToken",
			configSendURL:          "http://example.com/twiml_api/",
		})
	RunOutgoingTests(t, twChannel, newTWIMLHandler("TW", "TwiML", true), "testdata/tw_outgoing.json", opts)

	swChannel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SW", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			configAccountSID:       "accountSID",
			models.ConfigAuthToken: "authToken",
			configSendURL:          "http://example.com/sigware_api/",
		})
	RunOutgoingTests(t, swChannel, newTWIMLHandler("SW", "SignalWire", false), "testdata/sw_outgoing.json", opts)

	waChannel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SW", "+12065551212", "US",
		[]string{urns.WhatsApp.Prefix},
		map[string]any{
			configAccountSID:       "accountSID",
			models.ConfigAuthToken: "authToken",
			configSendURL:          "http://example.com/sigware_api/",
		},
	)
	RunOutgoingTests(t, waChannel, newTWIMLHandler("T", "Twilio Whatsapp", true), "testdata/wa_outgoing.json", opts)

	twaChannel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TWA", "+12065551212", "US",
		[]string{urns.WhatsApp.Prefix},
		map[string]any{
			configAccountSID:          "accountSID",
			models.ConfigAuthToken:    "authToken",
			configMessagingServiceSID: "messageServiceSID",
		},
	)
	RunOutgoingTests(t, twaChannel, newTWIMLHandler("TWA", "Twilio Whatsapp", true), "testdata/twa_outgoing.json", opts)
}

func TestBuildAttachmentRequest(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "T", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			configAccountSID:       "accountSID",
			models.ConfigAuthToken: "authToken"})

	twHandler := newTWIMLHandler("T", "Twilio", true)(nil, channels.NewRoutes()).(*handler)
	req, _ := twHandler.BuildAttachmentRequest(context.Background(), defaultChannel, "https://example.org/v1/media/41", nil)
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, "Basic YWNjb3VudFNJRDphdXRoVG9rZW4=", req.Header.Get("Authorization"))

	var swChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SW", "2020", "US",
		[]string{urns.WhatsApp.Prefix},
		map[string]any{
			configAccountSID:       "accountSID",
			models.ConfigAuthToken: "authToken",
			configSendURL:          "BASE_URL",
		})
	swHandler := newTWIMLHandler("SW", "SignalWire", false)(nil, channels.NewRoutes()).(*handler)
	req, _ = swHandler.BuildAttachmentRequest(context.Background(), swChannel, "https://example.org/v1/media/41", nil)
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, "", req.Header.Get("Authorization"))
}

func TestSendEvent(t *testing.T) {
	channel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TWA", "+12065551212", "US",
		[]string{urns.WhatsApp.Prefix},
		map[string]any{
			configAccountSID:       "accountSID",
			models.ConfigAuthToken: "authToken",
		},
	)

	s := web.NewServer(runtime.NewTestRuntime(runtime.NewDefaultConfig()))

	h := s.MountHandler(newTWIMLHandler("TWA", "Twilio Whatsapp", true)).(*handler)

	s.Runtime().HTTP.Default.Transport = test.MockTransport(map[string][]*httpx.MockResponse{
		"https://messaging.twilio.com/v3/Indicators/Typing.json": {
			httpx.NewMockResponse(200, nil, []byte(`{"success": true}`)),
			httpx.NewMockResponse(400, nil, []byte(`{"code": 21211, "message": "Invalid message SID"}`)),
			httpx.MockConnectionError,
		},
	})

	// typing indicators are supported on Twilio WhatsApp channels but no other TWIML channel types
	assert.Equal(t, map[string]time.Duration{events.TypeTypingStarted: 20 * time.Second}, h.SendableEvents(channel))
	assert.Nil(t, newTWIMLHandler("T", "Twilio", true)(nil, channels.NewRoutes()).(*handler).SendableEvents(channel))

	channelRef := assets.NewChannelReference("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "Twilio Whatsapp")
	typing := events.NewTypingStarted(events.DirectionOutgoing, channelRef, "whatsapp:12065551212", "SMabcdef1234567890abcdef1234567890")

	// a typing indicator is sent as a typing indicators resource call referencing the incoming message
	clog := models.NewChannelLogForEventSend(channel, nil)
	err := h.SendEvent(context.Background(), channel, typing, clog)
	assert.NoError(t, err)
	assert.Len(t, clog.HttpLogs, 1)
	assert.Equal(t, "https://messaging.twilio.com/v3/Indicators/Typing.json", clog.HttpLogs[0].URL)
	assert.Contains(t, clog.HttpLogs[0].Request, `{"channel":"WHATSAPP","messageId":"SMabcdef1234567890abcdef1234567890"}`)

	// an error response is a response error
	err = h.SendEvent(context.Background(), channel, typing, clog)
	assert.Equal(t, channels.ErrResponseStatus, err)

	// as is a connection error
	err = h.SendEvent(context.Background(), channel, typing, clog)
	assert.Equal(t, channels.ErrConnectionFailed, err)

	// an event without a msg external ID can't be sent
	err = h.SendEvent(context.Background(), channel, events.NewTypingStarted(events.DirectionOutgoing, channelRef, "whatsapp:12065551212", ""), clog)
	assert.ErrorContains(t, err, "requires msg_external_id")

	// a channel without complete auth config can't send
	noAuth := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TWA", "+12065551212", "US", []string{urns.WhatsApp.Prefix}, map[string]any{})
	err = h.SendEvent(context.Background(), noAuth, typing, clog)
	assert.Equal(t, channels.ErrChannelConfig, err)

	// nor can an event type the handler doesn't declare support for
	err = h.SendEvent(context.Background(), channel, events.NewTypingStopped(events.DirectionOutgoing, channelRef, "whatsapp:12065551212", ""), clog)
	assert.ErrorContains(t, err, "unsupported event type: typing_stopped")
}
