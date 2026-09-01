package line

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/courier/v26/web"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/core/events"
	"github.com/stretchr/testify/assert"
)

var testChannels = []*models.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "LN", "2020", "US",
		[]string{urns.Line.Prefix},
		map[string]any{
			"secret":     "Secret",
			"auth_token": "the-auth-token",
		}),
}

func addValidSignature(r *http.Request) {
	sig, _ := calculateSignature("Secret", r)
	r.Header.Set(signatureHeader, string(sig))
}

func setupMedia(t *testing.T, rt *runtime.Runtime) {
	rt.Config.MediaDomain = "mock.com"

	imageJPG := test.NewMockMedia("ec6972be-809c-4c8d-be59-ba9dbd74c977", "test.jpg", "image/jpeg", "http://mock.com/media/ec6972be-809c-4c8d-be59-ba9dbd74c977/test.jpg", 1024*1024, 640, 480, 0, nil)

	audioM4A := test.NewMockMedia("d8f6d8bb-9dd0-4b34-98b8-f2e9e857f2b6", "test.m4a", "audio/mp4", "http://mock.com/media/d8f6d8bb-9dd0-4b34-98b8-f2e9e857f2b6/test.m4a", 1024*1024, 0, 0, 200, nil)
	audioMP3 := test.NewMockMedia("9a4c4415-a06c-4edd-ad5b-33ed0be6b306", "test.mp3", "audio/mp3", "http://mock.com/media/9a4c4415-a06c-4edd-ad5b-33ed0be6b306/test.mp3", 1024*1024, 0, 0, 200, []*models.Media{audioM4A})

	thumbJPG := test.NewMockMedia("2f8db4b2-e21c-4fe4-a049-4dbcecf83cf6", "test.jpg", "image/jpeg", "http://mock.com/media/2f8db4b2-e21c-4fe4-a049-4dbcecf83cf6/test.jpg", 1024*1024, 640, 480, 0, nil)
	videoMP4 := test.NewMockMedia("55be7386-6851-406f-9c02-2b17bd05eb45", "test.mp4", "video/mp4", "http://mock.com/media/55be7386-6851-406f-9c02-2b17bd05eb45/test.mp4", 1024*1024, 0, 0, 1000, []*models.Media{thumbJPG})

	videoMOV := test.NewMockMedia("1a1a5b81-6f4f-4bf9-9dfc-e0e13c8b0d47", "test.mov", "video/quicktime", "http://mock.com/media/1a1a5b81-6f4f-4bf9-9dfc-e0e13c8b0d47/test.mov", 100*1024*1024, 0, 0, 2000, nil)

	filePDF := test.NewMockMedia("4b3a4a4e-2b4f-4bb1-9e0f-e19d17f0d0ea", "test.pdf", "application/pdf", "http://mock.com/media/4b3a4a4e-2b4f-4bb1-9e0f-e19d17f0d0ea/test.pdf", 100*1024*1024, 0, 0, 0, nil)

	testsuite.InsertMedia(t, rt, imageJPG)
	testsuite.InsertMedia(t, rt, audioMP3)
	testsuite.InsertMedia(t, rt, videoMP4)
	testsuite.InsertMedia(t, rt, videoMOV)
	testsuite.InsertMedia(t, rt, filePDF)
}

func TestBuildAttachmentRequest(t *testing.T) {
	lnHandler := newHandler(nil, channels.NewRoutes()).(*handler)
	req, _ := lnHandler.BuildAttachmentRequest(context.Background(), testChannels[0], "https://example.org/v1/media/41", nil)
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, "Bearer the-auth-token", req.Header.Get("Authorization"))
}

func TestSendEvent(t *testing.T) {
	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "LN", "2020", "US", []string{urns.Line.Prefix}, map[string]any{"auth_token": "AccessToken"})

	s := web.NewServer(runtime.NewTestRuntime(runtime.NewDefaultConfig()))
	h := s.MountHandler(newHandler).(*handler)

	s.Runtime().HTTP.Default.Transport = test.MockTransport(map[string][]*httpx.MockResponse{
		"https://api.line.me/v2/bot/chat/loading/start": {
			httpx.NewMockResponse(202, nil, []byte(`{}`)),
			httpx.NewMockResponse(400, nil, []byte(`{"message": "The property, 'chatId', in the request body is invalid"}`)),
			httpx.MockConnectionError,
		},
	})

	// typing indicators are supported but there's no explicit stop
	assert.Equal(t, map[string]time.Duration{events.TypeTypingStarted: 15 * time.Second}, h.SendableEvents(ch))

	channelRef := assets.NewChannelReference("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "LINE")
	typing := events.NewTypingStarted(events.DirectionOutgoing, channelRef, "line:uabcdefghij", "")

	// a typing started event is sent as a loading indicator
	clog := models.NewChannelLogForEventSend(ch, nil)
	err := h.SendEvent(context.Background(), ch, typing, clog)
	assert.NoError(t, err)
	assert.Len(t, clog.HttpLogs, 1)
	assert.Equal(t, "https://api.line.me/v2/bot/chat/loading/start", clog.HttpLogs[0].URL)
	assert.Contains(t, clog.HttpLogs[0].Request, `{"chatId":"uabcdefghij","loadingSeconds":20}`)

	// an error response is a response error
	err = h.SendEvent(context.Background(), ch, typing, clog)
	assert.Equal(t, channels.ErrResponseStatus, err)

	// as is a connection error
	err = h.SendEvent(context.Background(), ch, typing, clog)
	assert.Equal(t, channels.ErrConnectionFailed, err)

	// a channel without an auth token config can't send
	noAuth := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "LN", "2020", "US", []string{urns.Line.Prefix}, nil)
	err = h.SendEvent(context.Background(), noAuth, typing, clog)
	assert.Equal(t, channels.ErrChannelConfig, err)

	// nor can an event type the handler doesn't declare support for
	err = h.SendEvent(context.Background(), ch, events.NewTypingStopped(events.DirectionOutgoing, channelRef, "line:uabcdefghij", ""), clog)
	assert.ErrorContains(t, err, "unsupported event type: typing_stopped")
}

func TestIncoming(t *testing.T) {
	RunIncomingTests(t, testChannels, newHandler, "testdata/incoming.json", &IncomingOptions{Sign: addValidSignature})
}

func TestOutgoing(t *testing.T) {
	maxMsgLength = 160
	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "LN", "2020", "US", []string{urns.Line.Prefix}, map[string]any{"auth_token": "AccessToken"})

	RunOutgoingTests(t, ch, newHandler, "testdata/outgoing.json", &OutgoingOptions{CheckRedacted: []string{"AccessToken"}, Setup: setupMedia})
}
