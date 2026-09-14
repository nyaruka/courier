package models_test

import (
	"cmp"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/gocommon/aws/dynamo"
	"github.com/nyaruka/gocommon/dbutil/assertdb"
	"github.com/stretchr/testify/assert"
)

func TestWriteStatusUpdates(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)

	defer testsuite.ResetDB(t, rt)

	updates := []*models.StatusUpdate{
		{
			ChannelUUID_: "dbc126ed-66bc-4e28-b67b-81dc3327c95d",
			ChannelID_:   10,
			MsgUUID_:     "0199df0f-9f82-7689-b02d-f34105991321", // message 1
			Status_:      models.MsgStatusSent,
			LogUUID:      "019a6e53-e1e8-7df7-a264-ce2372824e1d",
		},
		{
			ChannelUUID_:        "dbc126ed-66bc-4e28-b67b-81dc3327c95d",
			ChannelID_:          10,
			MsgUUID_:            "0199df10-10dc-7e6e-834b-3d959ece93b2", // message 2
			Status_:             models.MsgStatusErrored,
			LogUUID:             "019a6e54-671f-789a-bbb1-31cddd66c681",
			ExternalIdentifier_: "new-external-id",
		},
		{
			ChannelUUID_: "dbc126ed-66bc-4e28-b67b-81dc3327c95d",
			ChannelID_:   10,
			MsgUUID_:     "019a6e61-a4ce-7e60-86d0-aca6405ddb90", // no such message
			Status_:      models.MsgStatusSent,
			LogUUID:      "019a6e62-81b9-79e5-b654-56e6094692a6",
		},
		{
			ChannelUUID_:        "dbc126ed-66bc-4e28-b67b-81dc3327c95d",
			ChannelID_:          10,
			MsgUUID_:            "019bb29e-b2c6-7e5f-b980-ccb3e9e21fbc", // message 3 - outgoing message
			Status_:             models.MsgStatusSent,
			LogUUID:             "019bb2a0-e472-7689-9f80-cb44bd0c7062",
			ExternalIdentifier_: "new-long-external-id",
		},
	}

	changes, err := models.WriteStatusUpdates(ctx, rt, updates)
	assert.NoError(t, err)
	if assert.Len(t, changes, 3) {
		sort.Slice(changes, func(i, j int) bool { return cmp.Compare(changes[i].MsgUUID, changes[j].MsgUUID) < 0 })

		assert.Equal(t, models.MsgUUID("0199df0f-9f82-7689-b02d-f34105991321"), changes[0].MsgUUID)
		assert.Equal(t, models.MsgStatus("S"), changes[0].MsgStatus)
		assert.Equal(t, "", string(changes[0].FailedReason))
		assert.Equal(t, models.ContactUUID("a984069d-0008-4d8c-a772-b14a8a6acccc"), changes[0].ContactUUID)
		assert.Equal(t, models.MsgUUID("0199df10-10dc-7e6e-834b-3d959ece93b2"), changes[1].MsgUUID)
		assert.Equal(t, models.MsgStatus("E"), changes[1].MsgStatus)
		assert.Equal(t, "", string(changes[1].FailedReason))
		assert.Equal(t, models.ContactUUID("a984069d-0008-4d8c-a772-b14a8a6acccc"), changes[1].ContactUUID)
		assert.Equal(t, models.MsgUUID("019bb29e-b2c6-7e5f-b980-ccb3e9e21fbc"), changes[2].MsgUUID)
		assert.Equal(t, models.MsgStatus("S"), changes[2].MsgStatus)
		assert.Equal(t, "", string(changes[2].FailedReason))
		assert.Equal(t, models.ContactUUID("a984069d-0008-4d8c-a772-b14a8a6acccc"), changes[2].ContactUUID)
	}

	assertdb.Query(t, rt.DB, `SELECT uuid, status FROM msgs_msg`).Map(map[string]any{
		"0199df0f-9f82-7689-b02d-f34105991321": "S",
		"0199df10-10dc-7e6e-834b-3d959ece93b2": "E",
		"0199df10-9519-7fe2-a29c-c890d1713673": "P",
		"019bb1ca-a92d-78f5-ba61-06aa62f2b41a": "P",
		"019bb29e-b2c6-7e5f-b980-ccb3e9e21fbc": "S",
	})

	// folder is written in step with status - sent messages in the sent folder, errored ones back in the outbox, and
	// the incoming messages left alone in pending
	assertdb.Query(t, rt.DB, `SELECT uuid, folder FROM msgs_msg`).Map(map[string]any{
		"0199df0f-9f82-7689-b02d-f34105991321": "S",
		"0199df10-10dc-7e6e-834b-3d959ece93b2": "O",
		"0199df10-9519-7fe2-a29c-c890d1713673": "P",
		"019bb1ca-a92d-78f5-ba61-06aa62f2b41a": "P",
		"019bb29e-b2c6-7e5f-b980-ccb3e9e21fbc": "S",
	})

	assertdb.Query(t, rt.DB, `SELECT uuid::text, status, external_identifier FROM msgs_msg WHERE uuid= '0199df0f-9f82-7689-b02d-f34105991321'`).
		Columns(map[string]any{
			"uuid":                "0199df0f-9f82-7689-b02d-f34105991321",
			"status":              "S",
			"external_identifier": "ext1",
		})

	assertdb.Query(t, rt.DB, `SELECT uuid::text, status, external_identifier FROM msgs_msg WHERE uuid= '0199df10-10dc-7e6e-834b-3d959ece93b2'`).
		Columns(map[string]any{
			"uuid":                "0199df10-10dc-7e6e-834b-3d959ece93b2",
			"status":              "E",
			"external_identifier": "new-external-id",
		})

	assertdb.Query(t, rt.DB, `SELECT uuid::text, status, external_identifier FROM msgs_msg WHERE uuid= '0199df10-9519-7fe2-a29c-c890d1713673'`).
		Columns(map[string]any{
			"uuid":                "0199df10-9519-7fe2-a29c-c890d1713673",
			"status":              "P",
			"external_identifier": "ext2",
		})

	assertdb.Query(t, rt.DB, `SELECT uuid::text, status, external_identifier FROM msgs_msg WHERE uuid= '019bb29e-b2c6-7e5f-b980-ccb3e9e21fbc'`).
		Columns(map[string]any{
			"uuid":                "019bb29e-b2c6-7e5f-b980-ccb3e9e21fbc",
			"status":              "S",
			"external_identifier": "new-long-external-id",
		})

	// write another errored status for message 2 - it's still errored so that isn't a change in status
	changes, err = models.WriteStatusUpdates(ctx, rt, []*models.StatusUpdate{
		{
			ChannelUUID_: "dbc126ed-66bc-4e28-b67b-81dc3327c95d",
			ChannelID_:   10,
			MsgUUID_:     "0199df10-10dc-7e6e-834b-3d959ece93b2",
			Status_:      models.MsgStatusErrored,
			LogUUID:      "019a6e53-e1e8-7df7-a264-ce2372824e1d",
		},
	})
	assert.NoError(t, err)
	assert.Len(t, changes, 0)

	// still errored so still in the outbox, but the attempt was counted
	assertdb.Query(t, rt.DB, `SELECT status, folder, error_count FROM msgs_msg WHERE uuid = '0199df10-10dc-7e6e-834b-3d959ece93b2'`).
		Columns(map[string]any{"status": "E", "folder": "O", "error_count": int64(2)})

	// write yet another errored status for message 2 - this should flip it to failed
	changes, err = models.WriteStatusUpdates(ctx, rt, []*models.StatusUpdate{
		{
			ChannelUUID_: "dbc126ed-66bc-4e28-b67b-81dc3327c95d",
			ChannelID_:   10,
			MsgUUID_:     "0199df10-10dc-7e6e-834b-3d959ece93b2",
			Status_:      models.MsgStatusErrored,
			LogUUID:      "019a6e53-e1e8-7df7-a264-ce2372824e1d",
		},
	})
	assert.NoError(t, err)
	if assert.Len(t, changes, 1) {
		assert.Equal(t, models.MsgUUID("0199df10-10dc-7e6e-834b-3d959ece93b2"), changes[0].MsgUUID)
		assert.Equal(t, models.MsgStatus("F"), changes[0].MsgStatus)
		assert.Equal(t, "E", string(changes[0].FailedReason))
	}

	// the errored status flipped the message to failed so its folder has to have followed it there
	assertdb.Query(t, rt.DB, `SELECT status, folder FROM msgs_msg WHERE uuid = '0199df10-10dc-7e6e-834b-3d959ece93b2'`).
		Columns(map[string]any{"status": "F", "folder": "X"})

	// and an update on an already failed message keeps it failed and in the failed folder
	_, err = models.WriteStatusUpdates(ctx, rt, []*models.StatusUpdate{
		{
			ChannelUUID_: "dbc126ed-66bc-4e28-b67b-81dc3327c95d",
			ChannelID_:   10,
			MsgUUID_:     "0199df10-10dc-7e6e-834b-3d959ece93b2",
			Status_:      models.MsgStatusErrored,
			LogUUID:      "019a6e53-e1e8-7df7-a264-ce2372824e1d",
		},
	})
	assert.NoError(t, err)

	assertdb.Query(t, rt.DB, `SELECT status, folder FROM msgs_msg WHERE uuid = '0199df10-10dc-7e6e-834b-3d959ece93b2'`).
		Columns(map[string]any{"status": "F", "folder": "X"})

	// courier never writes a NULL folder for a message it touches
	assertdb.Query(t, rt.DB, `SELECT count(*) FROM msgs_msg WHERE folder IS NULL`).Returns(0)
}

