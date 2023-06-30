package batch

import (
	"sync"
	"time"
)

// Batcher allows values to be queued and processed in a background thread.
type Batcher[T any] struct {
	process func(batch []T)
	timeout time.Duration
	wg      *sync.WaitGroup
	buffer  chan T
	stop    chan bool
}

// NewBatcher creates a new batcher.
func NewBatcher[T any](process func(batch []T), timeout time.Duration, capacity int, wg *sync.WaitGroup) *Batcher[T] {
	return &Batcher[T]{
		process: process,
		timeout: timeout,
		wg:      wg,
		buffer:  make(chan T, capacity),
		stop:    make(chan bool),
	}
}

// Start starts our batcher
func (b *Batcher[T]) Start() {
	b.wg.Add(1)

	go func() {
		defer b.wg.Done()

		for {
			select {
			case <-b.stop:
				for len(b.buffer) > 0 {
					b.flush()
				}
				return

			case <-time.After(b.timeout):
				b.flush()
			}
		}
	}()
}

// Queue queues the given value, potentially blocking. Returns the new free capacity.
func (b *Batcher[T]) Queue(value T) int {
	b.buffer <- value

	return cap(b.buffer) - len(b.buffer)
}

// Stop stops this batcher
func (b *Batcher[T]) Stop() {
	close(b.stop)
}

func (b *Batcher[T]) flush() {
	count := len(b.buffer)
	if count <= 0 {
		return
	}

	batch := make([]T, count)
	for i := 0; i < count; i++ {
		v := <-b.buffer
		batch[i] = v
	}

	b.process(batch)
}
