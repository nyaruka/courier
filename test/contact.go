package test

import (
	"github.com/nyaruka/courier/v26"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/gocommon/urns"
)

type mockContact struct {
	channel    courier.Channel
	urn        urns.URN
	authTokens map[string]string
	id         models.ContactID
	uuid       models.ContactUUID
}

func (c *mockContact) UUID() models.ContactUUID { return c.uuid }
