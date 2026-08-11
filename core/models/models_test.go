package models_test

import (
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartAndStop(t *testing.T) {
	_, rt := testsuite.Runtime(t) // starts the models layer

	// starting again is safe - each Start stops what the previous one left running, so that a process which
	// starts the layer repeatedly (as the handler test harness does) doesn't accumulate caches and spools
	require.NoError(t, models.Start(rt))
	require.NoError(t, models.Start(rt))

	// stopping is idempotent, and safe when nothing is started
	models.Stop()
	models.Stop()

	// the spool sizes reported to metrics tolerate a stopped layer rather than panicking
	msgs, statuses, events := models.SpoolSizes()
	assert.Equal(t, 0, msgs+statuses+events)

	// and it can be started again afterwards, which the suite's cleanup then stops
	require.NoError(t, models.Start(rt))
}
