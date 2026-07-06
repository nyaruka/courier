package courier_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nyaruka/courier/v26"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureSpoolDirPresent(t *testing.T) {
	spoolDir := t.TempDir()

	// creates the subdir if it doesn't exist
	assert.NoError(t, courier.EnsureSpoolDirPresent(spoolDir, "msgs"))
	info, err := os.Stat(filepath.Join(spoolDir, "msgs"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// ok with a subdir that already exists
	assert.NoError(t, courier.EnsureSpoolDirPresent(spoolDir, "msgs"))

	// errors if the path exists but can't be written to (a file rather than a directory here, since
	// permission based cases don't apply when running tests as root)
	require.NoError(t, os.WriteFile(filepath.Join(spoolDir, "statuses"), []byte("!"), 0640))
	assert.Error(t, courier.EnsureSpoolDirPresent(spoolDir, "statuses"))
}
