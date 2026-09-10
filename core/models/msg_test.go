package models_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/gocommon/dbutil/assertdb"
	"github.com/nyaruka/gocommon/svclogs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMsgOut(t *testing.T) {
	msgJSON := `{
		"attachments": ["https://foo.bar/image.jpg"],
		"quick_replies": [{"type": "text", "text": "Yes"}, {"type": "text", "text": "No"}],
		"text": "Test message 21",
		"contact": {"id": 100, "uuid": "a984069d-0008-4d8c-a772-b14a8a6acccc"},
		"flow": {"uuid": "9de3663f-c5c5-4c92-9f45-ecbc09abcc85", "name": "Favorites"},
		"id": 204,
		"channel_uuid": "f3ad3eb6-d00d-4dc3-92e9-9f34f32940ba",
		"uuid": "54c893b9-b026-44fc-a490-50aed0361c3f",
		"urn": "telegram:3527065",
		"urn_auth": "5ApPVsFDcFt:RZdK9ne7LgfvBYdtCYg7tv99hC9P2",
		"org_id": 1,
		"origin": "flow",
		"created_on": "2017-07-21T19:22:23.242757Z",
		"high_priority": true,
		"response_to_external_id": "external-id",
		"is_resend": true
	}`

	msg := &models.MsgOut{}
	err := json.Unmarshal([]byte(msgJSON), msg)
	assert.NoError(t, err)
	assert.Equal(t, models.ContactID(100), msg.Contact_.ID)
	assert.Equal(t, models.ContactUUID("a984069d-0008-4d8c-a772-b14a8a6acccc"), msg.Contact_.UUID)
	assert.Equal(t, models.ChannelUUID("f3ad3eb6-d00d-4dc3-92e9-9f34f32940ba"), msg.ChannelUUID_)
	assert.Equal(t, []string{"https://foo.bar/image.jpg"}, msg.Attachments())
	assert.Equal(t, "5ApPVsFDcFt:RZdK9ne7LgfvBYdtCYg7tv99hC9P2", msg.URNAuth_)
	assert.Equal(t, []models.QuickReply{{Type: "text", Text: "Yes"}, {Type: "text", Text: "No"}}, msg.QuickReplies())
	assert.Equal(t, "external-id", msg.ResponseToExternalID())
	assert.True(t, msg.HighPriority())
	assert.True(t, msg.IsResend())
	assert.Equal(t, &models.FlowReference{UUID: "9de3663f-c5c5-4c92-9f45-ecbc09abcc85", Name: "Favorites"}, msg.Flow())

	msgJSONNoQR := `{
		"text": "Test message 21",
		"contact": {"id": 100, "uuid": "a984069d-0008-4d8c-a772-b14a8a6acccc"},
		"id": 204,
		"channel_uuid": "f3ad3eb6-d00d-4dc3-92e9-9f34f32940ba",
		"uuid": "54c893b9-b026-44fc-a490-50aed0361c3f",
		"urn": "telegram:3527065",
		"org_id": 1,
		"origin": "chat",
		"created_on": "2017-07-21T19:22:23.242757Z",
		"high_priority": true,
		"response_to_external_id": null,
		"metadata": null
	}`

	msg = &models.MsgOut{}
	err = json.Unmarshal([]byte(msgJSONNoQR), msg)
	assert.NoError(t, err)
	assert.Nil(t, msg.Attachments())
	assert.Nil(t, msg.QuickReplies())
	assert.Equal(t, "", msg.ResponseToExternalID())
	assert.False(t, msg.IsResend())
	assert.Nil(t, msg.Flow())
}

