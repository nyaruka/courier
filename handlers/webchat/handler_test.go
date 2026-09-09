package webchat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gomodule/redigo/redis"
	"github.com/lib/pq"
	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/courier/v26/web"
	"github.com/nyaruka/gocommon/aws/dynamo/dyntest"
	"github.com/nyaruka/gocommon/centrifugo"
	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/dbutil/assertdb"
	"github.com/nyaruka/gocommon/random"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	channelUUID = "0665bf36-4d2e-4c3f-b8a1-9f8e6a5c2d71"
	startURL    = "/c/wch/" + channelUUID + "/start"
	receiveURL  = "/c/wch/" + channelUUID + "/receive"
	historyURL  = "/c/wch/" + channelUUID + "/history"
	uploadURL   = "/c/wch/" + channelUUID + "/upload"

	testChatID = "vM0GGhDrqpTQefIEinK0up3C" // what the secure source seeded below generates
)

var testChannels = []*models.Channel{
	test.NewMockChannel(channelUUID, "WCH", "", "", []string{urns.WebChat.Prefix}, nil),
}

func TestIncoming(t *testing.T) {
	RunIncomingTests(t, testChannels, newHandler, "testdata/incoming.json", nil)
}

// webchat traffic is internal to the platform so its channel logs are never stored - each visitor request
// would otherwise write one
func TestChannelLogsNotStored(t *testing.T) {
	_, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)
	dyntest.Truncate(t, rt.Dynamo.Main.Client(), rt.Dynamo.Main.Table())

	random.SetSecureSource(random.NewSeededSource(1234))
	defer random.SetSecureSource(random.DefaultSecureSource)

	s := web.NewServer(rt)
	testsuite.InsertChannel(t, rt, testChannels[0])
	s.MountHandler(newHandler)

	// capture the in-memory logs of handled requests
	var clogs []*models.ChannelLog
	s.OnRequestHandled(func(ch *models.Channel, evts []channels.Event, clog *models.ChannelLog) { clogs = append(clogs, clog) })

	post := func(path, body string) *httptest.ResponseRecorder {
		req, _ := http.NewRequest(http.MethodPost, "https://localhost"+path, strings.NewReader(body))
		rr := httptest.NewRecorder()
		s.Router().ServeHTTP(rr, req)
		return rr
	}

	assert.Equal(t, 200, post(startURL, `{}`).Code)
	assert.Equal(t, 200, post(receiveURL, `{"chat_id": "`+testChatID+`", "text": "Hello"}`).Code)

	// the logs still exist in memory during handling...
	require.Len(t, clogs, 2)
	assert.Equal(t, models.ChannelLogTypeChatStart, clogs[0].Type)
	assert.Equal(t, models.ChannelLogTypeReceive, clogs[1].Type)

	// ...but are never written to storage
	rt.Dynamo.Main.Flush()
	for _, item := range dyntest.ScanAll(t, rt.Dynamo.Main.Client(), rt.Dynamo.Main.Table()) {
		assert.False(t, strings.HasPrefix(item.Key.SK, "log#"), "unexpected channel log stored: %s", item.Key.SK)
	}
}

