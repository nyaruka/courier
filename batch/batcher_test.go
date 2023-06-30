package batch_test

import (
	"sync"
	"testing"
	"time"

	"github.com/nyaruka/courier/batch"
	"github.com/stretchr/testify/assert"
)

func TestBatcher(t *testing.T) {
	batches := make([][]int, 0, 5)

	wg := &sync.WaitGroup{}
	b := batch.NewBatcher(func(batch []int) {
		batches = append(batches, batch)
	}, time.Second, 3, wg)

	b.Start()

	assert.Equal(t, 2, b.Queue(1))
	assert.Equal(t, 1, b.Queue(2))
	assert.Equal(t, 0, b.Queue(3))
	assert.Equal(t, 2, b.Queue(4)) // blocks until 1,2,3 processed
	assert.Equal(t, 1, b.Queue(5))

	b.Stop()
	wg.Wait()

	assert.Equal(t, [][]int{{1, 2, 3}, {4, 5}}, batches)
}
