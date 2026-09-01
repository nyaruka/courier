package models_test

import (
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
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
