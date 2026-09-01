package handlertest

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// IncomingCase is a request to a handler and what it did with it
type IncomingCase struct {
	Label         string                           `json:"label"`
	URL           string                           `json:"url"`
	Headers       map[string]string                `json:"headers,omitempty"`
	Data          HTTPBody                         `json:"data,omitempty"`
	MultipartForm map[string]string                `json:"multipart_form,omitempty"`
	Unsigned      bool                             `json:"unsigned,omitempty"` // don't sign this request even if the run signs requests
	HTTPMocks     map[string][]*httpx.MockResponse `json:"http_mocks,omitempty"`

	// the outcome, written by running with -update
	Response *HandlerResponse   `json:"response"`
	Msgs     []json.RawMessage  `json:"msgs,omitempty"`
	Statuses []json.RawMessage  `json:"statuses,omitempty"`
	Events   []json.RawMessage  `json:"events,omitempty"`
	Requests []*CapturedRequest `json:"requests,omitempty"`
	Log      *HandlerLog        `json:"log"`
}

// IncomingOptions are the options for running a file of incoming cases
type IncomingOptions struct {
	// Sign signs a request the way the provider would, for handlers which validate signatures. It's applied to
	// every case's request except those marked unsigned, and sees the case's headers - which are then re-applied
	// so that a case can override what it wrote, e.g. with an invalid signature.
	Sign RequestPrepFunc

	// NoInvalidChannelCheck skips the check that the first case's request is rejected if its channel doesn't exist
	NoInvalidChannelCheck bool
}

// returns the function to prepare the given case's request with, if any
func (o *IncomingOptions) signer(tc *IncomingCase) RequestPrepFunc {
	if o.Sign == nil || tc.Unsigned {
		return nil
	}
	return func(r *http.Request) {
		o.Sign(r)

		// the case's own headers win over anything the signer set
		for k, v := range tc.Headers {
			r.Header.Set(k, v)
		}
	}
}

// RunIncomingTests runs the incoming cases in the given file against the given channels
func RunIncomingTests(t *testing.T, chs []*models.Channel, newFn channels.NewHandlerFunc, path string, opts *IncomingOptions) {
	if opts == nil {
		opts = &IncomingOptions{}
	}

	var cases []*IncomingCase
	loadTestFile(t, path, &cases)
	require.NotEmpty(t, cases, "no cases in test file %s", path)
	requireUniqueLabels(t, path, slices.Collect(func(yield func(string) bool) {
		for _, tc := range cases {
			if !yield(tc.Label) {
				return
			}
		}
	}))

	_, rt := testsuite.Runtime(t)

	// state is reset once for the whole run rather than per case because handlers can carry state between
	// requests, e.g. those which join the parts of a multi-part message via valkey
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)

	// cases which mock HTTP get a mocking transport, and the rest fail anything leaving the host so that a handler
	// which isn't pointed at a test server is caught here rather than calling a real channel API. Either way tracing
	// stays wrapped around the transport since that's what produces the channel logs.
	client := &http.Client{Transport: httpx.WithTraces(localOnlyTransport{http.DefaultTransport}), Timeout: 30 * time.Second}
	rt.HTTP.Default = client
	rt.HTTP.Proxied = client
	rt.HTTP.Attachments = client

	// data: attachments are saved to storage as they're received so ensure the bucket exists
	rt.S3.Client.CreateBucket(t.Context(), &s3.CreateBucketInput{Bucket: aws.String(rt.Config.S3AttachmentsBucket)})

	s := newServer(rt)

	for _, ch := range chs {
		testsuite.InsertChannel(t, rt, ch)
	}

	s.MountHandler(newFn)

	// capture the events and channel logs of each handled request
	var handledEvents []channels.Event
	var handledLogs []*models.ChannelLog
	s.OnRequestHandled(func(ch *models.Channel, evts []channels.Event, clog *models.ChannelLog) {
		handledEvents = append(handledEvents, evts...)
		handledLogs = append(handledLogs, clog)
	})

	actuals := make([]*IncomingCase, len(cases))

	for i, tc := range cases {
		// if the case aborts before its outcome is captured, the file keeps what it had for it
		actuals[i] = tc

		t.Run(tc.Label, func(t *testing.T) {
			mockTimeAndUUIDs(t, tc.Label)

			handledEvents = nil
			handledLogs = nil

			var mockHTTP *httpx.MocksTransport
			if len(tc.HTTPMocks) > 0 {
				mockHTTP = httpx.WithMocks(nil, tc.HTTPMocks)
				client.Transport = httpx.WithTraces(mockHTTP)
			} else {
				client.Transport = httpx.WithTraces(localOnlyTransport{http.DefaultTransport})
			}

			status, body := makeHandlerRequest(t, s, tc.URL, tc.Headers, string(tc.Data), tc.MultipartForm, opts.signer(tc))

			actual := *tc
			actual.Response = &HandlerResponse{Status: status, Body: body}
			actual.Msgs, actual.Statuses, actual.Events, actual.Requests, actual.Log = nil, nil, nil, nil, nil

			for _, event := range handledEvents {
				switch e := event.(type) {
				case *models.MsgIn:
					actual.Msgs = append(actual.Msgs, jsonx.MustMarshal(e))
				case *models.StatusUpdate:
					actual.Statuses = append(actual.Statuses, jsonx.MustMarshal(e))
				case *models.ChannelEvent:
					actual.Events = append(actual.Events, jsonx.MustMarshal(e))
				}
			}

			if mockHTTP != nil {
				assert.False(t, mockHTTP.HasUnused(), "unused HTTP mocks")

				for _, r := range mockHTTP.Requests() {
					actual.Requests = append(actual.Requests, captureRequest(r))
				}
			}

			if len(handledLogs) > 0 && handledLogs[0] != nil {
				actual.Log = &HandlerLog{Type: handledLogs[0].Type, Errors: handledLogs[0].Errors}
			}

			actuals[i] = &actual

			assertCase(t, i, tc.Label, tc, &actual)
		})
	}

	if test.UpdateSnapshots {
		writeTestFile(t, path, actuals)
	}

	// check invalid channel condition against first test case
	if !opts.NoInvalidChannelCheck {
		t.Run("Receive With Invalid Channel", func(t *testing.T) {
			for _, ch := range chs {
				rt.DB.MustExec(`DELETE FROM channels_channel WHERE uuid = $1`, ch.UUID())
			}
			models.FlushChannelCache()

			validCase := cases[0]
			status, body := makeHandlerRequest(t, s, validCase.URL, validCase.Headers, string(validCase.Data), validCase.MultipartForm, opts.signer(validCase))
			assert.Equal(t, 400, status, "status code mismatch")
			assert.Contains(t, string(body), "channel not found")
		})
	}
}