func TestStartRateLimit(t *testing.T) {
	_, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)

	const otherChannelUUID = "b81c3f45-2d6e-4a1f-9c72-8e5d0a4b6f13"

	s := web.NewServer(rt)
	testsuite.InsertChannel(t, rt, testChannels[0])
	testsuite.InsertChannel(t, rt, test.NewMockChannel(otherChannelUUID, "WCH", "", "", []string{urns.WebChat.Prefix}, nil))
	s.MountHandler(newHandler)

	startOn := func(chURL, ip string) *httptest.ResponseRecorder {
		req, _ := http.NewRequest(http.MethodPost, "https://localhost"+chURL, strings.NewReader(`{}`))
		req.RemoteAddr = ip
		rr := httptest.NewRecorder()
		s.Router().ServeHTTP(rr, req)
		return rr
	}
	start := func(ip string) *httptest.ResponseRecorder { return startOn(startURL, ip) }

	// an IP can start up to the limit of chats within the window...
	for i := range startLimit {
		assert.Equal(t, 200, start("41.23.45.67:1234").Code, "start %d", i)
	}

	// ...then gets throttled, with the CORS header still on the error so the widget can read it
	rr := start("41.23.45.67:1234")
	assert.Equal(t, 429, rr.Code)
	assert.Contains(t, rr.Body.String(), "rate limit exceeded")
	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))

	// without a contact being created
	var contacts int
	require.NoError(t, rt.DB.Get(&contacts, `SELECT count(*) FROM contacts_contact WHERE uuid != 'a984069d-0008-4d8c-a772-b14a8a6acccc'`))
	assert.Equal(t, startLimit, contacts)

	// but other IPs aren't affected
	assert.Equal(t, 200, start("41.23.45.68:1234").Code)

	// nor is the same IP on a different channel - the limit is scoped per channel
	assert.Equal(t, 200, startOn("/c/wch/"+otherChannelUUID+"/start", "41.23.45.67:1234").Code)

	// and the count expires with the window
	vc := rt.VK.Get()
	defer vc.Close()
	ttl, err := redis.Int(vc.Do("TTL", "chat-starts:"+channelUUID+"|41.23.45.67"))
	require.NoError(t, err)
	assert.Greater(t, ttl, 0)
	assert.LessOrEqual(t, ttl, startLimitWindow)
}

// the framework can't assert response headers or make OPTIONS requests, so CORS support is tested directly
func TestCORS(t *testing.T) {
	_, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)

	s := web.NewServer(rt)
	testsuite.InsertChannel(t, rt, testChannels[0])
	s.MountHandler(newHandler)

	// preflights on all the endpoints are answered without needing the channel
	for _, path := range []string{startURL, receiveURL, historyURL, uploadURL} {
		req, _ := http.NewRequest(http.MethodOptions, "https://localhost"+path, nil)
		rr := httptest.NewRecorder()
		s.Router().ServeHTTP(rr, req)

		assert.Equal(t, 204, rr.Code, path)
		assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"), path)
		assert.Equal(t, "GET, POST, OPTIONS", rr.Header().Get("Access-Control-Allow-Methods"), path)
		assert.Equal(t, "Content-Type", rr.Header().Get("Access-Control-Allow-Headers"), path)
	}

	// actual responses carry the allow-origin header too, including error responses
	req, _ := http.NewRequest(http.MethodPost, "https://localhost"+startURL, strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	s.Router().ServeHTTP(rr, req)
	assert.Equal(t, 200, rr.Code)
	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))

	req, _ = http.NewRequest(http.MethodPost, "https://localhost"+receiveURL, strings.NewReader(`{}`))
	rr = httptest.NewRecorder()
	s.Router().ServeHTTP(rr, req)
	assert.Equal(t, 400, rr.Code)
	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))
}

