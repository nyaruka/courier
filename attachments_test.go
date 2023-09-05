package courier_test

import (
	"context"
	"errors"
	"testing"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
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
		"http://mock.com/media/hello.mp3": {
			httpx.NewMockResponse(502, nil, []byte(`My gateways!`)),
		},
		"http://mock.com/media/hello.pdf": {
			httpx.MockConnectionError,
		},
		"http://mock.com/media/hello.txt": {
			httpx.NewMockResponse(200, nil, []byte(`hi`)),
		},
	}))

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(1234))

	ctx := context.Background()
	mb := test.NewMockBackend()

	mockChannel := test.NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230", "MCK", "2020", "US", map[string]any{})
	mb.AddChannel(mockChannel)

	clog := courier.NewChannelLogForAttachmentFetch(mockChannel, []string{"sesame"})

	att, err := courier.FetchAndStoreAttachment(ctx, mb, mockChannel, "http://mock.com/media/hello.jpg", clog)
	assert.NoError(t, err)
	assert.Equal(t, "image/jpeg", att.ContentType)
	assert.Equal(t, "https://backend.com/attachments/cdf7ed27-5ad5-4028-b664-880fc7581c77.jpg", att.URL)
	assert.Equal(t, 17301, att.Size)

	assert.Len(t, mb.SavedAttachments(), 1)
	assert.Equal(t, &test.SavedAttachment{Channel: mockChannel, ContentType: "image/jpeg", Data: testJPG, Extension: "jpg"}, mb.SavedAttachments()[0])
	assert.Len(t, clog.HTTPLogs(), 1)
	assert.Equal(t, "http://mock.com/media/hello.jpg", clog.HTTPLogs()[0].URL)

	// a non-200 response should return an unavailable attachment
	att, err = courier.FetchAndStoreAttachment(ctx, mb, mockChannel, "http://mock.com/media/hello.mp3", clog)
	assert.NoError(t, err)
	assert.Equal(t, &courier.Attachment{ContentType: "unavailable", URL: "http://mock.com/media/hello.mp3"}, att)

	// should have a logged HTTP request but no attachments will have been saved to storage
	assert.Len(t, clog.HTTPLogs(), 2)
	assert.Equal(t, "http://mock.com/media/hello.mp3", clog.HTTPLogs()[1].URL)
	assert.Len(t, mb.SavedAttachments(), 1)

	// same for a connection error
	att, err = courier.FetchAndStoreAttachment(ctx, mb, mockChannel, "http://mock.com/media/hello.pdf", clog)
	assert.NoError(t, err)
	assert.Equal(t, &courier.Attachment{ContentType: "unavailable", URL: "http://mock.com/media/hello.pdf"}, att)

	// an actual error on our part should be returned as an error
	mb.SetStorageError(errors.New("boom"))

	att, err = courier.FetchAndStoreAttachment(ctx, mb, mockChannel, "http://mock.com/media/hello.txt", clog)
	assert.EqualError(t, err, "boom")
	assert.Nil(t, att)
}
