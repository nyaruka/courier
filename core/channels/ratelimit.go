package channels

import (
	"log/slog"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier/v26/runtime"
)

// AllowRate counts a request against the given valkey key and returns whether it's within the given limit for a
// fixed window of the given number of seconds, starting at the key's first request. The TTL is only set when the key
// has none, so steady traffic can't keep extending the window, and a key left without one by a lost EXPIRE gets it on
// its next request. It fails open: a valkey problem shouldn't block real traffic, so errors are logged and the request
// allowed.
func AllowRate(rt *runtime.Runtime, key string, limit, window int) bool {
	rc := rt.VK.Get()
	defer rc.Close()

	count, err := redis.Int(rc.Do("INCR", key))
	if err != nil {
		slog.Error("error checking rate limit", "error", err, "key", key)
		return true
	}
	if _, err := rc.Do("EXPIRE", key, window, "NX"); err != nil {
		slog.Error("error setting rate limit expiry", "error", err, "key", key)
	}

	return count <= limit
}