func TestAllowedDomains(t *testing.T) {
	_, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)

	const cfgChannelUUID = "7d3fb8a2-5c1e-4b9f-a6d4-2e8c0f7b5a19"
	cfgStartURL := "/c/wch/" + cfgChannelUUID + "/start"
	cfgReceiveURL := "/c/wch/" + cfgChannelUUID + "/receive"

	s := web.NewServer(rt)
	testsuite.InsertChannel(t, rt, testChannels[0])
	testsuite.InsertChannel(t, rt, test.NewMockChannel(cfgChannelUUID, "WCH", "", "", []string{urns.WebChat.Prefix},
		map[string]any{"allowed_domains": []string{"example.com", "localhost:3000"}},
	))
	s.MountHandler(newHandler)

	request := func(path, origin string) *httptest.ResponseRecorder {
		req, _ := http.NewRequest(http.MethodPost, "https://localhost"+path, strings.NewReader(`{}`))
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rr := httptest.NewRecorder()
		s.Router().ServeHTTP(rr, req)
		return rr
	}

	// a channel without allowed_domains ignores the origin and keeps the wildcard header
	rr := request(startURL, "https://anywhere.com")
	assert.Equal(t, 200, rr.Code)
	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))

	// an allowed origin gets itself reflected back instead of the wildcard, with Vary: Origin
	rr = request(cfgStartURL, "https://example.com")
	assert.Equal(t, 200, rr.Code)
	assert.Equal(t, "https://example.com", rr.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "Origin", rr.Header().Get("Vary"))

	// host matching is case-insensitive, and entries can carry a port
	rr = request(cfgStartURL, "https://EXAMPLE.com")
	assert.Equal(t, 200, rr.Code)
	assert.Equal(t, "https://EXAMPLE.com", rr.Header().Get("Access-Control-Allow-Origin"))
	rr = request(cfgStartURL, "http://localhost:3000")
	assert.Equal(t, 200, rr.Code)
	assert.Equal(t, "http://localhost:3000", rr.Header().Get("Access-Control-Allow-Origin"))

	// a disallowed origin gets a 403 with no allow-origin header, so the embedding page can't read it either
	for _, origin := range []string{"https://evil.com", "https://sub.example.com", "https://example.com:8080", "null"} {
		rr = request(cfgStartURL, origin)
		assert.Equal(t, 403, rr.Code, origin)
		assert.Contains(t, rr.Body.String(), "origin not allowed", origin)
		assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"), origin)
	}

	// on the receive endpoint too
	rr = request(cfgReceiveURL, "https://evil.com")
	assert.Equal(t, 403, rr.Code)
	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))

	// and on the history endpoint, whose GET requests get the same treatment
	getHistory := func(origin string) *httptest.ResponseRecorder {
		req, _ := http.NewRequest(http.MethodGet, "https://localhost/c/wch/"+cfgChannelUUID+"/history?chat_id=abcdefabcdefabcdefabcdef", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rr := httptest.NewRecorder()
		s.Router().ServeHTTP(rr, req)
		return rr
	}
	rr = getHistory("https://evil.com")
	assert.Equal(t, 403, rr.Code)
	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))

	// with an allowed origin reflected even on an error response, so the widget can read it
	rr = getHistory("https://example.com")
	assert.Equal(t, 400, rr.Code)
	assert.Contains(t, rr.Body.String(), "unknown chat id")
	assert.Equal(t, "https://example.com", rr.Header().Get("Access-Control-Allow-Origin"))

	// without any contacts being created by the blocked starts
	var contacts int
	require.NoError(t, rt.DB.Get(&contacts, `SELECT count(*) FROM contacts_contact WHERE uuid != 'a984069d-0008-4d8c-a772-b14a8a6acccc'`))
	assert.Equal(t, 4, contacts)

	// but a request with no Origin header (a non-browser client) passes, still marked as varying on origin
	rr = request(cfgStartURL, "")
	assert.Equal(t, 200, rr.Code)
	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "Origin", rr.Header().Get("Vary"))

	// and preflights stay permissive - the channel isn't loaded for them, and the POST response is what gates
	// the browser
	req, _ := http.NewRequest(http.MethodOptions, "https://localhost"+cfgStartURL, nil)
	req.Header.Set("Origin", "https://evil.com")
	rr = httptest.NewRecorder()
	s.Router().ServeHTTP(rr, req)
	assert.Equal(t, 204, rr.Code)
	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))
}

