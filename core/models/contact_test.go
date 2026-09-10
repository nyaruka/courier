package models_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/gocommon/dbutil/assertdb"
	"github.com/nyaruka/gocommon/svclogs"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/null/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInsertContact(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)

	defer testsuite.ResetDB(t, rt)

	contact := &models.Contact{
		OrgID_:      1,
		Name_:       null.String("Test"),
		URNID_:      models.NilContactURNID,
		CreatedBy_:  1,
		ModifiedBy_: 1,
		IsNew_:      true,
	}

	tx := rt.DB.MustBegin()

	err := models.InsertContact(ctx, tx, contact)
	assert.NoError(t, err)
	assert.NoError(t, tx.Commit())

	assertdb.Query(t, rt.DB, "SELECT count(*) FROM contacts_contact WHERE org_id = 1 AND name = 'Test'").Returns(1)
}

func TestContactLimit(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)

	defer testsuite.ResetDB(t, rt)

	// counts are maintained by triggers in the real database, so here we insert them ourselves as contacts are
	// created - org 1 starts with one contact (id 100 with tel:+12067799192)
	addCount := func(groupID, delta int) {
		rt.DB.MustExec(`INSERT INTO contacts_contactgroupcount(group_id, count, is_squashed) VALUES($1, $2, FALSE)`, groupID, delta)
	}
	addCount(1, 1)

	setLimits := func(limits string) *models.Channel {
		rt.DB.MustExec(`UPDATE orgs_org SET limits = $1 WHERE id = 1`, limits)
		models.FlushChannelCache()

		ch, err := models.GetChannel(ctx, "KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
		require.NoError(t, err)
		return ch
	}
	getContact := func(ch *models.Channel, urn urns.URN) (*models.Contact, *models.ChannelLog, error) {
		clog := models.NewChannelLog(models.ChannelLogTypeReceive, ch, nil, nil)
		contact, err := models.GetContact(ctx, rt, ch, urn, nil, "", true, clog)
		return contact, clog, err
	}
	assertCreated := func(contact *models.Contact, clog *models.ChannelLog, err error) {
		require.NoError(t, err)
		assert.True(t, contact.IsNew_)
		assert.Len(t, clog.Errors, 0)
		addCount(1, 1)
	}
	assertLimitReached := func(clog *models.ChannelLog, err error, limit int) {
		var limitErr *models.LimitReachedError
		require.ErrorAs(t, err, &limitErr)
		assert.Equal(t, "contacts", limitErr.Limit)
		assert.Equal(t, limit, limitErr.Max)
		assert.EqualError(t, err, fmt.Sprintf("workspace has reached its limit of %d contacts", limit))

		// and the refusal is recorded on the channel log so it's visible to the workspace
		require.Len(t, clog.Errors, 1)
		assert.Equal(t, "contact_limit_reached", clog.Errors[0].Code)
	}

	ch := setLimits(`{"contacts": 2}`)

	// a contact that already exists is always returned, and no error is logged
	contact, clog, err := getContact(ch, "tel:+12067799192")
	require.NoError(t, err)
	assert.Equal(t, models.ContactID(100), contact.ID_)
	assert.Len(t, clog.Errors, 0)

	// there's room for one more contact...
	contact, clog, err = getContact(ch, "tel:+12065551111")
	assertCreated(contact, clog, err)

	// ...but not another, and the cached count knows about the one just created without being reloaded
	contact, clog, err = getContact(ch, "tel:+12065552222")
	assertLimitReached(clog, err, 2)
	assert.Nil(t, contact)
	assertdb.Query(t, rt.DB, `SELECT count(*) FROM contacts_contact WHERE org_id = 1`).Returns(2)
	assertdb.Query(t, rt.DB, `SELECT count(*) FROM contacts_contacturn WHERE identity = 'tel:+12065552222'`).Returns(0)

	// existing contacts are still returned at the limit
	contact, clog, err = getContact(ch, "tel:+12065551111")
	require.NoError(t, err)
	assert.False(t, contact.IsNew_)
	assert.Len(t, clog.Errors, 0)

	// the count is cached so a change to the counts in the database isn't seen until the cache is flushed
	ch = setLimits(`{"contacts": 3}`)
	addCount(1, 5)
	contact, clog, err = getContact(ch, "tel:+12065552222")
	assertCreated(contact, clog, err)
	assertdb.Query(t, rt.DB, `SELECT count(*) FROM contacts_contact WHERE org_id = 1`).Returns(3)

	addCount(1, -5)
	models.FlushContactCounts()
	_, clog, err = getContact(ch, "tel:+12065553333")
	assertLimitReached(clog, err, 3)

	// an org without its own limit gets the configured default...
	rt.Config.DefaultContactLimit = 3
	ch = setLimits(`{"fields": 250}`)
	_, clog, err = getContact(ch, "tel:+12065553333")
	assertLimitReached(clog, err, 3)

	// ...unless that's zero, meaning no limit
	rt.Config.DefaultContactLimit = 0
	ch = setLimits(`{}`)
	contact, clog, err = getContact(ch, "tel:+12065553333")
	assertCreated(contact, clog, err)

	// and an org's own limit applies even when the default is no limit
	ch = setLimits(`{"contacts": 2}`)
	models.FlushContactCounts()
	_, clog, err = getContact(ch, "tel:+12065554444")
	assertLimitReached(clog, err, 2)

	// the count covers every status, not just active contacts - there are 4 contacts so if only active contacts
	// were counted, blocking one would make room under a limit of 4
	rt.DB.MustExec(`UPDATE contacts_contact SET status = 'B' WHERE id = 100`)
	addCount(1, -1)
	addCount(2, 1)
	ch = setLimits(`{"contacts": 4}`)
	models.FlushContactCounts()
	_, clog, err = getContact(ch, "tel:+12065554444")
	assertLimitReached(clog, err, 4)
}

