package models_test

import (
	"cmp"
	"sort"
	"testing"

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
		sort.Slice(changes, func(i, j int) bool { return cmp.Compare(changes[0].MsgUUID, changes[1].MsgUUID) > 0 })

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
