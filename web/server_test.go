package web_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/core/sender"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/courier/v26/utils/queue"
	"github.com/nyaruka/courier/v26/web"
	"github.com/nyaruka/gocommon/aws/dynamo"
	"github.com/nyaruka/gocommon/aws/dynamo/dyntest"
	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// the server no longer owns the models layer's lifecycle - that moved to the service wiring - so tests start it
// themselves via testsuite.Runtime
func serverRuntime(t *testing.T) *runtime.Runtime {
	_, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)

	rt.Config.InternetPort = 8180
	rt.Config.InternalPort = 8181
	return rt
}

func TestIncoming(t *testing.T) {
	rt := serverRuntime(t)

	s := web.NewServer(rt)

	// capture the channel logs of handled requests
	var clogs []*models.ChannelLog
	s.OnRequestHandled(func(ch *models.Channel, evts []channels.Event, clog *models.ChannelLog) { clogs = append(clogs, clog) })

	require.NoError(t, s.Start())
	defer s.Stop()

	testsuite.InsertChannel(t, rt, test.NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230", "MCK", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{}))

	resp, err := http.Get("http://localhost:8180/c/mck/e4bb1578-29da-4fa5-a214-9da19dd24230/receive")
	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "missing from or text")

	if assert.Len(t, clogs, 1) {
		assert.Len(t, clogs[0].HttpLogs, 1)
	}

	req, _ := http.NewRequest("GET", "http://localhost:8180/c/mck/e4bb1578-29da-4fa5-a214-9da19dd24230/receive?from=2065551212&text=hello", nil)
	req.Header.Set("Cookie", "secret")
	resp, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	defer resp.Body.Close()
	body, _ = io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "ok")

	if assert.Len(t, clogs, 2) {
		assert.Len(t, clogs[1].HttpLogs, 1)
	}
}

