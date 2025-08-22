package models_test

import (
	"testing"

	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/courier/testsuite"
	"github.com/nyaruka/gocommon/dbutil/assertdb"
	"github.com/nyaruka/null/v3"
	"github.com/stretchr/testify/assert"
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
