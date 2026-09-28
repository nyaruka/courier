package testsuite

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	valkey "github.com/gomodule/redigo/redis"
	"github.com/stretchr/testify/require"
)

// Each test binary claims a slot which gives it its own valkey database (in the band reserved for courier tests), so
// that concurrent test runs sharing a valkey instance can't clobber each other's data or that of a dev stack. The
// claim is an advisory lock in Postgres held for the binary's lifetime, so it evaporates when the run that owns it
// dies - and the slot's valkey database is flushed on claim, clearing anything a dead run left behind. If every slot
// is taken, claiming waits for one to free up rather than failing.

const (
	slotCount    = 16
	slotVKDBBase = 32 // valkey databases 32-47

	// base of the advisory lock keys which guard slots
	slotLockBase = 0x636f7572_00000000

	// DSN format for a slot's valkey database
	vkTestDSNFormat = "valkey://valkey:6379/%d"
)

// this binary's slot - claimed on first use
var binSlot = sync.OnceValues(func() (int, error) {
	ctx := context.Background()

	// advisory locks are session scoped so pin a connection for the binary's lifetime - it's never closed, as
	// Postgres releases its locks when the process exits
	db, err := sql.Open("postgres", dbTestDSN)
	if err != nil {
		return 0, fmt.Errorf("error opening database: %w", err)
	}
	owner, err := db.Conn(ctx)
	if err != nil {
		return 0, fmt.Errorf("error taking connection: %w", err)
	}

	deadline := time.Now().Add(3 * time.Minute)
	for {
		for s := range slotCount {
			var got bool
			if err := owner.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, int64(slotLockBase+s)).Scan(&got); err != nil {
				return 0, fmt.Errorf("error trying claim on slot: %w", err)
			}
			if got {
				return s, flushVKDB(slotVKDB(s))
			}
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("timed out waiting for an unclaimed slot (0-%d)", slotCount-1)
		}
		time.Sleep(250 * time.Millisecond)
	}
})

// claimSlot returns this binary's slot, claiming one on first use
func claimSlot(t *testing.T) int {
	t.Helper()

	slot, err := binSlot()
	require.NoError(t, err, "error claiming slot")
	return slot
}

// slotVKDB returns the valkey database number for the given slot
func slotVKDB(slot int) int {
	return slotVKDBBase + slot
}

// flushVKDB clears out the given valkey database
func flushVKDB(n int) error {
	vc, err := valkey.DialURL(fmt.Sprintf(vkTestDSNFormat, n))
	if err != nil {
		return fmt.Errorf("error connecting to valkey: %w", err)
	}
	defer vc.Close()

	if _, err := vc.Do("FLUSHDB"); err != nil {
		return fmt.Errorf("error flushing valkey database: %w", err)
	}
	return nil
}
