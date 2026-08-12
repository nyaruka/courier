package web_test

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/courier/v26/web"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/stretchr/testify/assert"
)

func TestFetchAndStoreAttachment(t *testing.T) {
	testJPG := test.ReadFile("../test/testdata/test.jpg")

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(1234, time.Now))

	ctx := context.Background()
	rt := testsuite.NewRuntime(t)
	rt.S3.Client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String("test-attachments")})

	rt.HTTP.Attachments = &http.Client{Transport: httpx.WithTraces(httpx.WithMocks(nil, map[string][]*httpx.MockResponse{
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
	}))}

	mockChannel := test.NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230", "MCK", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{})

	clog := models.NewChannelLogForAttachmentFetch(mockChannel, []string{"sesame"})

	att, err := web.FetchAndStoreAttachment(ctx, rt, mockChannel, "http://mock.com/media/hello.jpg", clog)
	assert.NoError(t, err)
	assert.Equal(t, "image/jpeg", att.ContentType)
	assert.Equal(t, "http://localstack:4566/test-attachments/attachments/1/f884/4b62/f8844b62-b014-4975-9a98-cfcce3019710.jpg", att.URL)
	assert.Equal(t, 17301, att.Size)

	assert.Len(t, clog.HttpLogs, 1)
	assert.Equal(t, "http://mock.com/media/hello.jpg", clog.HttpLogs[0].URL)

	att, err = web.FetchAndStoreAttachment(ctx, rt, mockChannel, "http://mock.com/media/hello2", clog)
	assert.NoError(t, err)
	assert.Equal(t, "image/jpeg", att.ContentType)
	assert.Equal(t, "http://localstack:4566/test-attachments/attachments/1/d4bb/9822/d4bb9822-7160-4af3-b92b-40dae35f038b.jpg", att.URL)
	assert.Equal(t, 17301, att.Size)

	assert.Len(t, clog.HttpLogs, 2)
	assert.Equal(t, "http://mock.com/media/hello2", clog.HttpLogs[1].URL)

	// a non-200 response should return an unavailable attachment
	att, err = web.FetchAndStoreAttachment(ctx, rt, mockChannel, "http://mock.com/media/hello.mp3", clog)
	assert.NoError(t, err)
	assert.Equal(t, &web.Attachment{ContentType: "unavailable", URL: "http://mock.com/media/hello.mp3"}, att)

	// should have a logged HTTP request but no attachments will have been saved to storage
	assert.Len(t, clog.HttpLogs, 3)
	assert.Equal(t, "http://mock.com/media/hello.mp3", clog.HttpLogs[2].URL)

	// same for a connection error
	att, err = web.FetchAndStoreAttachment(ctx, rt, mockChannel, "http://mock.com/media/hello.pdf", clog)
	assert.NoError(t, err)
	assert.Equal(t, &web.Attachment{ContentType: "unavailable", URL: "http://mock.com/media/hello.pdf"}, att)

	att, err = web.FetchAndStoreAttachment(ctx, rt, mockChannel, "http://mock.com/media/hello3", clog)
	assert.NoError(t, err)
	assert.Equal(t, "image/jpeg", att.ContentType)
	assert.Equal(t, "http://localstack:4566/test-attachments/attachments/1/e527/3bef/e5273bef-6a8d-421f-8920-17713634b9f5.jpg", att.URL)
	assert.Equal(t, 17301, att.Size)

	att, err = web.FetchAndStoreAttachment(ctx, rt, mockChannel, "http://mock.com/media/hello7", clog)
	assert.NoError(t, err)
	assert.Equal(t, "application/octet-stream", att.ContentType)
	assert.Equal(t, "http://localstack:4566/test-attachments/attachments/1/f879/21a1/f87921a1-0484-4660-9955-f9b28b006b78", att.URL)
	assert.Equal(t, 11, att.Size)

	// an actual error on our part (e.g. storage unavailable) should be returned as an error
	rt.Config.S3AttachmentsBucket = "does-not-exist"

	att, err = web.FetchAndStoreAttachment(ctx, rt, mockChannel, "http://mock.com/media/hello.txt", clog)
	assert.Error(t, err)
	assert.Nil(t, att)
}

