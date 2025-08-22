package test

import (
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/gocommon/urns"
)

type mockContact struct {
	channel    courier.Channel
	urn        urns.URN
	authTokens map[string]string
	uuid       models.ContactUUID
}

func (c *mockContact) UUID() models.ContactUUID { return c.uuid }