func TestQuickRepliesToRows(t *testing.T) {
	tcs := []struct {
		replies      []models.QuickReply
		maxRows      int
		maxRowRunes  int
		paddingRunes int
		expected     [][]models.QuickReply
	}{
		{

			replies:      []models.QuickReply{{Text: "OK"}},
			maxRows:      3,
			maxRowRunes:  30,
			paddingRunes: 2,
			expected: [][]models.QuickReply{
				{{Text: "OK"}},
			},
		},
		{
			// all values fit in single row
			replies:      []models.QuickReply{{Text: "Yes"}, {Text: "No"}, {Text: "Maybe"}},
			maxRows:      3,
			maxRowRunes:  30,
			paddingRunes: 2,
			expected: [][]models.QuickReply{
				{{Text: "Yes"}, {Text: "No"}, {Text: "Maybe"}},
			},
		},
		{
			// all values can be their own row
			replies:      []models.QuickReply{{Text: "12345678901234567890"}, {Text: "23456789012345678901"}, {Text: "34567890123456789012"}},
			maxRows:      3,
			maxRowRunes:  25,
			paddingRunes: 2,
			expected: [][]models.QuickReply{
				{{Text: "12345678901234567890"}},
				{{Text: "23456789012345678901"}},
				{{Text: "34567890123456789012"}},
			},
		},
		{
			replies:      []models.QuickReply{{Text: "1234567890"}, {Text: "2345678901"}, {Text: "3456789012"}, {Text: "4567890123"}},
			maxRows:      3,
			maxRowRunes:  25,
			paddingRunes: 1,
			expected: [][]models.QuickReply{
				{{Text: "1234567890"}, {Text: "2345678901"}},
				{{Text: "3456789012"}, {Text: "4567890123"}},
			},
		},
		{
			// we break chars per row limit rather than row limit
			replies:      []models.QuickReply{{Text: "Vanilla"}, {Text: "Chocolate"}, {Text: "Strawberry"}, {Text: "Lemon Sorbet"}, {Text: "Ecuadorian Amazonian Papayas"}, {Text: "Mint"}},
			maxRows:      3,
			maxRowRunes:  30,
			paddingRunes: 2,
			expected: [][]models.QuickReply{
				{{Text: "Vanilla"}, {Text: "Chocolate"}},
				{{Text: "Strawberry"}, {Text: "Lemon Sorbet"}},
				{{Text: "Ecuadorian Amazonian Papayas"}, {Text: "Mint"}},
			},
		},
	}

	for _, tc := range tcs {
		rows := models.QuickRepliesToRows(tc.replies, tc.maxRows, tc.maxRowRunes, tc.paddingRunes)
		assert.Equal(t, tc.expected, rows, "rows mismatch for replies %v", tc.replies)
	}
}

func TestIncomingMsgCheck(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)

	defer testsuite.ResetDB(t, rt)

	ch, err := models.GetChannel(ctx, "KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	require.NoError(t, err)

	// a deployment can replace the check with its own policy on top of the default
	defer func() { models.IncomingMsgCheck = models.CheckDuplicateMsg }()
	models.IncomingMsgCheck = func(ctx context.Context, rt *runtime.Runtime, m *models.MsgIn, clog *models.ChannelLog) error {
		if err := models.CheckDuplicateMsg(ctx, rt, m, clog); err != nil || m.Duplicate_ {
			return err
		}
		if m.Text() == "spam" {
			clog.Error(&svclogs.Error{Code: "test_refused", Message: "Refused by test."})
			return &models.LimitReachedError{Limit: "test messages", Max: 1}
		}
		return nil
	}

	writeMsg := func(text, externalID string) (*models.MsgIn, *models.ChannelLog, error) {
		clog := models.NewChannelLog(models.ChannelLogTypeReceive, ch, nil, nil)
		msg := models.NewIncomingMsg(ch, "tel:+12067799192", text, externalID, clog)
		return msg, clog, models.WriteMsg(ctx, rt, msg, clog)
	}

	// a message the check allows is written
	msg, clog, err := writeMsg("hello", "ext1")
	require.NoError(t, err)
	assert.False(t, msg.Duplicate_)
	assert.Len(t, clog.Errors, 0)
	assertdb.Query(t, rt.DB, `SELECT count(*) FROM msgs_msg WHERE text = 'hello'`).Returns(1)

	// the default still marks a redelivery of it as a duplicate
	dup, clog, err := writeMsg("hello", "ext1")
	require.NoError(t, err)
	assert.True(t, dup.Duplicate_)
	assert.Equal(t, msg.UUID(), dup.UUID())
	assert.Len(t, clog.Errors, 0)
	assertdb.Query(t, rt.DB, `SELECT count(*) FROM msgs_msg WHERE text = 'hello'`).Returns(1)

	// and one the check refuses is neither written nor spooled, with the refusal reported as a limit being reached
	_, clog, err = writeMsg("spam", "ext2")
	assert.EqualError(t, err, "workspace has reached its limit of 1 test messages")
	require.Len(t, clog.Errors, 1)
	assert.Equal(t, "test_refused", clog.Errors[0].Code)
	assertdb.Query(t, rt.DB, `SELECT count(*) FROM msgs_msg WHERE text = 'spam'`).Returns(0)
	spooled, err := filepath.Glob(filepath.Join(rt.Config.SpoolDir, "msgs", "*.jsonl"))
	require.NoError(t, err)
	assert.Empty(t, spooled)
}
