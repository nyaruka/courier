package models_test

import (
	"testing"

	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/courier/testsuite"
	"github.com/nyaruka/gocommon/dbutil/assertdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContactURNs(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)

	defer testsuite.ResetDB(t, rt)

	urn := models.NewContactURN(1, 11, 100, "tel:+1234567890", nil)

	tx := rt.DB.MustBegin()

	err := models.InsertContactURN(ctx, tx, urn)
	assert.NoError(t, err)
	require.NoError(t, tx.Commit())

	assertdb.Query(t, rt.DB, "SELECT count(*) FROM contacts_contacturn WHERE org_id = 1 AND contact_id = 100").Returns(2)
	assertdb.Query(t, rt.DB, "SELECT identity FROM contacts_contacturn WHERE org_id = 1 AND channel_id = 11 AND contact_id = 100").Returns("tel:+1234567890")

	tx = rt.DB.MustBegin()

	curns, err := models.GetURNsForContact(ctx, tx, 100)
	assert.NoError(t, err)
	assert.Len(t, curns, 2)
	assert.Equal(t, "tel:+12067799192", curns[0].Identity)
	assert.Equal(t, "tel:+1234567890", curns[1].Identity)

	require.NoError(t, tx.Commit())
}
