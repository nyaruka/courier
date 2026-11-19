package models_test

import (
	"fmt"
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/gocommon/queues"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetChannelCachesAbsence(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)

	ch := test.NewMockChannel("d84e0f1e-6b1e-4f4a-9e6e-9a48c4b8b5a1", "NX", "2020", "US", []string{urns.Phone.Prefix}, nil)
	testsuite.InsertChannel(t, rt, ch) // flushes the cache

	got, err := models.GetChannel(ctx, "NX", ch.UUID())
	assert.NoError(t, err)
	assert.Equal(t, ch.UUID(), got.UUID())

	// deactivating puts it outside what the load query selects, so it's gone as far as we're concerned
	rt.DB.MustExec(`UPDATE channels_channel SET is_active = FALSE WHERE uuid = $1`, ch.UUID())
	models.FlushChannelCache()

	_, err = models.GetChannel(ctx, "NX", ch.UUID())
	assert.ErrorIs(t, err, models.ErrChannelNotFound)

	// that absence is cached, so bringing it back doesn't take effect until the entry expires or is flushed -
	// which is what saves a provider still calling a deleted channel's URL a query per request
	rt.DB.MustExec(`UPDATE channels_channel SET is_active = TRUE WHERE uuid = $1`, ch.UUID())

	_, err = models.GetChannel(ctx, "NX", ch.UUID())
	assert.ErrorIs(t, err, models.ErrChannelNotFound, "absence should have been served from the cache")

	models.FlushChannelCache()

	got, err = models.GetChannel(ctx, "NX", ch.UUID())
	assert.NoError(t, err)
	assert.Equal(t, ch.UUID(), got.UUID())

	// a channel that's there but of another type is still a type error rather than an absence
	_, err = models.GetChannel(ctx, "TG", ch.UUID())
	assert.ErrorIs(t, err, models.ErrChannelWrongType)
}

func TestGetChannelByAddressDoesntCacheAbsence(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)

	ch := test.NewMockChannel("6b1e0f1e-d84e-4f4a-9e6e-9a48c4b8b5a2", "NX", "2021", "US", []string{urns.Phone.Prefix}, nil)
	testsuite.InsertChannel(t, rt, ch)

	got, err := models.GetChannelByAddress(ctx, "NX", models.ChannelAddress("2021"))
	assert.NoError(t, err)
	assert.Equal(t, ch.UUID(), got.UUID())

	rt.DB.MustExec(`UPDATE channels_channel SET is_active = FALSE WHERE uuid = $1`, ch.UUID())
	models.FlushChannelCache()

	_, err = models.GetChannelByAddress(ctx, "NX", models.ChannelAddress("2021"))
	assert.ErrorIs(t, err, models.ErrChannelNotFound)

	// an address is reused and is asked about by shared webhooks that carry it in the body, so absence at one
	// isn't cached - a channel provisioned at an address we've already been asked about has to be visible at
	// once rather than when an entry expires
	rt.DB.MustExec(`UPDATE channels_channel SET is_active = TRUE WHERE uuid = $1`, ch.UUID())

	got, err = models.GetChannelByAddress(ctx, "NX", models.ChannelAddress("2021"))
	assert.NoError(t, err, "absence should not have been cached")
	assert.Equal(t, ch.UUID(), got.UUID())
}

func TestChannelsOfSuspendedOrgsNotFound(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)

	defer testsuite.ResetDB(t, rt)

	vc := rt.VK.Get()
	defer vc.Close()

	rt.DB.MustExec(`INSERT INTO orgs_org(id, name, language, is_anon, config, limits, status)
		VALUES(2, 'Suspended', 'eng', FALSE, '{}', '{}', 'S')`)

	ch := test.NewMockChannel("a3f1c6e2-5b7d-4c8e-9f0a-1b2c3d4e5f60", "NX", "2022", "US", []string{urns.Phone.Prefix}, nil)
	ch.OrgID_ = 2
	testsuite.InsertChannel(t, rt, ch)

	_, err := models.GetChannel(ctx, "NX", ch.UUID())
	assert.ErrorIs(t, err, models.ErrChannelNotFound)

	_, err = models.GetChannelByAddress(ctx, "NX", models.ChannelAddress("2022"))
	assert.ErrorIs(t, err, models.ErrChannelNotFound)

	// a message already queued for it is dropped when popped, and popping carries on to the next one
	pushMsg := func(channelUUID models.ChannelUUID, msgUUID string) {
		msgJSON := fmt.Sprintf(`[{
			"org_id": 1, "id": 10000, "uuid": %q, "text": "hi", "urn": "tel:+12067799192", "origin": "chat",
			"contact": {"id": 100, "uuid": "a984069d-0008-4d8c-a772-b14a8a6acccc"},
			"created_on": "2025-10-14T20:16:03.821434Z", "channel_uuid": %q
		}]`, msgUUID, channelUUID)
		require.NoError(t, models.MsgQueue.Push(ctx, vc, queues.OwnerID(channelUUID), 10, true, []byte(msgJSON)))
	}
	pushMsg(ch.UUID(), "019a9d4e-1c2b-7a3d-8e4f-5a6b7c8d9e01")
	pushMsg("dbc126ed-66bc-4e28-b67b-81dc3327c95d", "019a9d4e-1c2b-7a3d-8e4f-5a6b7c8d9e02")

	msg, err := models.PopNextOutgoingMsg(ctx, rt)
	require.NoError(t, err)
	require.NotNil(t, msg)
	assert.Equal(t, models.MsgUUID("019a9d4e-1c2b-7a3d-8e4f-5a6b7c8d9e02"), msg.UUID())

	msg, err = models.PopNextOutgoingMsg(ctx, rt)
	assert.NoError(t, err)
	assert.Nil(t, msg)

	// unsuspending brings it back, for a lookup by UUID once the cached absence goes
	rt.DB.MustExec(`UPDATE orgs_org SET status = 'A' WHERE id = 2`)
	models.FlushChannelCache()

	got, err := models.GetChannel(ctx, "NX", ch.UUID())
	assert.NoError(t, err)
	assert.Equal(t, ch.UUID(), got.UUID())
}

func TestChannelOrgContactLimit(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)

	defer testsuite.ResetDB(t, rt)

	// the test org has no limits set
	ch, err := models.GetChannel(ctx, "KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	require.NoError(t, err)
	assert.Equal(t, models.NoLimit, ch.OrgContactLimit())

	// other limit types don't count
	rt.DB.MustExec(`UPDATE orgs_org SET limits = '{"fields": 250}' WHERE id = 1`)
	models.FlushChannelCache()

	ch, err = models.GetChannel(ctx, "KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	require.NoError(t, err)
	assert.Equal(t, models.NoLimit, ch.OrgContactLimit())

	rt.DB.MustExec(`UPDATE orgs_org SET limits = '{"fields": 250, "contacts": 1000}' WHERE id = 1`)
	models.FlushChannelCache()

	ch, err = models.GetChannel(ctx, "KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	require.NoError(t, err)
	assert.Equal(t, 1000, ch.OrgContactLimit())
}
