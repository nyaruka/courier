package handlertest

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/svclogs"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// OutgoingMsg is the message an outgoing case sends
type OutgoingMsg struct {
	Text                 string                `json:"text,omitempty"`
	URN                  urns.URN              `json:"urn"`
	URNAuth              string                `json:"urn_auth,omitempty"`
	Attachments          []string              `json:"attachments,omitempty"`
	QuickReplies         []models.QuickReply   `json:"quick_replies,omitempty"`
	Locale               i18n.Locale           `json:"locale,omitempty"`
	Templating           json.RawMessage       `json:"templating,omitempty"`
	HighPriority         bool                  `json:"high_priority,omitempty"`
	ResponseToExternalID string                `json:"response_to_external_id,omitempty"`
	Flow                 *models.FlowReference `json:"flow,omitempty"`
	UserID               models.UserID         `json:"user_id,omitempty"`
	Origin               models.MsgOrigin      `json:"origin,omitempty"`
	ContactLastSeenOn    *time.Time            `json:"contact_last_seen_on,omitempty"`
	ContactOtherURNs     []urns.URN            `json:"contact_other_urns,omitempty"`
}

// builds the actual message to send on the given channel
func (m *OutgoingMsg) build(ch *models.Channel) *models.MsgOut {
	origin := models.MsgOriginFlow
	if m.Origin != "" {
		origin = m.Origin
	}

	contact := &models.ContactReference{ID: 100, UUID: "a984069d-0008-4d8c-a772-b14a8a6acccc", LastSeenOn: m.ContactLastSeenOn, OtherURNs: m.ContactOtherURNs}

	msg := &models.MsgOut{
		OrgID_:                ch.OrgID(),
		UUID_:                 "0191e180-7d60-7000-aded-7d8b151cbd5b",
		Contact_:              contact,
		URN_:                  m.URN,
		URNAuth_:              m.URNAuth,
		Text_:                 m.Text,
		Attachments_:          append([]string{}, m.Attachments...),
		QuickReplies_:         m.QuickReplies,
		Locale_:               m.Locale,
		HighPriority_:         m.HighPriority,
		ResponseToExternalID_: m.ResponseToExternalID,
		Flow_:                 m.Flow,
		UserID_:               m.UserID,
		Origin_:               origin,
		ChannelUUID_:          ch.UUID(),
		Channel_:              ch,
	}

	if len(m.Templating) > 0 {
		msg.Templating_ = &models.Templating{}
		jsonx.MustUnmarshal(m.Templating, msg.Templating_)
	}

	return msg
}

// OutgoingCase is a message sent via a handler and what it did with it
type OutgoingCase struct {
	Label     string                           `json:"label"`
	Msg       *OutgoingMsg                     `json:"msg"`
	HTTPMocks map[string][]*httpx.MockResponse `json:"http_mocks,omitempty"`

	// the outcome, written by running with -update
	Requests    []*CapturedRequest `json:"requests,omitempty"`
	ExternalIDs []string           `json:"external_ids,omitempty"`
	Error       *SendErrorInfo     `json:"error,omitempty"`
	LogErrors   []*svclogs.Error   `json:"log_errors"`
	NewURN      urns.URN           `json:"new_urn,omitempty"` // the URN of the contact_changed task queued, if any
}

// OutgoingOptions are the options for running a file of outgoing cases
type OutgoingOptions struct {
	// CheckRedacted are values which must not appear in channel logs
	CheckRedacted []string

	// Setup is called with the runtime before any cases run
	Setup func(*testing.T, *runtime.Runtime)
}

// RunOutgoingTests runs the outgoing cases in the given file against the given channel
func RunOutgoingTests(t *testing.T, ch *models.Channel, newFn channels.NewHandlerFunc, path string, opts *OutgoingOptions) {
	if opts == nil {
		opts = &OutgoingOptions{}
	}

	var cases []*OutgoingCase
	loadTestFile(t, path, &cases)
	require.NotEmpty(t, cases, "no cases in test file %s", path)
	requireUniqueLabels(t, path, slices.Collect(func(yield func(string) bool) {
		for _, tc := range cases {
			if !yield(tc.Label) {
				return
			}
		}
	}))

	ctx, rt := testsuite.Runtime(t)

	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)

	client := installTestClient(rt)

	if opts.Setup != nil {
		opts.Setup(t, rt)
	}

	s := newServer(rt)
	testsuite.InsertChannel(t, rt, ch)
	handler := s.MountHandler(newFn)

	actuals := make([]*OutgoingCase, len(cases))

	for i, tc := range cases {
		// if the case aborts before its outcome is captured, the file keeps what it had for it
		actuals[i] = tc

		t.Run(tc.Label, func(t *testing.T) {
			mockTimeAndRandomness(t, tc.Label)

			require.NotNil(t, tc.Msg, "case has no msg")
			msg := tc.Msg.build(ch)

			// drop any tasks a previous case queued for this contact so we see only this case's
			rc := rt.VK.Get()
			_, err := rc.Do("DEL", contactQueueKey(ch, msg))
			rc.Close()
			require.NoError(t, err)

			mockHTTP := setCaseTransport(client, tc.HTTPMocks)

			clog := models.NewChannelLogForSend(msg, handler.RedactValues(ch))
			sendCtx, cancel := context.WithTimeout(ctx, time.Millisecond*100)
			defer cancel()

			res := &channels.SendResult{}
			serr := handler.Send(sendCtx, msg, res, clog)

			actual := *tc
			actual.Requests = nil
			actual.ExternalIDs = res.ExternalIDs()
			actual.Error = newSendErrorInfo(serr)
			actual.LogErrors = clog.Errors
			actual.NewURN = ""

			if mockHTTP != nil {
				assert.False(t, mockHTTP.HasUnused(), "unused HTTP mocks")

				for _, r := range mockHTTP.Requests() {
					actual.Requests = append(actual.Requests, captureRequest(r))
				}
			}

			// simulate the sender completing the send so send results (e.g. new URNs) are processed
			status := models.NewStatusUpdate(ch, msg.UUID(), models.MsgStatusWired, clog)
			models.OnSendComplete(ctx, rt, msg, status, res.NewURN(), clog)

			newURNs := queuedContactChangedURNs(t, rt, ch, msg)
			require.LessOrEqual(t, len(newURNs), 1, "more than one contact_changed task queued")
			if len(newURNs) > 0 {
				actual.NewURN = newURNs[0]
			}

			AssertChannelLogRedaction(t, clog, opts.CheckRedacted)

			actuals[i] = &actual

			assertCase(t, i, tc.Label, tc, &actual)
		})
	}

	if test.UpdateSnapshots {
		writeTestFile(t, path, actuals)
	}
}