func TestHistory(t *testing.T) {
	_, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)

	random.SetSecureSource(random.NewSeededSource(1234))
	defer random.SetSecureSource(random.DefaultSecureSource)

	const otherChannelUUID = "b81c3f45-2d6e-4a1f-9c72-8e5d0a4b6f13"
	otherChannel := test.NewMockChannel(otherChannelUUID, "WCH", "", "", []string{urns.WebChat.Prefix}, nil)

	s := web.NewServer(rt)
	testsuite.InsertChannel(t, rt, testChannels[0])
	testsuite.InsertChannel(t, rt, otherChannel)
	s.MountHandler(newHandler)

	// start a chat to mint the test chat ID and the contact behind it
	req, _ := http.NewRequest(http.MethodPost, "https://localhost"+startURL, strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	s.Router().ServeHTTP(rr, req)
	require.Equal(t, 200, rr.Code)

	var contactID, urnID int64
	require.NoError(t, rt.DB.Get(&contactID, `SELECT contact_id FROM contacts_contacturn WHERE identity = $1`, "webchat:"+testChatID))
	require.NoError(t, rt.DB.Get(&urnID, `SELECT id FROM contacts_contacturn WHERE identity = $1`, "webchat:"+testChatID))

	insertMsg := func(uuid, direction, status, visibility, text string, attachments []string, quickReplies *string, createdOn time.Time, channel *models.Channel, contID, cURNID int64) {
		rt.DB.MustExec(`INSERT INTO msgs_msg(uuid, text, attachments, quickreplies, created_on, modified_on, direction, status, visibility, msg_type, is_android, high_priority, msg_count, error_count, channel_id, contact_id, contact_urn_id, org_id)
			VALUES($1, $2, $3, $4, $5, $5, $6, $7, $8, 'T', FALSE, FALSE, 1, 0, $9, $10, $11, 1)`,
			uuid, text, pq.StringArray(attachments), quickReplies, createdOn, direction, status, visibility, channel.ID(), contID, cURNID)
	}

	get := func(query string) *httptest.ResponseRecorder {
		req, _ := http.NewRequest(http.MethodGet, "https://localhost"+historyURL+query, nil)
		rr := httptest.NewRecorder()
		s.Router().ServeHTTP(rr, req)
		return rr
	}

	day := time.Date(2025, 10, 13, 0, 0, 0, 0, time.UTC)
	// includes a non-text quick reply which should be filtered out of the history event
	quickReplies := `[{"type": "text", "text": "Yes"}, {"type": "text", "text": "No"}, {"type": "url", "text": "More", "extra": "https://example.com"}]`

	// the conversation: a plain outgoing message, an outgoing one with attachments and quick replies, and an
	// incoming reply
	insertMsg("11f0a1d2-0000-7000-8000-200000000001", "O", "W", "V", "Hello", nil, nil, day.Add(11*time.Hour+1*time.Minute), testChannels[0], contactID, urnID)
	insertMsg("11f0a1d2-0000-7000-8000-200000000002", "O", "W", "V", "Pick one", []string{"image/jpeg:https://example.com/cat.jpg"}, &quickReplies, day.Add(11*time.Hour+2*time.Minute), testChannels[0], contactID, urnID)
	insertMsg("11f0a1d2-0000-7000-8000-200000000003", "I", "P", "V", "Hi there", nil, nil, day.Add(11*time.Hour+3*time.Minute), testChannels[0], contactID, urnID)

	// a second chat URN belonging to the same contact, to check history is scoped to a conversation rather
	// than a contact
	var otherURNID int64
	require.NoError(t, rt.DB.Get(&otherURNID,
		`INSERT INTO contacts_contacturn(identity, path, scheme, priority, contact_id, org_id)
		      VALUES('webchat:aaaabbbbccccddddeeeeffff', 'aaaabbbbccccddddeeeeffff', 'webchat', 50, $1, 1) RETURNING id`, contactID))

	// messages that shouldn't appear: deleted, archived, this chat on a different channel, the same contact's
	// other chat, and a different contact
	insertMsg("11f0a1d2-0000-7000-8000-000000000004", "I", "P", "D", "Deleted", nil, nil, day.Add(11*time.Hour+4*time.Minute), testChannels[0], contactID, urnID)
	insertMsg("11f0a1d2-0000-7000-8000-000000000005", "O", "W", "A", "Archived", nil, nil, day.Add(11*time.Hour+5*time.Minute), testChannels[0], contactID, urnID)
	insertMsg("11f0a1d2-0000-7000-8000-000000000006", "O", "W", "V", "Other channel", nil, nil, day.Add(11*time.Hour+6*time.Minute), otherChannel, contactID, urnID)
	insertMsg("11f0a1d2-0000-7000-8000-000000000007", "O", "W", "V", "Other chat", nil, nil, day.Add(11*time.Hour+7*time.Minute), testChannels[0], contactID, otherURNID)
	insertMsg("11f0a1d2-0000-7000-8000-000000000008", "O", "W", "V", "Other contact", nil, nil, day.Add(11*time.Hour+8*time.Minute), testChannels[0], 100, 1000)

	// the whole conversation fits in one page, newest first, with outgoing messages shaped exactly like the
	// msg_out socket events and incoming ones as msg_in
	rr = get("?chat_id=" + testChatID)
	assert.Equal(t, 200, rr.Code)
	assert.JSONEq(t, `{
		"events": [
			{"type": "msg_in", "created_on": "2025-10-13T11:03:00Z", "msg_uuid": "11f0a1d2-0000-7000-8000-200000000003", "text": "Hi there"},
			{
				"type": "msg_out",
				"created_on": "2025-10-13T11:02:00Z",
				"msg_uuid": "11f0a1d2-0000-7000-8000-200000000002",
				"text": "Pick one",
				"attachments": ["image/jpeg:https://example.com/cat.jpg"],
				"quick_replies": [{"type": "text", "text": "Yes"}, {"type": "text", "text": "No"}]
			},
			{"type": "msg_out", "created_on": "2025-10-13T11:01:00Z", "msg_uuid": "11f0a1d2-0000-7000-8000-200000000001", "text": "Hello"}
		]
	}`, rr.Body.String())

	// add enough older messages - older UUIDs, since v7 UUID order is message order - that the conversation no
	// longer fits in one page
	oldUUIDs := make([]string, 25)
	for i := range 25 {
		oldUUIDs[i] = fmt.Sprintf("11f0a1d2-0000-7000-8000-1000000000%02d", i)
		insertMsg(oldUUIDs[i], "I", "P", "V", fmt.Sprintf("Old %d", i), nil, nil, day.Add(10*time.Hour+time.Duration(i)*time.Minute), testChannels[0], contactID, urnID)
	}

	type page struct {
		Events []struct {
			Type      string    `json:"type"`
			CreatedOn time.Time `json:"created_on"`
			Text      string    `json:"text"`
		} `json:"events"`
		Next string `json:"next"`
	}

	// the first page is the newest 25 messages, with the oldest one's UUID as the cursor to the rest
	rr = get("?chat_id=" + testChatID)
	assert.Equal(t, 200, rr.Code)
	p1 := &page{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), p1))
	require.Len(t, p1.Events, 25)
	assert.Equal(t, "Hi there", p1.Events[0].Text)
	assert.Equal(t, "Old 3", p1.Events[24].Text)
	assert.Equal(t, oldUUIDs[3], p1.Next)

	// which fetches the remaining messages, which don't fill a page so there's no further cursor
	rr = get("?chat_id=" + testChatID + "&before=" + p1.Next)
	assert.Equal(t, 200, rr.Code)
	p2 := &page{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), p2))
	require.Len(t, p2.Events, 3)
	assert.Equal(t, "Old 2", p2.Events[0].Text)
	assert.Equal(t, "Old 1", p2.Events[1].Text)
	assert.Equal(t, "Old 0", p2.Events[2].Text)
	assert.Empty(t, p2.Next)

	// a chat ID we never minted is a bad request, as is a missing or malformed one, or a malformed cursor
	rr = get("?chat_id=xxxxxhDrqpTQefIEinK0up3C")
	assert.Equal(t, 400, rr.Code)
	assert.Contains(t, rr.Body.String(), "unknown chat id")
	rr = get("")
	assert.Equal(t, 400, rr.Code)
	assert.Contains(t, rr.Body.String(), "invalid chat id")
	for _, before := range []string{"yesterday", "11f0a1d2", "11f0a1d2-0000-7000-8000-10000000000x"} {
		rr = get("?chat_id=" + testChatID + "&before=" + before)
		assert.Equal(t, 400, rr.Code, before)
		assert.Contains(t, rr.Body.String(), "invalid before parameter", before)
	}
}

