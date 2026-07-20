package courier_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/nyaruka/courier/v26"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/stretchr/testify/assert"
)

func TestFetchAndStoreAttachment(t *testing.T) {
	testJPG := test.ReadFile("test/testdata/test.jpg")

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(1234, time.Now))

	ctx := context.Background()
	rt := runtime.NewTestRuntime(runtime.NewDefaultConfig())
	rt.HTTP.Transport = httpx.WithMocks(nil, map[string][]*httpx.MockResponse{
		"http://mock.com/media/hello.jpg": {
			httpx.NewMockResponse(200, nil, testJPG),
		},
		"http://mock.com/media/hello2": {
			httpx.NewMockResponse(200, map[string]string{"Content-Type": "image/jpeg"}, testJPG),
		},
		"http://mock.com/media/hello3": {
			httpx.NewMockResponse(200, map[string]string{"Content-Type": "application/octet-stream"}, testJPG),
		},
		"http://mock.com/media/hello.mp3": {
			httpx.NewMockResponse(502, nil, []byte(`My gateways!`)),
		},
		"http://mock.com/media/hello.pdf": {
			httpx.MockConnectionError,
		},
		"http://mock.com/media/hello.txt": {
			httpx.NewMockResponse(200, nil, []byte(`hi`)),
		},
		"http://mock.com/media/hello7": {
			httpx.NewMockResponse(200, nil, []byte(`hello world`)),
		},
	})
	mb := test.NewMockBackend()

	mockChannel := test.NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230", "MCK", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{})
	mb.AddChannel(mockChannel)

	clog := courier.NewChannelLogForAttachmentFetch(mockChannel, []string{"sesame"})

	att, err := courier.FetchAndStoreAttachment(ctx, rt, mb, mockChannel, "http://mock.com/media/hello.jpg", clog)
	assert.NoError(t, err)
	assert.Equal(t, "image/jpeg", att.ContentType)
	assert.Equal(t, "https://backend.com/attachments/f8844b62-b014-4975-9a98-cfcce3019710.jpg", att.URL)
	assert.Equal(t, 17301, att.Size)

	assert.Len(t, mb.SavedAttachments(), 1)
	assert.Equal(t, &test.SavedAttachment{Channel: mockChannel, ContentType: "image/jpeg", Data: testJPG, Extension: "jpg"}, mb.SavedAttachments()[0])
	assert.Len(t, clog.HttpLogs, 1)
	assert.Equal(t, "http://mock.com/media/hello.jpg", clog.HttpLogs[0].URL)

	att, err = courier.FetchAndStoreAttachment(ctx, rt, mb, mockChannel, "http://mock.com/media/hello2", clog)
	assert.NoError(t, err)
	assert.Equal(t, "image/jpeg", att.ContentType)
	assert.Equal(t, "https://backend.com/attachments/d4bb9822-7160-4af3-b92b-40dae35f038b.jpg", att.URL)
	assert.Equal(t, 17301, att.Size)

	assert.Len(t, mb.SavedAttachments(), 2)
	assert.Equal(t, &test.SavedAttachment{Channel: mockChannel, ContentType: "image/jpeg", Data: testJPG, Extension: "jpg"}, mb.SavedAttachments()[0])
	assert.Len(t, clog.HttpLogs, 2)
	assert.Equal(t, "http://mock.com/media/hello2", clog.HttpLogs[1].URL)

	// a non-200 response should return an unavailable attachment
	att, err = courier.FetchAndStoreAttachment(ctx, rt, mb, mockChannel, "http://mock.com/media/hello.mp3", clog)
	assert.NoError(t, err)
	assert.Equal(t, &courier.Attachment{ContentType: "unavailable", URL: "http://mock.com/media/hello.mp3"}, att)

	// should have a logged HTTP request but no attachments will have been saved to storage
	assert.Len(t, clog.HttpLogs, 3)
	assert.Equal(t, "http://mock.com/media/hello.mp3", clog.HttpLogs[2].URL)
	assert.Len(t, mb.SavedAttachments(), 2)

	// same for a connection error
	att, err = courier.FetchAndStoreAttachment(ctx, rt, mb, mockChannel, "http://mock.com/media/hello.pdf", clog)
	assert.NoError(t, err)
	assert.Equal(t, &courier.Attachment{ContentType: "unavailable", URL: "http://mock.com/media/hello.pdf"}, att)

	att, err = courier.FetchAndStoreAttachment(ctx, rt, mb, mockChannel, "http://mock.com/media/hello3", clog)
	assert.NoError(t, err)
	assert.Equal(t, "image/jpeg", att.ContentType)
	assert.Equal(t, "https://backend.com/attachments/e5273bef-6a8d-421f-8920-17713634b9f5.jpg", att.URL)
	assert.Equal(t, 17301, att.Size)

	att, err = courier.FetchAndStoreAttachment(ctx, rt, mb, mockChannel, "http://mock.com/media/hello7", clog)
	assert.NoError(t, err)
	assert.Equal(t, "application/octet-stream", att.ContentType)
	assert.Equal(t, "https://backend.com/attachments/f87921a1-0484-4660-9955-f9b28b006b78.", att.URL)
	assert.Equal(t, 11, att.Size)

	// an actual error on our part should be returned as an error
	mb.SetStorageError(errors.New("boom"))

	att, err = courier.FetchAndStoreAttachment(ctx, rt, mb, mockChannel, "http://mock.com/media/hello.txt", clog)
	assert.EqualError(t, err, "boom")
	assert.Nil(t, att)
}

func TestFetchAndStoreAttachmentAccessDenied(t *testing.T) {
	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(1234, time.Now))

	ctx := context.Background()

	rt := runtime.NewTestRuntime(runtime.NewDefaultConfig())

	// wrap the transport in access control that blocks loopback, so a fetch of a disallowed host is
	// rejected before any connection is made; the mocking transport underneath has no entries and so
	// would panic if a request ever reached it, guarding against the access check silently passing
	access := httpx.NewAccessConfig(time.Second, []net.IP{net.ParseIP("127.0.0.1")}, nil)
	rt.HTTP.Transport = httpx.WithAccessControl(httpx.WithMocks(nil, map[string][]*httpx.MockResponse{}), access)

	mb := test.NewMockBackend()
	mockChannel := test.NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230", "MCK", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{})
	mb.AddChannel(mockChannel)

	clog := courier.NewChannelLogForAttachmentFetch(mockChannel, nil)

	// a request denied by the SSRF blocklist should yield an "unavailable" attachment rather than an error
	att, err := courier.FetchAndStoreAttachment(ctx, rt, mb, mockChannel, "http://127.0.0.1/media/blocked.jpg", clog)
	assert.NoError(t, err)
	assert.Equal(t, &courier.Attachment{ContentType: "unavailable", URL: "http://127.0.0.1/media/blocked.jpg"}, att)

	// nothing is saved to storage, but the denied request is still logged
	assert.Empty(t, mb.SavedAttachments())
	assert.Len(t, clog.HttpLogs, 1)
}