func TestContactCreationCheck(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)

	defer testsuite.ResetDB(t, rt)

	ch, err := models.GetChannel(ctx, "KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	require.NoError(t, err)

	// a deployment can replace the check with its own policy, which is consulted only when a contact would be created
	var checked []urns.URN
	defer func() { models.ContactCreationCheck = models.CheckContactLimit }()
	models.ContactCreationCheck = func(ctx context.Context, rt *runtime.Runtime, channel *models.Channel, urn urns.URN, clog *models.ChannelLog) error {
		checked = append(checked, urn)
		if urn.Path() == "+12065552222" {
			clog.Error(&svclogs.Error{Code: "test_refused", Message: "Refused by test."})
			return &models.LimitReachedError{Limit: "test contacts", Max: 1}
		}
		return nil
	}

	getContact := func(urn urns.URN) (*models.Contact, *models.ChannelLog, error) {
		clog := models.NewChannelLog(models.ChannelLogTypeReceive, ch, nil, nil)
		contact, err := models.GetContact(ctx, rt, ch, urn, nil, "", true, clog)
		return contact, clog, err
	}

	// an existing contact is returned without the check being consulted
	contact, clog, err := getContact("tel:+12067799192")
	require.NoError(t, err)
	assert.Equal(t, models.ContactID(100), contact.ID_)
	assert.Len(t, clog.Errors, 0)
	assert.Empty(t, checked)

	// a new contact the check allows is created
	contact, clog, err = getContact("tel:+12065551111")
	require.NoError(t, err)
	assert.True(t, contact.IsNew_)
	assert.Len(t, clog.Errors, 0)
	assert.Equal(t, []urns.URN{"tel:+12065551111"}, checked)

	// and one it refuses isn't, with the refusal reported as a limit being reached
	contact, clog, err = getContact("tel:+12065552222")
	var limitErr *models.LimitReachedError
	require.True(t, errors.As(err, &limitErr))
	assert.EqualError(t, err, "workspace has reached its limit of 1 test contacts")
	assert.Nil(t, contact)
	require.Len(t, clog.Errors, 1)
	assert.Equal(t, "test_refused", clog.Errors[0].Code)
	assertdb.Query(t, rt.DB, `SELECT count(*) FROM contacts_contacturn WHERE identity = 'tel:+12065552222'`).Returns(0)
}