func TestOutgoing(t *testing.T) {
	rt := serverRuntime(t)
	dyntest.Truncate(t, rt.Dynamo.Main.Client(), rt.Dynamo.Main.Table())

	s := web.NewServer(rt)
	rt.HTTP.Default.Transport = httpx.WithTraces(httpx.WithMocks(nil, map[string][]*httpx.MockResponse{
		"http://mock.com/send": {
			httpx.NewMockResponse(200, nil, []byte(`SENT`)),
			httpx.MockConnectionError,
			httpx.NewMockResponse(200, nil, []byte(`SENT`)),
			httpx.NewMockResponse(429, nil, []byte(`too much!`)),
			httpx.NewMockResponse(403, nil, []byte(`stop!`)),
		},
	}))

	require.NoError(t, s.Start())
	defer s.Stop()

	// sending is its own component now, so the test drives one alongside the server
	foreman := sender.NewForeman(rt, 32)
	foreman.Start()
	defer foreman.Stop()

	// create two channels but only one of them has a handler (MCK)
	brokenChannel := test.NewMockChannel("53e5aafa-8155-449d-9009-fcb30d54bd26", "XX", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{})
	mockChannel := test.NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230", "MCK", "2021", "US", []string{urns.Phone.Prefix}, map[string]any{})
	testsuite.InsertChannel(t, rt, brokenChannel)
	testsuite.InsertChannel(t, rt, mockChannel)

	// inserts an outgoing msg into the database so status updates can be written for it
	insertMsg := func(uuid models.MsgUUID, ch *models.Channel) {
		rt.DB.MustExec(`INSERT INTO msgs_msg(uuid, text, high_priority, created_on, modified_on, direction, status, visibility, msg_type, is_android,
			msg_count, error_count, external_identifier, channel_id, contact_id, contact_urn_id, org_id, log_uuids)
			VALUES($1, 'test', TRUE, NOW(), NOW(), 'O', 'Q', 'V', 'T', FALSE, 1, 0, '', $2, 100, 1000, 1, '{}')`, uuid, ch.ID())
	}

	// queues an outgoing msg to be popped and sent by the server's foreman
	send := func(uuid models.MsgUUID, ch *models.Channel, text string) {
		m := &models.MsgOut{
			OrgID_:        1,
			UUID_:         uuid,
			Contact_:      &models.ContactReference{ID: 100, UUID: "a984069d-0008-4d8c-a772-b14a8a6acccc"},
			Text_:         text,
			HighPriority_: true,
			CreatedOn_:    time.Now(),
			ChannelUUID_:  ch.UUID(),
			URN_:          "tel:+12067799192",
			Origin_:       models.MsgOriginChat,
		}
		insertMsg(uuid, ch)

		rc := rt.VK.Get()
		defer rc.Close()
		require.NoError(t, queue.PushOntoQueue(rc, "msgs", string(ch.UUID()), 10, "["+string(jsonx.MustMarshal(m))+"]", queue.HighPriority))
	}

	// waits for the msg to reach the given status in the database (status writes are batched), reporting the status
	// it actually reached if it doesn't get there
	assertStatus := func(uuid models.MsgUUID, expected models.MsgStatus) {
		t.Helper()

		var actual string
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
			require.NoError(t, rt.DB.Get(&actual, `SELECT status FROM msgs_msg WHERE uuid = $1`, uuid))
			if actual == string(expected) {
				return
			}
		}
		require.Equal(t, string(expected), actual, "msg %s never reached expected status", uuid)
	}

	// try to send message via the channel without a handler.. should be marked as failed
	send("0199df01-b383-7610-80f9-fd952f8d489c", brokenChannel, "test message")
	assertStatus("0199df01-b383-7610-80f9-fd952f8d489c", models.MsgStatusFailed)

	// send message via the registered channel.. should be marked as wired
	send("0199df01-dacc-754b-a830-ab2bf0f511d3", mockChannel, "test message 2")
	assertStatus("0199df01-dacc-754b-a830-ab2bf0f511d3", models.MsgStatusWired)

	// and we should have a channel log for the send with redacted errors and traces
	require.Eventually(t, func() bool {
		for _, item := range dyntest.ScanAll(t, rt.Dynamo.Main.Client(), rt.Dynamo.Main.Table()) {
			if strings.HasPrefix(item.Key.SK, "log#") && item.Data["type"] == "msg_send" {
				var dataGZ struct {
					HttpLogs []*httpx.Log `json:"http_logs"`
					Errors   []struct {
						Code    string `json:"code"`
						Message string `json:"message"`
					} `json:"errors"`
				}
				require.NoError(t, dynamo.UnmarshalJSONGZ(item.DataGZ, &dataGZ))

				if len(dataGZ.HttpLogs) == 1 && dataGZ.HttpLogs[0].URL == "http://mock.com/send" {
					assert.Contains(t, dataGZ.HttpLogs[0].Request, "Authorization: Token **********")
					require.Len(t, dataGZ.Errors, 1)
					assert.Equal(t, "contains ********** seeds", dataGZ.Errors[0].Message)
					return true
				}
			}
		}
		return false
	}, 5*time.Second, 100*time.Millisecond)

	// queue the same message again - it should be skipped as a dupe send but again marked as wired,
	// which we can observe as a second log UUID appended to the msg
	rc := rt.VK.Get()
	m2 := &models.MsgOut{
		OrgID_: 1, UUID_: "0199df01-dacc-754b-a830-ab2bf0f511d3", Contact_: &models.ContactReference{ID: 100, UUID: "a984069d-0008-4d8c-a772-b14a8a6acccc"},
		Text_: "test message 2", HighPriority_: true, CreatedOn_: time.Now(), ChannelUUID_: mockChannel.UUID(), URN_: "tel:+12067799192", Origin_: models.MsgOriginChat,
	}
	require.NoError(t, queue.PushOntoQueue(rc, "msgs", string(mockChannel.UUID()), 10, "["+string(jsonx.MustMarshal(m2))+"]", queue.HighPriority))
	rc.Close()

	var numLogs int
	require.Eventually(t, func() bool {
		require.NoError(t, rt.DB.Get(&numLogs, `SELECT coalesce(array_length(log_uuids, 1), 0) FROM msgs_msg WHERE uuid = '0199df01-dacc-754b-a830-ab2bf0f511d3'`))
		return numLogs == 2
	}, 5*time.Second, 100*time.Millisecond)

	// send message which will have mocked connection error.. should be marked as errored (retryable)
	send("0199df02-1ec4-73ba-8e69-fa77d344fb25", mockChannel, "3")
	assertStatus("0199df02-1ec4-73ba-8e69-fa77d344fb25", models.MsgStatusErrored)

	// send message which will have mocked channel config error.. should be marked as failed (non-retryable)
	send("0199df02-3d56-7c69-a25e-2dc8ecff4da5", mockChannel, "err:config")
	assertStatus("0199df02-3d56-7c69-a25e-2dc8ecff4da5", models.MsgStatusFailed)

	// send message which will have mocked rate limiting error.. should be marked as errored (retryable)
	send("0199df02-5f1b-782b-b457-61ee96333d48", mockChannel, "5")
	assertStatus("0199df02-5f1b-782b-b457-61ee96333d48", models.MsgStatusErrored)

	// send message which will have mocked contact-stopped error.. should be marked as failed (non-retryable)
	send("0199df05-037b-718f-a6ad-ab66c10243b2", mockChannel, "6")
	assertStatus("0199df05-037b-718f-a6ad-ab66c10243b2", models.MsgStatusFailed)

	// and we should have created a contact stop event
	var numStopEvents int
	require.NoError(t, rt.DB.Get(&numStopEvents, `SELECT count(*) FROM channels_channelevent WHERE event_type = 'stop_contact'`))
	assert.Equal(t, 1, numStopEvents)
}

