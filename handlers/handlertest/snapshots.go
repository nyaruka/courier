package handlertest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/svclogs"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/stretchr/testify/require"
)

// Handler test cases live in JSON files alongside the tests, each file holding the cases for one channel
// configuration. A case is its inputs (the request or message, and any mocked HTTP responses) followed by what the
// handler did with them, which is written by running the tests with -update and checked against on every other run.
// The types below are the building blocks of those files.

// HTTPBody is a request or response body. In test files a body which is a JSON object or array is written as
// that JSON, and any other body is written as a string. A body can also be written as a string even though it's
// JSON, which preserves its exact bytes - needed when a signature is calculated over them.
type HTTPBody struct {
	data   []byte
	quoted bool // written as a string in the file
}

// NewHTTPBody creates a body from the given bytes
func NewHTTPBody(data []byte) HTTPBody {
	return HTTPBody{data: data}
}

// Bytes returns the bytes of this body
func (b HTTPBody) Bytes() []byte { return b.data }

// IsZero returns whether this body is empty, so that empty bodies are omitted from files
func (b HTTPBody) IsZero() bool { return len(b.data) == 0 }

func (b HTTPBody) MarshalJSON() ([]byte, error) {
	if !b.quoted && isJSONContainer(b.data) {
		return b.data, nil
	}
	return json.Marshal(string(b.data))
}

func (b *HTTPBody) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*b = HTTPBody{data: []byte(s), quoted: true}
	} else if bytes.Equal(data, []byte("null")) {
		*b = HTTPBody{}
	} else {
		*b = HTTPBody{data: data}
	}
	return nil
}

// whether the given bytes are a JSON object or array
func isJSONContainer(b []byte) bool {
	trimmed := bytes.TrimSpace(b)
	return len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') && json.Valid(trimmed)
}

// HandlerResponse is how a handler answered a request
type HandlerResponse struct {
	Status int      `json:"status"`
	Body   HTTPBody `json:"body,omitzero"`
}

// HandlerLog is the channel log a handler wrote, minus the timing and HTTP traces which aren't stable
type HandlerLog struct {
	Type   svclogs.Type     `json:"type"`
	Errors []*svclogs.Error `json:"errors"`
}

// CapturedRequest is an HTTP request a handler made which was answered by a mock. A form encoded body is written as
// its decoded values so that the file stays readable, and a multipart form as its values and files - with the
// boundary dropped from its Content-Type header, because it's generated randomly for each request. The User-Agent
// header is left out because every request carries the same one and a change to it would otherwise touch every file.
type CapturedRequest struct {
	Method  string                   `json:"method"`
	URL     string                   `json:"url"`
	Headers map[string]string        `json:"headers,omitempty"`
	Form    url.Values               `json:"form,omitempty"`
	Files   map[string]*CapturedFile `json:"files,omitempty"`
	Body    HTTPBody                 `json:"body,omitzero"`
}

// CapturedFile is a file part of a multipart form request
type CapturedFile struct {
	Filename    string   `json:"filename"`
	ContentType string   `json:"content_type,omitempty"`
	Body        HTTPBody `json:"body,omitzero"`
}

func captureRequest(r *http.Request) *CapturedRequest {
	c := &CapturedRequest{Method: r.Method, URL: r.URL.String(), Headers: make(map[string]string, len(r.Header))}
	for k := range r.Header {
		if k != "User-Agent" {
			c.Headers[k] = r.Header.Get(k)
		}
	}

	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
	}

	mediaType, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))

	switch mediaType {
	case "application/x-www-form-urlencoded":
		c.Form, _ = url.ParseQuery(string(body))
	case "multipart/form-data":
		c.Headers["Content-Type"] = mediaType

		form, err := multipart.NewReader(bytes.NewReader(body), params["boundary"]).ReadForm(32 << 20)
		if err != nil {
			c.Body = NewHTTPBody(body) // not actually a multipart form so keep the raw body
			break
		}
		c.Form = form.Value
		c.Files = make(map[string]*CapturedFile, len(form.File))
		for name, headers := range form.File {
			f, _ := headers[0].Open()
			data, _ := io.ReadAll(f)
			f.Close()
			c.Files[name] = &CapturedFile{
				Filename:    headers[0].Filename,
				ContentType: headers[0].Header.Get("Content-Type"),
				Body:        NewHTTPBody(data),
			}
		}
	default:
		c.Body = NewHTTPBody(body)
	}
	return c
}

// SendErrorInfo is the error a handler's send returned
type SendErrorInfo struct {
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable,omitempty"`
	Loggable  bool           `json:"loggable,omitempty"`
	LogError  *svclogs.Error `json:"log_error,omitempty"`
}

func newSendErrorInfo(err error) *SendErrorInfo {
	if err == nil {
		return nil
	}
	info := &SendErrorInfo{Message: err.Error()}
	if serr, ok := err.(*channels.SendError); ok {
		info.Retryable = serr.Retryable()
		info.Loggable = serr.Loggable()
		info.LogError = serr.ClogError()
	}
	return info
}

// loads the cases in the given test file into the given slice
func loadTestFile(t *testing.T, path string, cases any) {
	data, err := os.ReadFile(path)
	require.NoError(t, err, "error reading test file %s", path)
	require.NoError(t, json.Unmarshal(data, cases), "error parsing test file %s", path)
}

// writes the given cases back to the given test file
func writeTestFile(t *testing.T, path string, cases any) {
	data, err := jsonx.MarshalPretty(cases)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(data, '\n'), 0644), "error writing test file %s", path)
}

// checks the actual outcome of a case against what the file says, unless we're updating the file
func assertCase(t *testing.T, i int, label string, expected, actual any) {
	if !test.UpdateSnapshots {
		test.AssertEqualJSON(t, jsonx.MustMarshal(expected), jsonx.MustMarshal(actual), fmt.Sprintf("case %d '%s' mismatch", i, label))
	}
}

// makes time and UUIDs deterministic for the duration of the given test. Both are derived from the case's label
// rather than the run so far, so that the outcome of a case doesn't change when cases before it are added or
// removed, and so that cases in the same file don't generate the same UUIDs - which the database would reject as
// duplicates. Labels must therefore be unique within a file.
func mockTimeAndUUIDs(t *testing.T, label string) {
	hash := fnv.New64a()
	hash.Write([]byte(label))
	seed := int64(hash.Sum64() & (1<<62 - 1)) // keep it positive

	start := time.Date(2025, 10, 13, 11, 20, 0, 0, time.UTC).Add(time.Duration(seed%100000) * time.Minute)
	now := dates.NewSequentialNow(start, time.Second)
	dates.SetNowFunc(now)
	uuids.SetGenerator(uuids.NewSeededGenerator(seed, now))

	t.Cleanup(func() {
		dates.SetNowFunc(time.Now)
		uuids.SetGenerator(uuids.DefaultGenerator)
	})
}

// checks that case labels are unique within a file, which the mocking of time and UUIDs depends on
func requireUniqueLabels(t *testing.T, path string, labels []string) {
	seen := make(map[string]bool, len(labels))
	for _, label := range labels {
		require.False(t, seen[label], "duplicate case label '%s' in test file %s", label, path)
		seen[label] = true
	}
}
