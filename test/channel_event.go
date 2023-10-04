package test

import (
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/gocommon/urns"
)

type mockChannelEvent struct {
	channel    courier.Channel
	eventType  courier.ChannelEventType
	urn        urns.URN
	createdOn  time.Time
	occurredOn time.Time

	contactName   string
	urnAuthTokens map[string]string
	extra         map[string]string
}

func (e *mockChannelEvent) EventID() int64                      { return 0 }
func (e *mockChannelEvent) ChannelUUID() courier.ChannelUUID    { return e.channel.UUID() }
func (e *mockChannelEvent) EventType() courier.ChannelEventType { return e.eventType }
func (e *mockChannelEvent) CreatedOn() time.Time                { return e.createdOn }
func (e *mockChannelEvent) OccurredOn() time.Time               { return e.occurredOn }
func (e *mockChannelEvent) Extra() map[string]string            { return e.extra }
func (e *mockChannelEvent) ContactName() string                 { return e.contactName }
func (e *mockChannelEvent) URN() urns.URN                       { return e.urn }

func (e *mockChannelEvent) WithExtra(extra map[string]string) courier.ChannelEvent {
	e.extra = extra
	return e
}

func (e *mockChannelEvent) WithContactName(name string) courier.ChannelEvent {
	e.contactName = name
	return e
}

func (e *mockChannelEvent) WithURNAuthTokens(tokens map[string]string) courier.ChannelEvent {
	e.urnAuthTokens = tokens
	return e
}

func (e *mockChannelEvent) WithOccurredOn(time time.Time) courier.ChannelEvent {
	e.occurredOn = time
	return e
}