func TestStatusChanges(t *testing.T) {
	createdOn := time.Date(2025, 11, 10, 16, 14, 30, 123456789, time.UTC)
	ttl90 := createdOn.Add(90 * 24 * time.Hour)
	ttl365 := createdOn.Add(365 * 24 * time.Hour)

	// every status is written as its own item, keyed by the status code, so they can never overwrite each other
	change1 := &models.StatusChange{
		ContactUUID: "a984069d-0008-4d8c-a772-b14a8a6acccc",
		MsgUUID:     "0199df10-10dc-7e6e-834b-3d959ece93b2",
		MsgStatus:   models.MsgStatusSent,
		OrgID:       1,
		CreatedOn:   createdOn,
	}

	assert.Equal(t, dynamo.Key{PK: "con#a984069d-0008-4d8c-a772-b14a8a6acccc", SK: "evt#0199df10-10dc-7e6e-834b-3d959ece93b2#sts#S"}, change1.DynamoKey())

	item1, err := change1.MarshalDynamo()
	assert.NoError(t, err)
	assert.Equal(t, &ttl90, item1.TTL)

	marshaled1, err := attributevalue.MarshalMap(item1)
	assert.NoError(t, err)
	assert.Equal(t, map[string]types.AttributeValue{
		"PK":    &types.AttributeValueMemberS{Value: "con#a984069d-0008-4d8c-a772-b14a8a6acccc"},
		"SK":    &types.AttributeValueMemberS{Value: "evt#0199df10-10dc-7e6e-834b-3d959ece93b2#sts#S"},
		"OrgID": &types.AttributeValueMemberN{Value: "1"},
		"TTL":   &types.AttributeValueMemberN{Value: "1770567270"},
		"Data": &types.AttributeValueMemberM{
			Value: map[string]types.AttributeValue{
				"created_on": &types.AttributeValueMemberS{Value: "2025-11-10T16:14:30.123456789Z"},
				"status":     &types.AttributeValueMemberS{Value: "sent"},
			},
		},
	}, marshaled1)

	// failed is terminal so its item is kept forever
	change2 := &models.StatusChange{
		ContactUUID:  "a984069d-0008-4d8c-a772-b14a8a6acccc",
		MsgUUID:      "0199df10-10dc-7e6e-834b-3d959ece93b2",
		MsgStatus:    models.MsgStatusFailed,
		FailedReason: "E",
		OrgID:        1,
		CreatedOn:    createdOn,
	}

	assert.Equal(t, dynamo.Key{PK: "con#a984069d-0008-4d8c-a772-b14a8a6acccc", SK: "evt#0199df10-10dc-7e6e-834b-3d959ece93b2#sts#F"}, change2.DynamoKey())

	item2, err := change2.MarshalDynamo()
	assert.NoError(t, err)
	assert.Nil(t, item2.TTL)

	marshaled2, err := attributevalue.MarshalMap(item2)
	assert.NoError(t, err)

	assert.Equal(t, map[string]types.AttributeValue{
		"PK":    &types.AttributeValueMemberS{Value: "con#a984069d-0008-4d8c-a772-b14a8a6acccc"},
		"SK":    &types.AttributeValueMemberS{Value: "evt#0199df10-10dc-7e6e-834b-3d959ece93b2#sts#F"},
		"OrgID": &types.AttributeValueMemberN{Value: "1"},
		"Data": &types.AttributeValueMemberM{
			Value: map[string]types.AttributeValue{
				"created_on": &types.AttributeValueMemberS{Value: "2025-11-10T16:14:30.123456789Z"},
				"status":     &types.AttributeValueMemberS{Value: "failed"},
				"reason":     &types.AttributeValueMemberS{Value: "error_limit"},
			},
		},
	}, marshaled2)

	// sent-ish statuses expire after 90 days, read after a year, failed never
	for status, expected := range map[models.MsgStatus]*time.Time{
		models.MsgStatusWired:     &ttl90,
		models.MsgStatusSent:      &ttl90,
		models.MsgStatusDelivered: &ttl90,
		models.MsgStatusErrored:   &ttl90,
		models.MsgStatusRead:      &ttl365,
		models.MsgStatusFailed:    nil,
	} {
		change := &models.StatusChange{
			ContactUUID: "a984069d-0008-4d8c-a772-b14a8a6acccc",
			MsgUUID:     "0199df10-10dc-7e6e-834b-3d959ece93b2",
			MsgStatus:   status,
			OrgID:       1,
			CreatedOn:   createdOn,
		}

		assert.Equal(t, "evt#0199df10-10dc-7e6e-834b-3d959ece93b2#sts#"+string(status), change.DynamoKey().SK, "status %s", status)

		item, err := change.MarshalDynamo()
		assert.NoError(t, err)
		assert.Equal(t, expected, item.TTL, "status %s", status)
	}
}

