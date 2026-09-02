package meta

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers"
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

// note that the address here can't be a channel address in the test database seed data because these channels are
// routed to by address rather than UUID
var facebookTestChannels = []*models.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "FBA", "1234567890", "", []string{urns.Facebook.Prefix}, map[string]any{models.ConfigAuthToken: "a123"}),
}

func TestFacebookIncoming(t *testing.T) {
	RunIncomingTests(t, facebookTestChannels, newHandler("FBA", "Facebook"), "testdata/facebook_incoming.json", &IncomingOptions{Sign: addValidSignature, NoInvalidChannelCheck: true})
}

func TestFacebookDescribeURN(t *testing.T) {
	rt := runtime.NewTestRuntime(runtime.NewDefaultConfig())
	rt.HTTP.Default.Transport = test.MockTransport(map[string][]*httpx.MockResponse{
		"https://graph.facebook.com/1337?access_token=a123&fields=first_name%2Clast_name": {
			httpx.NewMockResponse(200, nil, []byte(`{"first_name": "John", "last_name": "Doe"}`)),
		},
		"https://graph.facebook.com/4567?access_token=a123&fields=first_name%2Clast_name": {
			httpx.NewMockResponse(200, nil, []byte(`{"first_name": "", "last_name": ""}`)),
		},
	})

	channel := facebookTestChannels[0]
	handler := web.NewServer(rt).MountHandler(newHandler("FBA", "Facebook"))
	clog := models.NewChannelLog(models.ChannelLogTypeUnknown, channel, nil, handler.RedactValues(channel))

	tcs := []struct {
		urn              urns.URN
		expectedMetadata map[string]string
	}{
		{"facebook:1337", map[string]string{"name": "John Doe"}},
		{"facebook:4567", map[string]string{"name": ""}},
	}

	for _, tc := range tcs {
		metadata, _ := handler.(models.URNDescriber).DescribeURN(context.Background(), channel, tc.urn, clog)
		assert.Equal(t, metadata, tc.expectedMetadata)
	}

	AssertChannelLogRedaction(t, clog, []string{"a123", "wac_admin_system_user_token"})
}

func TestFacebookOutgoing(t *testing.T) {
	// shorter max msg length for testing
	maxMsgLength = 100

	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "FBA", "12345", "", []string{urns.Facebook.Prefix}, map[string]any{models.ConfigAuthToken: "a123"})

	checkRedacted := []string{"wac_admin_system_user_token", "missing_facebook_app_secret", "missing_facebook_webhook_secret", "a123"}

	RunOutgoingTests(t, ch, newHandler("FBA", "Facebook"), "testdata/facebook_outgoing.json", &OutgoingOptions{CheckRedacted: checkRedacted})
}

func TestFacebookSendEvent(t *testing.T) {
	channel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "FBA", "12345", "", []string{urns.Facebook.Prefix}, map[string]any{models.ConfigAuthToken: "a123"})

	s := web.NewServer(runtime.NewTestRuntime(runtime.NewDefaultConfig()))

	h := s.MountHandler(newHandler("FBA", "Facebook")).(*handler)

	s.Runtime().HTTP.Default.Transport = test.MockTransport(map[string][]*httpx.MockResponse{
		"https://graph.facebook.com/v25.0/me/messages*": {
			httpx.NewMockResponse(200, nil, []byte(`{"recipient_id": "5678"}`)),
			httpx.NewMockResponse(200, nil, []byte(`{"recipient_id": "5678"}`)),
			httpx.NewMockResponse(400, nil, []byte(`{"error": {"message": "Invalid user", "code": 100}}`)),
			httpx.MockConnectionError,
		},
	})

	// typing events are supported on both Facebook and Instagram channels, including explicit stop
	expected := map[string]time.Duration{events.TypeTypingStarted: 15 * time.Second, events.TypeTypingStopped: 0}
	assert.Equal(t, expected, h.SendableEvents(channel))
	assert.Equal(t, expected, newHandler("IG", "Instagram")(nil, channels.NewRoutes()).(*handler).SendableEvents(channel))

	channelRef := assets.NewChannelReference("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "Facebook")

	// a typing started event is sent as a typing_on sender action
	clog := models.NewChannelLogForEventSend(channel, nil)
	err := h.SendEvent(context.Background(), channel, events.NewTypingStarted(events.DirectionOutgoing, channelRef, "facebook:5678", ""), clog)
	assert.NoError(t, err)
	assert.Len(t, clog.HttpLogs, 1)
	assert.Equal(t, "https://graph.facebook.com/v25.0/me/messages?access_token=a123", clog.HttpLogs[0].URL)
	assert.Contains(t, clog.HttpLogs[0].Request, `{"recipient":{"id":"5678"},"sender_action":"typing_on"}`)

	// and a typing stopped event as a typing_off sender action
	err = h.SendEvent(context.Background(), channel, events.NewTypingStopped(events.DirectionOutgoing, channelRef, "facebook:5678", ""), clog)
	assert.NoError(t, err)
	assert.Len(t, clog.HttpLogs, 2)
	assert.Contains(t, clog.HttpLogs[1].Request, `{"recipient":{"id":"5678"},"sender_action":"typing_off"}`)

	// an error response is a response error
	err = h.SendEvent(context.Background(), channel, events.NewTypingStarted(events.DirectionOutgoing, channelRef, "facebook:5678", ""), clog)
	assert.Equal(t, channels.ErrResponseStatus, err)

	// as is a connection error
	err = h.SendEvent(context.Background(), channel, events.NewTypingStarted(events.DirectionOutgoing, channelRef, "facebook:5678", ""), clog)
	assert.Equal(t, channels.ErrConnectionFailed, err)

	// a channel without an auth token config can't send
	noAuth := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "FBA", "12345", "", []string{urns.Facebook.Prefix}, nil)
	err = h.SendEvent(context.Background(), noAuth, events.NewTypingStarted(events.DirectionOutgoing, channelRef, "facebook:5678", ""), clog)
	assert.Equal(t, channels.ErrChannelConfig, err)

	// nor can an event type the handler doesn't know how to send
	err = h.SendEvent(context.Background(), channel, events.NewContactLanguageChanged("eng"), clog)
	assert.ErrorContains(t, err, "unsupported event type: contact_language_changed")
}

func TestSigning(t *testing.T) {
	tcs := []struct {
		Body      string
		Signature string
	}{
		{
			"hello world",
			"f39034b29165ec6a5104d9aef27266484ab26c8caa7bca8bcb2dd02e8be61b17",
		},
		{
			"hello world2",
			"60905fdf409d0b4f721e99f6f25b31567a68a6b45e933d814e17a246be4c5a53",
		},
	}

	for i, tc := range tcs {
		sig, err := fbCalculateSignature("sesame", []byte(tc.Body))
		assert.NoError(t, err)
		assert.Equal(t, tc.Signature, sig, "%d: mismatched signature", i)
	}
}

func TestFacebookBuildAttachmentRequest(t *testing.T) {
	s := web.NewServer(runtime.NewTestRuntime(runtime.NewDefaultConfig()))

	handler := s.MountHandler(newHandler("FBA", "Facebook")).(*handler)
	req, _ := handler.BuildAttachmentRequest(context.Background(), facebookTestChannels[0], "https://example.org/v1/media/41", nil)
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, http.Header{}, req.Header)
}

func addValidSignature(r *http.Request) {
	body, _ := ReadBody(r, maxRequestBodyBytes)
	sig, _ := fbCalculateSignature("fb_app_secret", body)
	r.Header.Set(signatureHeader, fmt.Sprintf("sha256=%s", string(sig)))
}
