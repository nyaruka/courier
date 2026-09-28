package queue_test

import (
	"os"
	"testing"

	"github.com/nyaruka/vkutil/assertvk"
)

// TestMain coordinates this binary's valkey database claim like the testsuite package does.
func TestMain(m *testing.M) {
	assertvk.Coordinate(16, 17, 63)

	os.Exit(m.Run())
}
