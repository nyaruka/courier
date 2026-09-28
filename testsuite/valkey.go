package testsuite

import "github.com/nyaruka/vkutil/assertvk"

// Each test binary claims its own valkey database, so that concurrent test runs sharing a valkey can't interfere with
// each other. On a valkey with enough databases, claims are coordinated with those of other projects' tests.
func init() {
	assertvk.Coordinate(16, 17, 63)
}
