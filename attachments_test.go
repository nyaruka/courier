package courier_test

import (
	"context"
	"testing"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/stretchr/testify/assert"
)

func TestFetchAndStoreAttachment(t *testing.T) {
	testJPG := test.ReadFile("test/testdata/test.jpg")

	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]*httpx.MockResponse{
		"http://mock.com/media/hello.jpg": {
			httpx.NewMockResponse(200, nil, testJPG),
		},
	}))

	ctx := context.Background()
	mb := test.NewMockBackend()

	mockChannel := test.NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230", "MCK", "2020", "US", map[string]interface{}{})
	mb.AddChannel(mockChannel)

	clog := courier.NewChannelLogForAttachmentFetch(mockChannel, []string{"sesame"})

	newURL, err := courier.FetchAndStoreAttachment(ctx, mb, mockChannel, "http://mock.com/media/hello.jpg", clog)
	assert.NoError(t, err)
	assert.Equal(t, "https://backend.com/attachments/test.jpg", newURL)

	assert.Len(t, mb.SavedAttachments(), 1)
	assert.Equal(t, &test.SavedAttachment{Channel: mockChannel, ContentType: "image/jpeg", Data: testJPG, Extension: "jpg"}, mb.SavedAttachments()[0])
	assert.Len(t, clog.HTTPLogs(), 1)
	assert.Equal(t, "http://mock.com/media/hello.jpg", clog.HTTPLogs()[0].URL)
}