func TestHistoryRateLimit(t *testing.T) {
	_, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)

	random.SetSecureSource(random.NewSeededSource(1234))
	defer random.SetSecureSource(random.DefaultSecureSource)

	s := web.NewServer(rt)
	testsuite.InsertChannel(t, rt, testChannels[0])
	s.MountHandler(newHandler)

	req, _ := http.NewRequest(http.MethodPost, "https://localhost"+startURL, strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	s.Router().ServeHTTP(rr, req)
	require.Equal(t, 200, rr.Code)

	get := func(chatID string) *httptest.ResponseRecorder {
		req, _ := http.NewRequest(http.MethodGet, "https://localhost"+historyURL+"?chat_id="+chatID, nil)
		rr := httptest.NewRecorder()
		s.Router().ServeHTTP(rr, req)
		return rr
	}

	// a chat can fetch history up to the limit of requests within the window...
	for i := range historyLimit {
		assert.Equal(t, 200, get(testChatID).Code, "request %d", i)
	}

	// ...then gets throttled, with the CORS header still on the error so the widget can read it
	rr = get(testChatID)
	assert.Equal(t, 429, rr.Code)
	assert.Contains(t, rr.Body.String(), "rate limit exceeded")
	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))

	// but other chats aren't affected - the limit is per chat (an unknown one just fails its lookup)
	assert.Equal(t, 400, get("xxxxxhDrqpTQefIEinK0up3C").Code)

	// and the count expires with the window
	vc := rt.VK.Get()
	defer vc.Close()
	ttl, err := redis.Int(vc.Do("TTL", "chat-history:"+channelUUID+"|"+testChatID))
	require.NoError(t, err)
	assert.Greater(t, ttl, 0)
	assert.LessOrEqual(t, ttl, historyLimitWindow)
}

