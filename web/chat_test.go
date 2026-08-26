package web_test

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/courier/v26/web"
	"github.com/nyaruka/gocommon/centrifugo"
	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatSubscribe(t *testing.T) {
	rt := serverRuntime(t)
	rt.Config.AuthToken = "sesame"

	server := web.NewServer(rt)
	require.NoError(t, server.Start())
	defer server.Stop()

	dates.SetNowFunc(dates.NewFixedNow(time.Date(2025, 10, 13, 11, 20, 30, 0, time.UTC)))
	defer dates.SetNowFunc(time.Now)

	const webchatChannelUUID = "0665bf36-4d2e-4c3f-b8a1-9f8e6a5c2d71"
	const otherChannelUUID = "e4bb1578-29da-4fa5-a214-9da19dd24230"
	const chatID = "aB3dE5fG7hJ9kL1mN3pQ5rS7"

	testsuite.InsertChannel(t, rt, test.NewMockChannel(webchatChannelUUID, "WCH", "", "", []string{urns.WebChat.Prefix}, nil))
	testsuite.InsertChannel(t, rt, test.NewMockChannel(otherChannelUUID, "MCK", "2020", "US", []string{urns.Phone.Prefix}, nil))

	// give test contact #100 a webchat URN as the start endpoint would have
	rt.DB.MustExec(`INSERT INTO contacts_contacturn("identity", "path", "scheme", "priority", "contact_id", "org_id")
		VALUES($1, $2, 'webchat', 50, 100, 1)`, "webchat:"+chatID, chatID)

	submit := func(path, body, authToken string) (int, []byte) {
		req, _ := http.NewRequest("POST", "http://localhost:8181"+path, strings.NewReader(body))
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

	socketTTL := func(socket string) int {
		rc := rt.VK.Get()
		defer rc.Close()
		ttl, err := redis.Int(rc.Do("TTL", centrifugo.SubscriptionKey(socket)))
		require.NoError(t, err)
		return ttl
	}

	validSocket := fmt.Sprintf("chat:%s:%s", webchatChannelUUID, chatID)

	// both endpoints require internal auth
	statusCode, respBody := submit("/ci/chat/subscribe", `{"channel": "`+validSocket+`"}`, "")
	assert.Equal(t, 401, statusCode)
	assert.Equal(t, "Unauthorized", string(respBody))
	statusCode, _ = submit("/ci/chat/sub_refresh", `{"channel": "`+validSocket+`"}`, "wrong")
	assert.Equal(t, 401, statusCode)

	// an authorized subscribe gets an expire_at one window ahead and records the presence key with its TTL
	statusCode, respBody = submit("/ci/chat/subscribe", `{"channel": "`+validSocket+`"}`, "sesame")
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{"result": {"expire_at": 1760354490}}`, string(respBody)) // 2025-10-13T11:20:30Z + 60s
	assert.Equal(t, 150, socketTTL(validSocket))

	// as does an authorized sub_refresh
	statusCode, respBody = submit("/ci/chat/sub_refresh", `{"channel": "`+validSocket+`"}`, "sesame")
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{"result": {"expire_at": 1760354490}}`, string(respBody))
	assert.Equal(t, 150, socketTTL(validSocket))

	// everything else is denied - subscribes with a forbidden error, refreshes as expired
	denied := []struct {
		label  string
		socket string
	}{
		{"unknown channel", "chat:8eb23e93-5ecb-45ba-b726-3b064e0c568c:" + chatID},
		{"channel of another type", fmt.Sprintf("chat:%s:%s", otherChannelUUID, chatID)},
		{"unknown chat id", fmt.Sprintf("chat:%s:Xx3dE5fG7hJ9kL1mN3pQ5rXX", webchatChannelUUID)},
		{"wrong namespace", fmt.Sprintf("history:%s:%s", webchatChannelUUID, chatID)},
		{"missing chat id", "chat:" + webchatChannelUUID},
		{"extra segment", fmt.Sprintf("chat:%s:%s:extra", webchatChannelUUID, chatID)},
		{"chat id too short", fmt.Sprintf("chat:%s:abc123", webchatChannelUUID)},
		{"chat id with invalid chars", fmt.Sprintf("chat:%s:aB3dE5fG7hJ9kL1mN3pQ5r$!", webchatChannelUUID)},
		{"non-canonical channel uuid", fmt.Sprintf("chat:%s:%s", strings.ToUpper(webchatChannelUUID), chatID)},
		{"empty socket", ""},
	}
	for _, tc := range denied {
		statusCode, respBody = submit("/ci/chat/subscribe", `{"channel": "`+tc.socket+`"}`, "sesame")
		assert.Equal(t, 200, statusCode, tc.label)
		assert.JSONEq(t, `{"error": {"code": 403, "message": "forbidden"}}`, string(respBody), tc.label)

		statusCode, respBody = submit("/ci/chat/sub_refresh", `{"channel": "`+tc.socket+`"}`, "sesame")
		assert.Equal(t, 200, statusCode, tc.label)
		assert.JSONEq(t, `{"result": {"expired": true}}`, string(respBody), tc.label)
	}

	// a non-string socket is a denial rather than a 500
	statusCode, respBody = submit("/ci/chat/subscribe", `{"channel": 123}`, "sesame")
	assert.Equal(t, 400, statusCode)

	// deactivating the channel revokes future refreshes
	rt.DB.MustExec(`UPDATE channels_channel SET is_active = FALSE WHERE uuid = $1`, webchatChannelUUID)
	models.FlushChannelCache()

	statusCode, respBody = submit("/ci/chat/sub_refresh", `{"channel": "`+validSocket+`"}`, "sesame")
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{"result": {"expired": true}}`, string(respBody))
}