func TestStatusTransitions(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)

	defer testsuite.ResetDB(t, rt)

	const msgUUID = models.MsgUUID("0199df0f-9f82-7689-b02d-f34105991321") // message 1

	tcs := []struct {
		from         models.MsgStatus
		fromErrors   int
		write        models.MsgStatus
		status       models.MsgStatus
		errors       int
		failedReason string
		sentOn       bool
		changed      bool
	}{
		// queued or wired moves on to anything
		{from: "Q", write: "W", status: "W", sentOn: true, changed: true},
		{from: "Q", write: "F", status: "F", sentOn: false, changed: true},
		{from: "W", write: "S", status: "S", sentOn: true, changed: true},
		{from: "W", write: "D", status: "D", sentOn: true, changed: true},
		{from: "W", write: "R", status: "R", sentOn: true, changed: true},
		{from: "W", write: "F", status: "F", sentOn: false, changed: true},
		{from: "W", write: "E", status: "E", errors: 1, sentOn: false, changed: true},
		{from: "W", write: "W", status: "W", sentOn: true, changed: false},

		// sent doesn't go back to wired
		{from: "S", write: "W", status: "S", sentOn: true, changed: false},
		{from: "S", write: "S", status: "S", sentOn: true, changed: false},
		{from: "S", write: "D", status: "D", sentOn: true, changed: true},
		{from: "S", write: "R", status: "R", sentOn: true, changed: true},
		{from: "S", write: "F", status: "F", sentOn: false, changed: true},
		{from: "S", write: "E", status: "E", errors: 1, sentOn: false, changed: true},

		// delivered only moves on to read or failed
		{from: "D", write: "W", status: "D", sentOn: true, changed: false},
		{from: "D", write: "S", status: "D", sentOn: true, changed: false},
		{from: "D", write: "D", status: "D", sentOn: true, changed: false},
		{from: "D", write: "E", status: "D", sentOn: true, changed: false},
		{from: "D", write: "R", status: "R", sentOn: true, changed: true},
		{from: "D", write: "F", status: "F", sentOn: false, changed: true},

		// read is terminal
		{from: "R", write: "W", status: "R", sentOn: true, changed: false},
		{from: "R", write: "S", status: "R", sentOn: true, changed: false},
		{from: "R", write: "D", status: "R", sentOn: true, changed: false},
		{from: "R", write: "E", status: "R", sentOn: true, changed: false},
		{from: "R", write: "F", status: "R", sentOn: true, changed: false},

		// failed is terminal
		{from: "F", write: "W", status: "F", sentOn: false, changed: false},
		{from: "F", write: "S", status: "F", sentOn: false, changed: false},
		{from: "F", write: "D", status: "F", sentOn: false, changed: false},
		{from: "F", write: "R", status: "F", sentOn: false, changed: false},
		{from: "F", write: "E", status: "F", sentOn: false, changed: false},

		// errored is retried so moves on to anything, and fails once the attempt limit is reached
		{from: "E", fromErrors: 1, write: "W", status: "W", errors: 1, sentOn: true, changed: true},
		{from: "E", fromErrors: 1, write: "S", status: "S", errors: 1, sentOn: true, changed: true},
		{from: "E", fromErrors: 1, write: "D", status: "D", errors: 1, sentOn: true, changed: true},
		{from: "E", fromErrors: 1, write: "F", status: "F", errors: 1, sentOn: false, changed: true},
		{from: "E", fromErrors: 1, write: "E", status: "E", errors: 2, sentOn: false, changed: false},
		{from: "E", fromErrors: 2, write: "E", status: "F", errors: 3, failedReason: "E", sentOn: false, changed: true},
		{from: "E", fromErrors: 2, write: "W", status: "W", errors: 2, sentOn: true, changed: true},
	}

	for _, tc := range tcs {
		desc := fmt.Sprintf("%s -> %s", tc.from, tc.write)

		// an errored message is awaiting a retry
		rt.DB.MustExec(`UPDATE msgs_msg SET status = $1::varchar, error_count = $2, failed_reason = NULL, log_uuids = '{}',
			next_attempt = CASE WHEN $1::varchar = 'E' THEN NOW() ELSE NULL END,
			sent_on = CASE WHEN $1::varchar IN ('W', 'S', 'D', 'R') THEN NOW() ELSE NULL END WHERE uuid = $3`, tc.from, tc.fromErrors, msgUUID)

		changes, err := models.WriteStatusUpdates(ctx, rt, []*models.StatusUpdate{
			{
				ChannelUUID_: "dbc126ed-66bc-4e28-b67b-81dc3327c95d",
				ChannelID_:   10,
				MsgUUID_:     msgUUID,
				Status_:      tc.write,
				LogUUID:      "019a6e53-e1e8-7df7-a264-ce2372824e1d",
			},
		})
		assert.NoError(t, err, desc)

		if tc.changed {
			if assert.Len(t, changes, 1, desc) {
				assert.Equal(t, tc.status, changes[0].MsgStatus, desc)
				assert.Equal(t, tc.failedReason, string(changes[0].FailedReason), desc)
			}
		} else {
			assert.Len(t, changes, 0, desc)
		}

		// a message is awaiting a retry exactly whilst it's errored - an errored attempt that was recorded schedules one,
		// a rejected one doesn't, and moving on to any other status (including failing on the last attempt) clears it
		retryScheduled := tc.status == "E"

		assertdb.Query(t, rt.DB, `SELECT status, error_count, COALESCE(failed_reason, '') AS failed_reason, sent_on IS NOT NULL AS sent_on, 
			next_attempt IS NOT NULL AS retry, array_length(log_uuids, 1) AS logs FROM msgs_msg WHERE uuid = $1`, msgUUID).
			Columns(map[string]any{
				"status":        string(tc.status),
				"error_count":   int64(tc.errors),
				"failed_reason": tc.failedReason,
				"sent_on":       tc.sentOn,
				"retry":         retryScheduled,
				"logs":          int64(1), // log is recorded on the message even if it didn't change its status
			}, desc)
	}
}