// makeUpload posts a multipart upload request - a nil file omits the file part entirely
func makeUpload(t *testing.T, s *web.Server, chatID string, file []byte) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	require.NoError(t, mw.WriteField("chat_id", chatID))
	if file != nil {
		fw, err := mw.CreateFormFile("file", "upload.bin")
		require.NoError(t, err)
		_, err = fw.Write(file)
		require.NoError(t, err)
	}
	require.NoError(t, mw.Close())

	req, _ := http.NewRequest(http.MethodPost, "https://localhost"+uploadURL, bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rr := httptest.NewRecorder()
	s.Router().ServeHTTP(rr, req)
	return rr
}

func TestUpload(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)

	random.SetSecureSource(random.NewSeededSource(1234))
	defer random.SetSecureSource(random.DefaultSecureSource)

	s := web.NewServer(rt)
	testsuite.InsertChannel(t, rt, testChannels[0])
	s.MountHandler(newHandler)

	// uploads are saved to attachment storage so ensure the bucket exists
	rt.S3.Client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(rt.Config.S3AttachmentsBucket)})

	// start a chat to mint the test chat ID and the contact behind it
	req, _ := http.NewRequest(http.MethodPost, "https://localhost"+startURL, strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	s.Router().ServeHTTP(rr, req)
	require.Equal(t, 200, rr.Code)

	testJPG := test.ReadFile("../../test/testdata/test.jpg")

	// a valid upload returns the stored attachment as a content-type:url value, typed by sniffing the bytes
	// and stored under this channel's workspace
	rr = makeUpload(t, s, testChatID, testJPG)
	assert.Equal(t, 200, rr.Code)
	resp := &struct {
		Attachment string `json:"attachment"`
	}{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), resp))
	prefix := "image/jpeg:http://localstack:4566/test-attachments/attachments/1/"
	assert.True(t, strings.HasPrefix(resp.Attachment, prefix), resp.Attachment)
	assert.True(t, strings.HasSuffix(resp.Attachment, ".jpg"), resp.Attachment)

	// whose URL serves back exactly what was uploaded
	storageURL := strings.TrimPrefix(resp.Attachment, "image/jpeg:")
	key := strings.TrimPrefix(storageURL, "http://localstack:4566/"+rt.Config.S3AttachmentsBucket+"/")
	contentType, body, err := rt.S3.GetObject(ctx, rt.Config.S3AttachmentsBucket, key)
	require.NoError(t, err)
	assert.Equal(t, "image/jpeg", contentType)
	assert.Equal(t, testJPG, body)

	// and whose attachment value the receive endpoint accepts on a message with no text
	req, _ = http.NewRequest(http.MethodPost, "https://localhost"+receiveURL,
		strings.NewReader(`{"chat_id": "`+testChatID+`", "attachments": ["`+resp.Attachment+`"]}`))
	rr = httptest.NewRecorder()
	s.Router().ServeHTTP(rr, req)
	assert.Equal(t, 200, rr.Code)
	assertdb.Query(t, rt.DB, `SELECT count(*) FROM msgs_msg WHERE direction = 'I' AND text = '' AND attachments[1] = $1`, resp.Attachment).Returns(1)

	// a recognized but disallowed file type is rejected, with the CORS header still on the error so the
	// widget can read it
	zip := append([]byte{0x50, 0x4b, 0x03, 0x04}, make([]byte, 100)...)
	rr = makeUpload(t, s, testChatID, zip)
	assert.Equal(t, 400, rr.Code)
	assert.Contains(t, rr.Body.String(), "unsupported file type")
	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))

	// as is a file whose type can't be recognized at all
	rr = makeUpload(t, s, testChatID, []byte("just some text"))
	assert.Equal(t, 400, rr.Code)
	assert.Contains(t, rr.Body.String(), "unsupported file type")

	// an oversize upload is cut off rather than buffered
	rr = makeUpload(t, s, testChatID, make([]byte, maxUploadBytes+uploadFormOverheadBytes+1))
	assert.Equal(t, 413, rr.Code)
	assert.Contains(t, rr.Body.String(), "upload too large")

	// a chat ID we never minted is a bad request, as is a malformed one or a request with no file part
	rr = makeUpload(t, s, "xxxxxhDrqpTQefIEinK0up3C", testJPG)
	assert.Equal(t, 400, rr.Code)
	assert.Contains(t, rr.Body.String(), "unknown chat id")

	rr = makeUpload(t, s, "not-a-chat-id!", testJPG)
	assert.Equal(t, 400, rr.Code)
	assert.Contains(t, rr.Body.String(), "invalid chat id")

	rr = makeUpload(t, s, testChatID, nil)
	assert.Equal(t, 400, rr.Code)
	assert.Contains(t, rr.Body.String(), "missing file part")
}

