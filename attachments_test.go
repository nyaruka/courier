package courier_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/stretchr/testify/assert"
)

func TestFetchAndStoreAttachment(t *testing.T) {
	testJPG := test.ReadFile("test/testdata/test.jpg")

	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]*httpx.MockResponse{
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
	}))

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(1234, time.Now))

	ctx := context.Background()
	mb := test.NewMockBackend()

	mockChannel := test.NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230", "MCK", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{})
	mb.AddChannel(mockChannel)

	clog := courier.NewChannelLogForAttachmentFetch(mockChannel, []string{"sesame"})

	att, err := courier.FetchAndStoreAttachment(ctx, mb, mockChannel, "http://mock.com/media/hello.jpg", clog)
	assert.NoError(t, err)
	assert.Equal(t, "image/jpeg", att.ContentType)
	assert.Equal(t, "https://backend.com/attachments/cdf7ed27-5ad5-4028-b664-880fc7581c77.jpg", att.URL)
	assert.Equal(t, 17301, att.Size)

	assert.Len(t, mb.SavedAttachments(), 1)
	assert.Equal(t, &test.SavedAttachment{Channel: mockChannel, ContentType: "image/jpeg", Data: testJPG, Extension: "jpg"}, mb.SavedAttachments()[0])
	assert.Len(t, clog.HttpLogs, 1)
	assert.Equal(t, "http://mock.com/media/hello.jpg", clog.HttpLogs[0].URL)

	att, err = courier.FetchAndStoreAttachment(ctx, mb, mockChannel, "http://mock.com/media/hello2", clog)
	assert.NoError(t, err)
	assert.Equal(t, "image/jpeg", att.ContentType)
	assert.Equal(t, "https://backend.com/attachments/547deaf7-7620-4434-95b3-58675999c4b7.jpg", att.URL)
	assert.Equal(t, 17301, att.Size)

	assert.Len(t, mb.SavedAttachments(), 2)
	assert.Equal(t, &test.SavedAttachment{Channel: mockChannel, ContentType: "image/jpeg", Data: testJPG, Extension: "jpg"}, mb.SavedAttachments()[0])
	assert.Len(t, clog.HttpLogs, 2)
	assert.Equal(t, "http://mock.com/media/hello2", clog.HttpLogs[1].URL)

	// a non-200 response should return an unavailable attachment
	att, err = courier.FetchAndStoreAttachment(ctx, mb, mockChannel, "http://mock.com/media/hello.mp3", clog)
	assert.NoError(t, err)
	assert.Equal(t, &courier.Attachment{ContentType: "unavailable", URL: "http://mock.com/media/hello.mp3"}, att)

	// should have a logged HTTP request but no attachments will have been saved to storage
	assert.Len(t, clog.HttpLogs, 3)
	assert.Equal(t, "http://mock.com/media/hello.mp3", clog.HttpLogs[2].URL)
	assert.Len(t, mb.SavedAttachments(), 2)

	// same for a connection error
	att, err = courier.FetchAndStoreAttachment(ctx, mb, mockChannel, "http://mock.com/media/hello.pdf", clog)
	assert.NoError(t, err)
	assert.Equal(t, &courier.Attachment{ContentType: "unavailable", URL: "http://mock.com/media/hello.pdf"}, att)

	att, err = courier.FetchAndStoreAttachment(ctx, mb, mockChannel, "http://mock.com/media/hello3", clog)
	assert.NoError(t, err)
	assert.Equal(t, "image/jpeg", att.ContentType)
	assert.Equal(t, "https://backend.com/attachments/338ff339-5663-49ed-8ef6-384876655d1b.jpg", att.URL)
	assert.Equal(t, 17301, att.Size)

	att, err = courier.FetchAndStoreAttachment(ctx, mb, mockChannel, "http://mock.com/media/hello7", clog)
	assert.NoError(t, err)
	assert.Equal(t, "application/octet-stream", att.ContentType)
	assert.Equal(t, "https://backend.com/attachments/9b955e36-ac16-4c6b-8ab6-9b9af5cd042a.", att.URL)
	assert.Equal(t, 11, att.Size)

	// an actual error on our part should be returned as an error
	mb.SetStorageError(errors.New("boom"))

	att, err = courier.FetchAndStoreAttachment(ctx, mb, mockChannel, "http://mock.com/media/hello.txt", clog)
	assert.EqualError(t, err, "boom")
	assert.Nil(t, att)
}
