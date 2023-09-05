package courier_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/stretchr/testify/assert"
)

func TestChannelLog(t *testing.T) {
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]*httpx.MockResponse{
		"https://api.messages.com/send.json": {
			httpx.NewMockResponse(200, nil, []byte(`{"status":"success"}`)),
			httpx.MockConnectionError,
		},
	}))
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	uuids.SetGenerator(uuids.NewSeededGenerator(1234))
	defer uuids.SetGenerator(uuids.DefaultGenerator)

	channel := test.NewMockChannel("fef91e9b-a6ed-44fb-b6ce-feed8af585a8", "NX", "1234", "US", nil)
	clog := courier.NewChannelLog(courier.ChannelLogTypeTokenRefresh, channel, nil)

	// make a request that will have a response
	req, _ := http.NewRequest("POST", "https://api.messages.com/send.json", nil)
	trace, err := httpx.DoTrace(http.DefaultClient, req, nil, nil, 0)
	assert.NoError(t, err)

	clog.HTTP(trace)

	// make a request that has no response (connection error)
	req, _ = http.NewRequest("POST", "https://api.messages.com/send.json", nil)
	trace, err = httpx.DoTrace(http.DefaultClient, req, nil, nil, 0)
	assert.EqualError(t, err, "unable to connect to server")

	clog.HTTP(trace)
	clog.Error(courier.NewChannelError("not_right", "", "Something not right"))
	clog.RawError(errors.New("this is an error"))
	clog.End()

	assert.Equal(t, courier.ChannelLogUUID("c00e5d67-c275-4389-aded-7d8b151cbd5b"), clog.UUID())
	assert.Equal(t, courier.ChannelLogTypeTokenRefresh, clog.Type())
	assert.Equal(t, channel, clog.Channel())
	assert.False(t, clog.Attached())
	assert.Equal(t, 2, len(clog.HTTPLogs()))
	assert.Equal(t, 2, len(clog.Errors()))
	assert.False(t, clog.CreatedOn().IsZero())
	assert.Greater(t, clog.Elapsed(), time.Duration(0))

	hlog1 := clog.HTTPLogs()[0]
	assert.Equal(t, "https://api.messages.com/send.json", hlog1.URL)
	assert.Equal(t, 200, hlog1.StatusCode)
	assert.Equal(t, "POST /send.json HTTP/1.1\r\nHost: api.messages.com\r\nUser-Agent: Go-http-client/1.1\r\nContent-Length: 0\r\nAccept-Encoding: gzip\r\n\r\n", hlog1.Request)
	assert.Equal(t, "HTTP/1.0 200 OK\r\nContent-Length: 20\r\n\r\n{\"status\":\"success\"}", hlog1.Response)

	hlog2 := clog.HTTPLogs()[1]
	assert.Equal(t, 0, hlog2.StatusCode)
	assert.Equal(t, "POST /send.json HTTP/1.1\r\nHost: api.messages.com\r\nUser-Agent: Go-http-client/1.1\r\nContent-Length: 0\r\nAccept-Encoding: gzip\r\n\r\n", hlog2.Request)
	assert.Equal(t, "", hlog2.Response)

	err1 := clog.Errors()[0]
	assert.Equal(t, "not_right", err1.Code())
	assert.Equal(t, "", err1.ExtCode())
	assert.Equal(t, "Something not right", err1.Message())

	err2 := clog.Errors()[1]
	assert.Equal(t, "this is an error", err2.Message())
	assert.Equal(t, "", err2.Code())

	clog.SetAttached(true)
	clog.SetType(courier.ChannelLogTypeEventReceive)

	assert.True(t, clog.Attached())
	assert.Equal(t, courier.ChannelLogTypeEventReceive, clog.Type())
}

func TestChannelErrors(t *testing.T) {
	tcs := []struct {
		err             *courier.ChannelError
		expectedCode    string
		expectedExtCode string
		expectedMessage string
	}{
		{
			err:             courier.ErrorResponseStatusCode(),
			expectedCode:    "response_status_code",
			expectedMessage: "Unexpected response status code.",
		},
		{
			err:             courier.ErrorResponseUnparseable("FOO"),
			expectedCode:    "response_unparseable",
			expectedMessage: "Unable to parse response as FOO.",
		},
		{
			err:             courier.ErrorResponseUnexpected("all good!"),
			expectedCode:    "response_unexpected",
			expectedMessage: "Expected response to be 'all good!'.",
		},
		{
			err:             courier.ErrorResponseValueMissing("id"),
			expectedCode:    "response_value_missing",
			expectedMessage: "Unable to find 'id' response.",
		},
		{
			err:             courier.ErrorResponseValueUnexpected("status", "SUCCESS"),
			expectedCode:    "response_value_unexpected",
			expectedMessage: "Expected 'status' in response to be 'SUCCESS'.",
		},
		{
			err:             courier.ErrorResponseValueUnexpected("status", "SUCCESS", "OK"),
			expectedCode:    "response_value_unexpected",
			expectedMessage: "Expected 'status' in response to be 'SUCCESS' or 'OK'.",
		},
		{
			err:             courier.ErrorMediaUnsupported("image/tiff"),
			expectedCode:    "media_unsupported",
			expectedMessage: "Unsupported attachment media type: image/tiff.",
		},
		{
			err:             courier.ErrorAttachmentNotDecodable(),
			expectedCode:    "attachment_not_decodable",
			expectedMessage: "Unable to decode embedded attachment data.",
		},
		{
			err:             courier.ErrorExternal("20002", "Invalid FriendlyName."),
			expectedCode:    "external",
			expectedExtCode: "20002",
			expectedMessage: "Invalid FriendlyName.",
		},
		{
			err:             courier.ErrorExternal("20003", ""),
			expectedCode:    "external",
			expectedExtCode: "20003",
			expectedMessage: "Service specific error: 20003.",
		},
	}

	for _, tc := range tcs {
		assert.Equal(t, tc.expectedCode, tc.err.Code())
		assert.Equal(t, tc.expectedExtCode, tc.err.ExtCode())
		assert.Equal(t, tc.expectedMessage, tc.err.Message())
	}
}