func TestUploadRateLimit(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)

	random.SetSecureSource(random.NewSeededSource(1234))
	defer random.SetSecureSource(random.DefaultSecureSource)

	s := web.NewServer(rt)
	testsuite.InsertChannel(t, rt, testChannels[0])
	s.MountHandler(newHandler)

	rt.S3.Client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(rt.Config.S3AttachmentsBucket)})

	req, _ := http.NewRequest(http.MethodPost, "https://localhost"+startURL, strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	s.Router().ServeHTTP(rr, req)
	require.Equal(t, 200, rr.Code)

	testJPG := test.ReadFile("../../test/testdata/test.jpg")

	// a chat can upload up to the limit of files within the window...
	for i := range uploadLimit {
		assert.Equal(t, 200, makeUpload(t, s, testChatID, testJPG).Code, "upload %d", i)
	}

	// ...then gets throttled, with the CORS header still on the error so the widget can read it
	rr = makeUpload(t, s, testChatID, testJPG)
	assert.Equal(t, 429, rr.Code)
	assert.Contains(t, rr.Body.String(), "rate limit exceeded")
	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))

	// but other chats aren't affected - the limit is per chat (an unknown one just fails its lookup)
	assert.Equal(t, 400, makeUpload(t, s, "xxxxxhDrqpTQefIEinK0up3C", testJPG).Code)

	// and the count expires with the window
	vc := rt.VK.Get()
	defer vc.Close()
	ttl, err := redis.Int(vc.Do("TTL", "chat-uploads:"+channelUUID+"|"+testChatID))
	require.NoError(t, err)
	assert.Greater(t, ttl, 0)
	assert.LessOrEqual(t, ttl, uploadLimitWindow)
}

