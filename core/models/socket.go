package models

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gomodule/redigo/redis"
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

// subscriptionKey is the valkey key marking that a realtime socket has at least one active subscriber, e.g.
// "socket-subs:history:<contact-uuid>". The key is a per-socket presence marker written by the service that
// authorizes subscriptions (it sets/re-arms the key with a TTL on every subscribe and refresh); courier only
// reads it.
func subscriptionKey(socket string) string {
	return fmt.Sprintf("socket-subs:%s", socket)
}

// SubscribedSockets returns the subset of the given sockets that currently have at least one active subscriber. It
// resolves them all in a single round-trip by MGETting their presence keys, so checking many sockets at once costs
// one lookup rather than one per socket. A socket is subscribed when its key is present; missing keys come back nil.
// The returned map only contains the subscribed sockets.
func SubscribedSockets(ctx context.Context, rt *runtime.Runtime, sockets ...string) (map[string]bool, error) {
	if len(sockets) == 0 {
		return nil, nil
	}

	keys := make([]any, len(sockets))
	for i, s := range sockets {
		keys[i] = subscriptionKey(s)
	}

	vc := rt.VK.Get()
	defer vc.Close()

	values, err := redis.Values(redis.DoContext(vc, ctx, "MGET", keys...))
	if err != nil {
		return nil, fmt.Errorf("error checking socket subscriptions: %w", err)
	}

	subscribed := make(map[string]bool, len(sockets))
	for i, v := range values {
		if v != nil {
			subscribed[sockets[i]] = true
		}
	}
	return subscribed, nil
}

// PublishStatusChanges publishes the given status changes to their contacts' history sockets, each as a
// msg_status_changed event - the same shape mailroom publishes for engine events. It's best-effort and a no-op for
// any socket that currently has no subscribers; the sockets a batch touches are resolved in a single presence
// lookup, then every subscribed socket's events are batched into one pipelined request so the whole batch costs one
// centrifugo round-trip and lands or fails together.
func PublishStatusChanges(ctx context.Context, rt *runtime.Runtime, changes []*StatusChange) error {
	if len(changes) == 0 {
		return nil
	}

	sockets := make([]string, 0, len(changes))
	seen := make(map[string]bool, len(changes))
	for _, c := range changes {
		if socket := HistorySocket(c.ContactUUID); !seen[socket] {
			seen[socket] = true
			sockets = append(sockets, socket)
		}
	}

	subscribed, err := SubscribedSockets(ctx, rt, sockets...)
	if err != nil {
		return err
	}

	var pubs []*centrifugo.Publish
	for _, c := range changes {
		socket := HistorySocket(c.ContactUUID)
		if !subscribed[socket] {
			continue
		}
		data, err := json.Marshal(c.historyEvent())
		if err != nil {
			return fmt.Errorf("error marshaling status change event for %s: %w", socket, err)
		}
		pubs = append(pubs, &centrifugo.Publish{Channel: socket, Data: data})
	}

	if err := rt.Centrifugo.Publish(ctx, pubs...); err != nil {
		return fmt.Errorf("error publishing status change events: %w", err)
	}

	return nil
}
