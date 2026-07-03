package models_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/gocommon/centrifugo"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistorySocket(t *testing.T) {
	contact := models.ContactUUID("a393abc0-283d-4c9b-a1b3-641a035c34bf")

	assert.Equal(t, "history:a393abc0-283d-4c9b-a1b3-641a035c34bf", models.HistorySocket(contact))
}

func TestPublishStatusChanges(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)

	defer testsuite.ResetValkey(t, rt)

	vc := rt.VK.Get()
	defer vc.Close()

	contact1Socket := "history:a984069d-0008-4d8c-a772-b14a8a6acccc"
	contact2Socket := "history:60d5c8bf-16c8-4dcb-9cda-11a19e3d1c95"

	changes := []*models.StatusChange{
		{
			ContactUUID: "a984069d-0008-4d8c-a772-b14a8a6acccc",
			MsgUUID:     "0199df0f-9f82-7689-b02d-f34105991321",
			MsgStatus:   models.MsgStatusDelivered,
			OrgID:       1,
			CreatedOn:   time.Date(2025, 11, 10, 16, 14, 30, 123456789, time.UTC),
		},
		{
			ContactUUID:  "60d5c8bf-16c8-4dcb-9cda-11a19e3d1c95",
			MsgUUID:      "0199df10-10dc-7e6e-834b-3d959ece93b2",
			MsgStatus:    models.MsgStatusFailed,
			FailedReason: "E",
			OrgID:        1,
			CreatedOn:    time.Date(2025, 11, 10, 16, 15, 0, 0, time.UTC),
		},
	}

	// no changes is a no-op
	require.NoError(t, models.PublishStatusChanges(ctx, rt, nil))

	// no sockets subscribed yet, so nothing is published
	require.NoError(t, models.PublishStatusChanges(ctx, rt, changes))
	assert.Empty(t, testsuite.CentrifugoHistory(t, rt, contact1Socket))
	assert.Empty(t, testsuite.CentrifugoHistory(t, rt, contact2Socket))

	// mark contact 1's socket subscribed (as the authorizing service would)
	_, err := vc.Do("SET", centrifugo.SubscriptionKey(contact1Socket), "1")
	require.NoError(t, err)

	// now only contact 1's change is published
	require.NoError(t, models.PublishStatusChanges(ctx, rt, changes))

	sent := testsuite.CentrifugoHistory(t, rt, contact1Socket)
	require.Len(t, sent, 1)
	assert.Empty(t, testsuite.CentrifugoHistory(t, rt, contact2Socket))

	// event matches the JSON of goflow's msg_status_changed event
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(sent[0], &decoded))
	assert.True(t, uuids.Is(decoded["uuid"].(string)))
	assert.Equal(t, "msg_status_changed", decoded["type"])
	assert.Equal(t, "2025-11-10T16:14:30.123456789Z", decoded["created_on"])
	assert.Equal(t, "0199df0f-9f82-7689-b02d-f34105991321", decoded["msg_uuid"])
	assert.Equal(t, "delivered", decoded["status"])
	assert.NotContains(t, decoded, "reason") // omitted when there isn't one

	// mark contact 2's socket subscribed as well and both changes are published
	_, err = vc.Do("SET", centrifugo.SubscriptionKey(contact2Socket), "1")
	require.NoError(t, err)

	require.NoError(t, models.PublishStatusChanges(ctx, rt, changes))

	assert.Len(t, testsuite.CentrifugoHistory(t, rt, contact1Socket), 2)

	sent = testsuite.CentrifugoHistory(t, rt, contact2Socket)
	require.Len(t, sent, 1)
	require.NoError(t, json.Unmarshal(sent[0], &decoded))
	assert.Equal(t, "msg_status_changed", decoded["type"])
	assert.Equal(t, "0199df10-10dc-7e6e-834b-3d959ece93b2", decoded["msg_uuid"])
	assert.Equal(t, "failed", decoded["status"])
	assert.Equal(t, "error_limit", decoded["reason"])
}