func TestFetchAttachment(t *testing.T) {
	testJPG := test.ReadFile("../test/testdata/test.jpg")

	rt := serverRuntime(t)
	rt.Config.AuthToken = "sesame"
	rt.S3.Client.CreateBucket(t.Context(), &s3.CreateBucketInput{Bucket: aws.String("test-attachments")})

	server := web.NewServer(rt)

	// attachments are fetched through the dedicated bounded client, so that's where the mocks go
	server.Runtime().HTTP.Attachments.Transport = test.MockTransport(map[string][]*httpx.MockResponse{
		"http://mock.com/media/hello.jpg": {
			httpx.NewMockResponse(200, nil, testJPG),
		},
		"http://mock.com/media/hello.mp3": {
			httpx.NewMockResponse(404, nil, []byte(`No such file`)),
		},
		"http://mock.com/media/hello.pdf": {
			httpx.MockConnectionError,
		},
	})
	require.NoError(t, server.Start())
	defer server.Stop()

	testsuite.InsertChannel(t, rt, test.NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230", "MCK", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{}))

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(1234, dates.NewSequentialNow(time.Date(2024, 9, 11, 14, 33, 0, 0, time.UTC), time.Second)))

	submit := func(body, authToken string) (int, []byte) {
		req, _ := http.NewRequest("POST", "http://localhost:8181/ci/attachment/fetch", strings.NewReader(body))
		if authToken != "" {
			req.Header.Set("Authorization", "Bearer "+authToken)
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp.StatusCode, respBody
	}

	// try to submit with no auth header
	statusCode, respBody := submit(`{}`, "")
	assert.Equal(t, 401, statusCode)
	assert.Equal(t, "Unauthorized", string(respBody))

	// try to submit with wrong auth header
	statusCode, respBody = submit(`{}`, "23462")
	assert.Equal(t, 401, statusCode)
	assert.Equal(t, "Unauthorized", string(respBody))

	// try to submit with empty body
	statusCode, respBody = submit(`{}`, "sesame")
	assert.Equal(t, 400, statusCode)
	assert.Contains(t, string(respBody), `Field validation for 'ChannelType' failed on the 'required' tag`)

	// try to submit with non-existent channel
	statusCode, respBody = submit(`{"channel_uuid": "c25aab53-f23a-46c9-8ae3-1af850ad9fd9", "channel_type": "VV", "url": "http://mock.com/media/hello.jpg"}`, "sesame")
	assert.Equal(t, 400, statusCode)
	assert.Contains(t, string(respBody), `channel not found`)

	statusCode, respBody = submit(`{"channel_uuid": "e4bb1578-29da-4fa5-a214-9da19dd24230", "channel_type": "MCK", "url": "http://mock.com/media/hello.jpg"}`, "sesame")
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{"attachment": {"content_type": "image/jpeg", "url": "http://localstack:4566/test-attachments/attachments/1/f884/4b62/f8844b62-b014-4975-9a98-cfcce3019710.jpg", "size": 17301}, "log_uuid": "0191e180-7d60-7000-8e0f-6b2abe4360d8"}`, string(respBody))

	// if fetching attachment from channel returns non-200, return unavailable attachment so caller doesn't retry
	statusCode, respBody = submit(`{"channel_uuid": "e4bb1578-29da-4fa5-a214-9da19dd24230", "channel_type": "MCK", "url": "http://mock.com/media/hello.mp3"}`, "sesame")
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{"attachment": {"content_type": "unavailable", "url": "http://mock.com/media/hello.mp3", "size": 0}, "log_uuid": "0191e180-8148-7000-b92b-40dae35f038b"}`, string(respBody))

	// same if fetching attachment times out
	statusCode, respBody = submit(`{"channel_uuid": "e4bb1578-29da-4fa5-a214-9da19dd24230", "channel_type": "MCK", "url": "http://mock.com/media/hello.pdf"}`, "sesame")
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{"attachment": {"content_type": "unavailable", "url": "http://mock.com/media/hello.pdf", "size": 0}, "log_uuid": "0191e180-8530-7000-8920-17713634b9f5"}`, string(respBody))

	// and channel logs should have been written for the fetches
	require.Eventually(t, func() bool {
		count := 0
		for _, item := range dyntest.ScanAll(t, rt.Dynamo.Main.Client(), rt.Dynamo.Main.Table()) {
			if strings.HasPrefix(item.Key.SK, "log#") && item.Data["type"] == "attachment_fetch" {
				count++
			}
		}
		return count == 3
	}, 5*time.Second, 100*time.Millisecond)
}

// TestListeners verifies that internet and internal endpoints are correctly split between
// the two listener ports.
func TestListeners(t *testing.T) {
	rt := serverRuntime(t)
	rt.Config.AuthToken = "sesame"

	server := web.NewServer(rt)
	require.NoError(t, server.Start())
	defer server.Stop()

	testsuite.InsertChannel(t, rt, test.NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230", "MCK", "2020", "US", []string{urns.Phone.Prefix}, nil))

	const internetURL = "http://localhost:8180"
	const internalURL = "http://localhost:8181"

	// don't follow redirects so we can observe StripSlashes redirects directly
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	tcs := []struct {
		label  string
		method string
		url    string
		status int
		body   string // asserted as JSON when non-empty
	}{
		// internet listener: health at /, /c/*
		{"internet: health", "GET", internetURL + "/", 200, `{"component": "courier", "listener": "internet", "version": "Dev"}`},
		{"internet: channel route (bad params)", "GET", internetURL + "/c/mck/e4bb1578-29da-4fa5-a214-9da19dd24230/receive", 400, ""},
		{"internet: internal route not exposed", "POST", internetURL + "/ci/attachment/fetch", 404, ""},
		{"internet: unknown path", "GET", internetURL + "/nope", 404, ""},

		// internal listener: health at /, /ci/*
		{"internal: health", "GET", internalURL + "/", 200, `{"component": "courier", "listener": "internal", "version": "Dev"}`},
		{"internal: internal route (no auth)", "POST", internalURL + "/ci/attachment/fetch", 401, ""},
		{"internal: channel route not exposed", "GET", internalURL + "/c/mck/e4bb1578-29da-4fa5-a214-9da19dd24230/receive", 404, ""},
		{"internal: unknown path", "GET", internalURL + "/nope", 404, ""},
	}

	for _, tc := range tcs {
		req, err := http.NewRequest(tc.method, tc.url, nil)
		require.NoError(t, err, tc.label)

		resp, err := client.Do(req)
		require.NoError(t, err, tc.label)
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err, tc.label)
		resp.Body.Close()

		assert.Equal(t, tc.status, resp.StatusCode, tc.label)
		if tc.body != "" {
			assert.JSONEq(t, tc.body, string(respBody), tc.label)
		}
	}
}