// sends don't make HTTP requests so the framework's outgoing cases don't fit - instead we test the socket
// publishes directly
func TestOutgoing(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)

	dates.SetNowFunc(dates.NewFixedNow(time.Date(2025, 10, 13, 11, 20, 30, 0, time.UTC)))
	defer dates.SetNowFunc(time.Now)

	ch := testChannels[0]
	testsuite.InsertChannel(t, rt, ch)

	h := newHandler(rt, channels.NewRoutes())

	msg := &models.MsgOut{
		OrgID_:       ch.OrgID(),
		UUID_:        "0191e180-7d60-7000-aded-7d8b151cbd5b",
		Contact_:     &models.ContactReference{ID: 100, UUID: "a984069d-0008-4d8c-a772-b14a8a6acccc"},
		URN_:         urns.URN("webchat:" + testChatID),
		Text_:        "Hello there",
		ChannelUUID_: ch.UUID(),
		Channel_:     ch,
	}

	socket := models.ChatSocket(ch.UUID(), testChatID)

	send := func() error {
		clog := models.NewChannelLogForSend(msg, h.RedactValues(ch))
		return h.Send(ctx, msg, &channels.SendResult{}, clog)
	}

	// no subscriber on the socket yet, so the publish is dropped but the send still succeeds
	require.NoError(t, send())
	assert.Empty(t, testsuite.CentrifugoHistory(t, rt, socket))

	// mark the socket subscribed (as the subscribe proxy endpoint would)
	vc := rt.VK.Get()
	_, err := vc.Do("SET", centrifugo.SubscriptionKey(socket), "1")
	vc.Close()
	require.NoError(t, err)

	require.NoError(t, send())

	sent := testsuite.CentrifugoHistory(t, rt, socket)
	require.Len(t, sent, 1)

	// a plain text message publishes just the envelope and text - no attachments or quick replies fields
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(sent[0], &decoded))
	assert.Equal(t, map[string]any{
		"type":       "msg_out",
		"created_on": "2025-10-13T11:20:30Z",
		"msg_uuid":   "0191e180-7d60-7000-aded-7d8b151cbd5b",
		"text":       "Hello there",
	}, decoded)

	// a message with attachments and quick replies includes them in the event - except non-text quick replies,
	// which the widget doesn't render and are filtered out like any other unsupporting channel
	msg.Attachments_ = []string{"image/jpeg:https://example.com/cat.jpg", "audio/mp3:https://example.com/hi.mp3"}
	msg.QuickReplies_ = []models.QuickReply{{Type: "text", Text: "Yes"}, {Type: "url", Text: "More", Extra: "https://example.com"}}

	require.NoError(t, send())

	sent = testsuite.CentrifugoHistory(t, rt, socket)
	require.Len(t, sent, 2)

	decoded = map[string]any{}
	require.NoError(t, json.Unmarshal(sent[1], &decoded))
	assert.Equal(t, map[string]any{
		"type":        "msg_out",
		"created_on":  "2025-10-13T11:20:30Z",
		"msg_uuid":    "0191e180-7d60-7000-aded-7d8b151cbd5b",
		"text":        "Hello there",
		"attachments": []any{"image/jpeg:https://example.com/cat.jpg", "audio/mp3:https://example.com/hi.mp3"},
		"quick_replies": []any{
			map[string]any{"type": "text", "text": "Yes"},
		},
	}, decoded)

	// a publish failure is returned as a send error
	rt.Centrifugo.Client.(*centrifugo.MockClient).SetError(errors.New("boom"))
	assert.EqualError(t, send(), "error publishing message event: boom")
}

func TestStartAtContactLimit(t *testing.T) {
	_, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)

	defer testsuite.ResetDB(t, rt)

	// org 1 has one contact so record a count for it and cap the org at that
	rt.DB.MustExec(`INSERT INTO contacts_contactgroupcount(group_id, count, is_squashed) VALUES(1, 1, TRUE)`)
	rt.DB.MustExec(`UPDATE orgs_org SET limits = '{"contacts": 1}' WHERE id = 1`)
	models.FlushChannelCache()
	models.FlushContactCounts()

	s := web.NewServer(rt)
	testsuite.InsertChannel(t, rt, testChannels[0])
	s.MountHandler(newHandler)

	req, _ := http.NewRequest(http.MethodPost, "https://localhost"+startURL, strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	s.Router().ServeHTTP(rr, req)

	// a visitor can't start a chat because there's no room for their contact
	assert.Equal(t, 422, rr.Code)
	assert.JSONEq(t, `{"message":"Error","data":[{"type":"error","error":"workspace has reached its limit of 1 contacts"}]}`, rr.Body.String())
	assertdb.Query(t, rt.DB, `SELECT count(*) FROM contacts_contact WHERE org_id = 1`).Returns(1)
}
