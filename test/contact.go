package test

import (
	"github.com/nyaruka/courier"
	"github.com/nyaruka/gocommon/urns"
)

type mockContact struct {
	channel    courier.Channel
	urn        urns.URN
	authTokens map[string]string
	uuid       courier.ContactUUID
}

func (c *mockContact) UUID() courier.ContactUUID { return c.uuid }
