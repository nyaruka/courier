package channels_test

import (
	"testing"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllowRate(t *testing.T) {
	_, rt := testsuite.Runtime(t)
	testsuite.ResetValkey(t, rt)

	// a key is allowed up to the limit within the window...
	for i := range 3 {
		assert.True(t, channels.AllowRate(rt, "test-limit:a", 3, 60), "request %d", i)
	}

	// ...and then refused
	assert.False(t, channels.AllowRate(rt, "test-limit:a", 3, 60))

	// without affecting other keys
	assert.True(t, channels.AllowRate(rt, "test-limit:b", 3, 60))

	// and the count expires with the window
	rc := rt.VK.Get()
	defer rc.Close()
	ttl, err := redis.Int(rc.Do("TTL", "test-limit:a"))
	require.NoError(t, err)
	assert.Greater(t, ttl, 0)
	assert.LessOrEqual(t, ttl, 60)
}
