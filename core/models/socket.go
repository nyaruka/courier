package models

import (
	"context"
	"fmt"

	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/gocommon/centrifugo"
)

// SocketHistoryNamespace is the realtime pub/sub namespace for a contact's message history. A history socket is
// addressed as "history:<contact-uuid>". Courier publishes msg status changes to these sockets for any live
// subscribers. ("Socket" is our name for a realtime pub/sub address - so as not to overload Channel, which already
// means a messaging channel.)
const SocketHistoryNamespace = "history"

// HistorySocket returns the realtime pub/sub socket for a contact's message history ("history:<contact-uuid>").
func HistorySocket(contactUUID ContactUUID) string {
	return fmt.Sprintf("%s:%s", SocketHistoryNamespace, contactUUID)
}

// PublishStatusChanges publishes the given status changes to their contacts' history sockets, each as a
// msg_status_changed event - the same shape mailroom publishes for engine events. It's best-effort and a no-op for
// any socket that currently has no subscribers - the centrifugo service resolves subscriber presence for the whole
// batch in a single lookup and sends the surviving events as one pipelined request, so the batch lands or fails
// together.
func PublishStatusChanges(ctx context.Context, rt *runtime.Runtime, changes []*StatusChange) error {
	pubs := make([]*centrifugo.Publication, len(changes))
	for i, c := range changes {
		pubs[i] = &centrifugo.Publication{Channel: HistorySocket(c.ContactUUID), Data: c.historyEvent()}
	}

	if err := rt.Centrifugo.Publish(ctx, pubs...); err != nil {
		return fmt.Errorf("error publishing status change events: %w", err)
	}

	return nil
}
