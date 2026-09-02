package handlertest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/web"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RequestPrepFunc is our type for a hook for tests to use before a request is fired in a test
type RequestPrepFunc func(*http.Request)

func makeHandlerRequest(t *testing.T, s *web.Server, path string, headers map[string]string, data string, multipartFormFields map[string]string, requestPrepFunc RequestPrepFunc) (int, []byte) {
	var req *http.Request
	var err error
	url := fmt.Sprintf("https://%s%s", s.Runtime().Config.Domain, path)

	if data != "" {
		req, err = http.NewRequest(http.MethodPost, url, strings.NewReader(data))
		require.Nil(t, err)

		// guess our content type
		contentType := "application/x-www-form-urlencoded"
		if strings.Contains(data, "{") && strings.Contains(data, "}") {
			contentType = "application/json"
		} else if strings.Contains(data, "<") && strings.Contains(data, ">") {
			contentType = "application/xml"
		}
		req.Header.Set("Content-Type", contentType)
	} else if multipartFormFields != nil {
		var body bytes.Buffer
		bodyMultipartWriter := multipart.NewWriter(&body)
		for k, v := range multipartFormFields {
			fieldWriter, err := bodyMultipartWriter.CreateFormField(k)
			require.Nil(t, err)
			_, err = fieldWriter.Write([]byte(v))
			require.Nil(t, err)
		}
		contentType := fmt.Sprintf("multipart/form-data;boundary=%v", bodyMultipartWriter.Boundary())
		bodyMultipartWriter.Close()

		req, err = http.NewRequest(http.MethodPost, url, bytes.NewReader(body.Bytes()))
		require.Nil(t, err)
		req.Header.Set("Content-Type", contentType)
	} else {
		req, err = http.NewRequest(http.MethodGet, url, nil)
	}

	for key, val := range headers {
		req.Header.Set(key, val)
	}

	require.Nil(t, err)

	if requestPrepFunc != nil {
		requestPrepFunc(req)
	}

	rr := httptest.NewRecorder()
	s.Router().ServeHTTP(rr, req)

	return rr.Code, rr.Body.Bytes()
}

// noNetworkTransport fails every request, so that a call a handler makes which its case hasn't mocked is caught here
// rather than reaching a real API
type noNetworkTransport struct{}

func (t noNetworkTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("handler tests can't make unmocked requests: %s %s", r.Method, r.URL)
}

// gives the runtime a single HTTP client, shared by all three of its clients so that a case's transport intercepts
// every request a handler makes via any of them
func installTestClient(rt *runtime.Runtime) *http.Client {
	client := &http.Client{Transport: httpx.WithTraces(noNetworkTransport{}), Timeout: 30 * time.Second}
	rt.HTTP.Default = client
	rt.HTTP.Proxied = client
	rt.HTTP.Attachments = client
	return client
}

// gives the client the transport for a case: one answering with the case's mocks if it has any, and otherwise one
// which fails every request. Tracing is wrapped around either, since that's what produces the channel logs.
func setCaseTransport(client *http.Client, mocks map[string][]*httpx.MockResponse) *httpx.MocksTransport {
	if len(mocks) == 0 {
		client.Transport = httpx.WithTraces(noNetworkTransport{})
		return nil
	}
	mockHTTP := httpx.WithMocks(nil, mocks)
	client.Transport = httpx.WithTraces(mockHTTP)
	return mockHTTP
}

func newServer(rt *runtime.Runtime) *web.Server {
	rt.Config.FacebookWebhookSecret = "fb_webhook_secret"
	rt.Config.FacebookApplicationSecret = "fb_app_secret"
	rt.Config.WhatsappAdminSystemUserToken = "wac_admin_system_user_token"

	return web.NewServer(rt)
}

func contactQueueKey(ch *models.Channel, msg *models.MsgOut) string {
	return fmt.Sprintf("c:%d:%d", ch.OrgID(), msg.Contact_.ID)
}

// returns the new URNs of the contact_changed tasks queued for the contact of the given message, which is how a send
// reporting a URN the contact already has can be seen to queue none
func queuedContactChangedURNs(t *testing.T, rt *runtime.Runtime, ch *models.Channel, msg *models.MsgOut) []urns.URN {
	rc := rt.VK.Get()
	defer rc.Close()

	tasks, err := redis.Strings(rc.Do("LRANGE", contactQueueKey(ch, msg), 0, -1))
	require.NoError(t, err)

	newURNs := make([]urns.URN, 0, len(tasks))
	for _, task := range tasks {
		payload := &struct {
			Type string `json:"type"`
			Task struct {
				NewURN struct {
					Value urns.URN `json:"value"`
				} `json:"new_urn"`
			} `json:"task"`
		}{}
		require.NoError(t, json.Unmarshal([]byte(task), payload), "error unmarshaling queued task: %s", task)

		if payload.Type == "contact_changed" {
			newURNs = append(newURNs, payload.Task.NewURN.Value)
		}
	}

	return newURNs
}

// asserts that the given channel log doesn't contain any of the given values
func AssertChannelLogRedaction(t *testing.T, clog *models.ChannelLog, vals []string) {
	assertRedacted := func(s string) {
		for _, v := range vals {
			assert.NotContains(t, s, v, "expected '%s' to not contain redacted value '%s'", s, v)
		}
	}

	for _, h := range clog.HttpLogs {
		assertRedacted(h.URL)
		assertRedacted(h.Request)
		assertRedacted(h.Response)
	}
	for _, e := range clog.Errors {
		assertRedacted(e.Message)
	}
}