func TestFetchAndStoreAttachmentAccessDenied(t *testing.T) {
	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(1234, time.Now))

	ctx := context.Background()

	rt := testsuite.NewRuntime(t)

	// wrap the transport in access control that blocks loopback, so a fetch of a disallowed host is
	// rejected before any connection is made; the mocking transport underneath has no entries and so
	// would panic if a request ever reached it, guarding against the access check silently passing
	access := httpx.NewAccessConfig(time.Second, []net.IP{net.ParseIP("127.0.0.1")}, nil)
	rt.HTTP.Attachments = &http.Client{Transport: httpx.WithTraces(httpx.WithAccessControl(httpx.WithMocks(nil, map[string][]*httpx.MockResponse{}), access))}

	mockChannel := test.NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230", "MCK", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{})

	clog := models.NewChannelLogForAttachmentFetch(mockChannel, nil)

	// a request denied by the SSRF blocklist should yield an "unavailable" attachment rather than an error
	att, err := web.FetchAndStoreAttachment(ctx, rt, mockChannel, "http://127.0.0.1/media/blocked.jpg", clog)
	assert.NoError(t, err)
	assert.Equal(t, &web.Attachment{ContentType: "unavailable", URL: "http://127.0.0.1/media/blocked.jpg"}, att)

	// nothing is saved to storage, but the denied request is still logged
	assert.Len(t, clog.HttpLogs, 1)
}

func TestFetchAndStoreAttachmentOversized(t *testing.T) {
	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(1234, time.Now))

	ctx := context.Background()

	rt := testsuite.NewRuntime(t)
	rt.S3.Client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String("test-attachments")})

	testJPG := test.ReadFile("../test/testdata/test.jpg")

	// the real client bounds bodies at runtime.MaxAttachmentBodyBytes; use a tiny bound here so an oversized
	// response is cheap to produce. The limit goes inside the tracing, exactly as the runtime composes it, so
	// that it applies before the body is buffered into the trace.
	const limit = 64
	rt.HTTP.Attachments = &http.Client{Transport: httpx.WithTraces(httpx.WithReadLimit(httpx.WithMocks(nil, map[string][]*httpx.MockResponse{
		"http://mock.com/media/huge.jpg": {
			httpx.NewMockResponse(200, nil, bytes.Repeat([]byte("x"), limit*10)),
		},
		"http://mock.com/media/small.jpg": {
			httpx.NewMockResponse(200, map[string]string{"Content-Type": "image/jpeg"}, testJPG[:limit]),
		},
	}), limit))}

	mockChannel := test.NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230", "MCK", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{})
	clog := models.NewChannelLogForAttachmentFetch(mockChannel, nil)

	// a body past the limit must come back unavailable rather than being stored truncated. The limit is enforced
	// as the body is read and surfaced on the handed-back body rather than returned, so this is what proves the
	// fetch drains it - without that drain a partial body would be saved as if it were the whole file.
	att, err := web.FetchAndStoreAttachment(ctx, rt, mockChannel, "http://mock.com/media/huge.jpg", clog)
	assert.NoError(t, err)
	assert.Equal(t, &web.Attachment{ContentType: "unavailable", URL: "http://mock.com/media/huge.jpg"}, att)

	// a body within the limit is still fetched and stored as normal
	att, err = web.FetchAndStoreAttachment(ctx, rt, mockChannel, "http://mock.com/media/small.jpg", clog)
	assert.NoError(t, err)
	assert.Equal(t, "image/jpeg", att.ContentType)
	assert.Equal(t, limit, att.Size)

	// both attempts are logged either way
	assert.Len(t, clog.HttpLogs, 2)
}
