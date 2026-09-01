package test

import (
	"flag"
	"testing"

	"github.com/nyaruka/gocommon/jsonx"
	diff "github.com/sergi/go-diff/diffmatchpatch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// UpdateSnapshots is whether tests should rewrite their snapshot files with actual values rather than checking
// against them - set by passing -update to go test.
var UpdateSnapshots bool

func init() {
	flag.BoolVar(&UpdateSnapshots, "update", false, "whether to update test snapshots")
}

// NormalizeJSON re-formats the given JSON so that formatting differences don't matter
func NormalizeJSON(data []byte) ([]byte, error) {
	var asGeneric any
	if err := jsonx.Unmarshal(data, &asGeneric); err != nil {
		return nil, err
	}
	return jsonx.MarshalPretty(asGeneric)
}

// AssertEqualJSON checks two JSON values for equality, failing with a readable diff if they differ
func AssertEqualJSON(t *testing.T, expected, actual []byte, message string) bool {
	t.Helper()

	expectedNormalized, err := NormalizeJSON(expected)
	require.NoError(t, err, "%s: unable to normalize expected JSON: %s", message, string(expected))

	actualNormalized, err := NormalizeJSON(actual)
	require.NoError(t, err, "%s: unable to normalize actual JSON: %s", message, string(actual))

	differ := diff.New()
	diffs := differ.DiffMain(string(expectedNormalized), string(actualNormalized), false)

	if len(diffs) != 1 || diffs[0].Type != diff.DiffEqual {
		assert.Fail(t, message, differ.DiffPrettyText(diffs))
		return false
	}
	return true
}
