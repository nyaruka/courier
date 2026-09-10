package channels

import (
	"log/slog"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier/v26/runtime"
)

// AllowRate counts a request against the given valkey key and returns whether it's within the given limit for a
// window of the given number of seconds. The key's TTL is re-armed on every request rather than only the first:
// INCR + EXPIRE isn't atomic, and a key left behind by a lost EXPIRE would otherwise count forever and permanently
// block whatever the key identifies. The result is a sliding window - continuous callers stay throttled, which is
// fine for an abuse cap. It fails open: a valkey problem shouldn't block real traffic, so errors are logged and the
// request allowed.
func AllowRate(rt *runtime.Runtime, key string, limit, window int) bool {
	rc := rt.VK.Get()
	defer rc.Close()

	count, err := redis.Int(rc.Do("INCR", key))
	if err != nil {
		slog.Error("error checking rate limit", "error", err, "key", key)
		return true
	}
	if _, err := rc.Do("EXPIRE", key, window); err != nil {
		slog.Error("error setting rate limit expiry", "error", err, "key", key)
	}

	return count <= limit
}
