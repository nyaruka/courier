package queue

import (
	_ "embed"
	"time"

	"github.com/gomodule/redigo/redis"
)

// UnackedTTL is how long a send can go unacknowledged before it no longer counts against its channel's limits, so that
// lost acknowledgements can't hold a channel back forever
const UnackedTTL = 10 * time.Minute

// the fewest unacknowledged sends for which a channel's max ack lag applies
const ackLagFloor = 100

// the whole seconds over which a channel's acknowledgement rate is measured
const ackRateWindow = 10

func unackedKey(channelUUID string) string {
	return keyBase + ":unacked:" + channelUUID
}

func ackLimitsKey(channelUUID string) string {
	return keyBase + ":acklimits:" + channelUUID
}

func acksKey(channelUUID string) string {
	return keyBase + ":acks:" + channelUUID
}

//go:embed lua/track.lua
var luaTrack string
var scriptTrack = redis.NewScript(2, luaTrack)

// TrackUnacked records a send that the provider accepted but hasn't yet acknowledged handing on, e.g. an aggregator
// that queues messages before submitting them to the network. The channel isn't popped from while it has maxUnacked or
// more such sends, or while at least ackLagFloor of them would take longer than maxAckLag to clear at the rate the
// provider acknowledged sends over the last ackRateWindow seconds - so a slow provider backs messages up in our queue
// rather than its own. A zero limit is no limit.
func TrackUnacked(conn redis.Conn, channelUUID, msgUUID string, maxUnacked int, maxAckLag time.Duration) error {
	_, err := scriptTrack.Do(conn, unackedKey(channelUUID), ackLimitsKey(channelUUID), msgUUID, maxUnacked, maxAckLag.Seconds(), int(UnackedTTL/time.Second))
	return err
}

//go:embed lua/ack.lua
var luaAck string
var scriptAck = redis.NewScript(3, luaAck)

// Acked records that the provider has acknowledged a send, which may or may not have been passed to TrackUnacked, and
// applies the channel's current limits - so that changing them reaches a channel that's waiting on them and so isn't
// sending. Zero limits remove them.
func Acked(conn redis.Conn, channelUUID, msgUUID string, maxUnacked int, maxAckLag time.Duration) error {
	_, err := scriptAck.Do(conn, unackedKey(channelUUID), acksKey(channelUUID), ackLimitsKey(channelUUID), msgUUID, maxUnacked, maxAckLag.Seconds(), ackRateWindow, int(UnackedTTL/time.Second))
	return err
}
