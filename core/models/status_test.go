package models_test

import (
	"cmp"
	"sort"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/courier/testsuite"
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
			ChannelUUID_: "dbc126ed-66bc-4e28-b67b-81dc3327c95d",
			ChannelID_:   10,
			MsgUUID_:     "0199df10-10dc-7e6e-834b-3d959ece93b2", // message 2
			Status_:      models.MsgStatusErrored,
			LogUUID:      "019a6e54-671f-789a-bbb1-31cddd66c681",
		},
		{
			ChannelUUID_: "dbc126ed-66bc-4e28-b67b-81dc3327c95d",
			ChannelID_:   10,
			MsgUUID_:     "019a6e61-a4ce-7e60-86d0-aca6405ddb90", // no such message
			Status_:      models.MsgStatusSent,
			LogUUID:      "019a6e62-81b9-79e5-b654-56e6094692a6",
		},
	}

	changes, err := models.WriteStatusUpdates(ctx, rt, updates)
	assert.NoError(t, err)
	if assert.Len(t, changes, 2) {
		sort.Slice(changes, func(i, j int) bool { return cmp.Compare(changes[i].MsgUUID, changes[j].MsgUUID) < 0 })

		assert.Equal(t, models.MsgUUID("0199df0f-9f82-7689-b02d-f34105991321"), changes[0].MsgUUID)
		assert.Equal(t, models.MsgStatus("S"), changes[0].MsgStatus)
		assert.Equal(t, "", string(changes[0].FailedReason))
		assert.Equal(t, models.ContactUUID("a984069d-0008-4d8c-a772-b14a8a6acccc"), changes[0].ContactUUID)
		assert.Equal(t, models.MsgUUID("0199df10-10dc-7e6e-834b-3d959ece93b2"), changes[1].MsgUUID)
		assert.Equal(t, models.MsgStatus("E"), changes[1].MsgStatus)
		assert.Equal(t, "", string(changes[1].FailedReason))
		assert.Equal(t, models.ContactUUID("a984069d-0008-4d8c-a772-b14a8a6acccc"), changes[1].ContactUUID)
	}

	assertdb.Query(t, rt.DB, `SELECT uuid, status FROM msgs_msg`).Map(map[string]any{
		"0199df0f-9f82-7689-b02d-f34105991321": "S",
		"0199df10-10dc-7e6e-834b-3d959ece93b2": "E",
		"0199df10-9519-7fe2-a29c-c890d1713673": "P",
	})

	// write another errored status for message 2
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
		assert.Equal(t, models.MsgStatus("E"), changes[0].MsgStatus)
		assert.Equal(t, "", string(changes[0].FailedReason))
	}

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
}

func TestStatusChanges(t *testing.T) {
	change1 := &models.StatusChange{
		ContactUUID: "a984069d-0008-4d8c-a772-b14a8a6acccc",
		MsgUUID:     "0199df10-10dc-7e6e-834b-3d959ece93b2",
		MsgStatus:   models.MsgStatusSent,
		OrgID:       1,
		CreatedOn:   time.Date(2025, 11, 10, 16, 14, 30, 123456789, time.UTC),
	}

	item1, err := change1.MarshalDynamo()
	assert.NoError(t, err)

	marshaled1, err := attributevalue.MarshalMap(item1)
	assert.NoError(t, err)
	assert.Equal(t, map[string]types.AttributeValue{
		"PK":    &types.AttributeValueMemberS{Value: "con#a984069d-0008-4d8c-a772-b14a8a6acccc"},
		"SK":    &types.AttributeValueMemberS{Value: "evt#0199df10-10dc-7e6e-834b-3d959ece93b2#sts"},
		"OrgID": &types.AttributeValueMemberN{Value: "1"},
		"Data": &types.AttributeValueMemberM{
			Value: map[string]types.AttributeValue{
				"created_on": &types.AttributeValueMemberS{Value: "2025-11-10T16:14:30.123456789Z"},
				"status":     &types.AttributeValueMemberS{Value: "sent"},
			},
		},
	}, marshaled1)

	change2 := &models.StatusChange{
		ContactUUID:  "a984069d-0008-4d8c-a772-b14a8a6acccc",
		MsgUUID:      "0199df10-10dc-7e6e-834b-3d959ece93b2",
		MsgStatus:    models.MsgStatusFailed,
		FailedReason: "E",
		OrgID:        1,
		CreatedOn:    time.Date(2025, 11, 10, 16, 14, 30, 123456789, time.UTC),
	}

	item2, err := change2.MarshalDynamo()
	assert.NoError(t, err)

	marshaled2, err := attributevalue.MarshalMap(item2)
	assert.NoError(t, err)

	assert.Equal(t, map[string]types.AttributeValue{
		"PK":    &types.AttributeValueMemberS{Value: "con#a984069d-0008-4d8c-a772-b14a8a6acccc"},
		"SK":    &types.AttributeValueMemberS{Value: "evt#0199df10-10dc-7e6e-834b-3d959ece93b2#sts"},
		"OrgID": &types.AttributeValueMemberN{Value: "1"},
		"Data": &types.AttributeValueMemberM{
			Value: map[string]types.AttributeValue{
				"created_on": &types.AttributeValueMemberS{Value: "2025-11-10T16:14:30.123456789Z"},
				"status":     &types.AttributeValueMemberS{Value: "failed"},
				"reason":     &types.AttributeValueMemberS{Value: "error_limit"},
			},
		},
	}, marshaled2)
}