func TestWriteStatusUpdatesSameMsg(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)

	defer testsuite.ResetDB(t, rt)

	const msgUUID = models.MsgUUID("0199df0f-9f82-7689-b02d-f34105991321") // message 1

	rt.DB.MustExec(`UPDATE msgs_msg SET status = 'Q', sent_on = NULL, log_uuids = '{}' WHERE uuid = $1`, msgUUID)

	// a failed callback from the provider overtakes the sender's wired write and both land in the same batch
	changes, err := models.WriteStatusUpdates(ctx, rt, []*models.StatusUpdate{
		{
			ChannelUUID_: "dbc126ed-66bc-4e28-b67b-81dc3327c95d",
			ChannelID_:   10,
			MsgUUID_:     msgUUID,
			Status_:      models.MsgStatusFailed,
			LogUUID:      "019a6e53-e1e8-7df7-a264-ce2372824e1d",
		},
		{
			ChannelUUID_: "dbc126ed-66bc-4e28-b67b-81dc3327c95d",
			ChannelID_:   10,
			MsgUUID_:     msgUUID,
			Status_:      models.MsgStatusWired,
			LogUUID:      "019a6e54-671f-789a-bbb1-31cddd66c681",
		},
	})
	assert.NoError(t, err)
	if assert.Len(t, changes, 1) {
		assert.Equal(t, models.MsgStatusFailed, changes[0].MsgStatus)
	}

	// message is failed and both updates are recorded on it
	assertdb.Query(t, rt.DB, `SELECT status, sent_on IS NOT NULL AS sent_on, array_to_string(log_uuids, ',') AS logs FROM msgs_msg WHERE uuid = $1`, msgUUID).
		Columns(map[string]any{
			"status":  "F",
			"sent_on": false,
			"logs":    "019a6e53-e1e8-7df7-a264-ce2372824e1d,019a6e54-671f-789a-bbb1-31cddd66c681",
		})

	rt.DB.MustExec(`UPDATE msgs_msg SET status = 'Q', sent_on = NULL, log_uuids = '{}' WHERE uuid = $1`, msgUUID)

	// sent and delivered callbacks landing in the same batch are each applied in turn
	changes, err = models.WriteStatusUpdates(ctx, rt, []*models.StatusUpdate{
		{
			ChannelUUID_: "dbc126ed-66bc-4e28-b67b-81dc3327c95d",
			ChannelID_:   10,
			MsgUUID_:     msgUUID,
			Status_:      models.MsgStatusSent,
			LogUUID:      "019a6e53-e1e8-7df7-a264-ce2372824e1d",
		},
		{
			ChannelUUID_: "dbc126ed-66bc-4e28-b67b-81dc3327c95d",
			ChannelID_:   10,
			MsgUUID_:     msgUUID,
			Status_:      models.MsgStatusDelivered,
			LogUUID:      "019a6e54-671f-789a-bbb1-31cddd66c681",
		},
	})
	assert.NoError(t, err)
	if assert.Len(t, changes, 2) {
		assert.Equal(t, models.MsgStatusSent, changes[0].MsgStatus)
		assert.Equal(t, models.MsgStatusDelivered, changes[1].MsgStatus)
	}

	assertdb.Query(t, rt.DB, `SELECT status, sent_on IS NOT NULL AS sent_on, array_to_string(log_uuids, ',') AS logs FROM msgs_msg WHERE uuid = $1`, msgUUID).
		Columns(map[string]any{
			"status":  "D",
			"sent_on": true,
			"logs":    "019a6e53-e1e8-7df7-a264-ce2372824e1d,019a6e54-671f-789a-bbb1-31cddd66c681",
		})
}
