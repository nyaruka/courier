package batch

import (
	"context"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/gocommon/dbutil"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const batchSize = 1000

// Committer commits items in a background thread
type Committer interface {
	Start()
	Queue(Value)
	Stop()
}

// Value represents the interface values must satisfy. The RowID() method should return the unique row ID (if any) for the
// item. This will be used to make sure that no update is made on the same value in the same statement. (which would
// fail for bulk updates) Items which are being inserted may return an empty string for RowID()
type Value interface {
	RowID() string
}

// ErrorCallback lets callers get a callback when a value fails to be committed
type ErrorCallback func(err error, value Value)

// NewCommitter creates a new committer that will commit items in batches as quickly as possible.
func NewCommitter(label string, db *sqlx.DB, sql string, timeout time.Duration, wg *sync.WaitGroup, callback ErrorCallback) Committer {
	return &committer{
		db:       db,
		label:    label,
		sql:      sql,
		timeout:  timeout,
		callback: callback,

		wg:     wg,
		stop:   make(chan bool),
		buffer: make(chan Value, 1000),
	}
}

// Start starts our committer
func (c *committer) Start() {
	c.wg.Add(1)

	go func() {
		defer c.wg.Done()

		for {
			select {
			case <-c.stop:
				for len(c.buffer) > 0 {
					c.flush(batchSize)
				}
				logrus.WithField("label", c.label).Info("committer flushed and exiting")
				return

			case <-time.After(c.timeout):
				count := len(c.buffer)
				for i := 0; i <= count/batchSize; i++ {
					c.flush(batchSize)
				}
			}
		}
	}()
}

// Queue will queue the passed in value to committed. This will block in cases where our buffer is full
func (c *committer) Queue(value Value) {
	// our buffer is full, log an error but continue (our channel will block)
	if len(c.buffer) >= cap(c.buffer) {
		logrus.WithField("label", c.label).Error("buffer full, you may want to decrease your timeout")
		time.Sleep(250 * time.Millisecond)
	} else {
		// we are approaching our max size, start slowing down queueing so we can catch up
		if len(c.buffer) > int(float64(cap(c.buffer))*.90) {
			time.Sleep(100 * time.Millisecond)
		}
	}

	c.buffer <- value
}

// Stop stops our committer, callers can use the WaitGroup used during initialization to block for stop
func (c *committer) Stop() {
	close(c.stop)
}

// flushes up to size items from our queue, returning whether any items were committed
func (c *committer) flush(size int) bool {
	if len(c.buffer) <= 0 {
		return false
	}

	start := time.Now()

	// gather our values
	values := make([]Value, 0, size)

	// build a batch of values
	for i := 0; i < size; i++ {
		select {
		case v := <-c.buffer:
			values = append(values, v)
		default:
			break
		}
	}

	count := len(values)

	for len(values) > 0 {
		ids := make(map[string]bool, len(values))
		dupes := make([]Value, 0)
		batch := make([]Value, 0, len(values))

		// dedupe our values into a batch, we don't want to do a batch update that includes the same row more
		// thank once. we dedupe our batch here and commit in another query
		for _, v := range values {
			if ids[v.RowID()] {
				dupes = append(dupes, v)
			} else {
				batch = append(batch, v)
				id := v.RowID()

				// only track values with a row id
				if id != "" {
					ids[id] = true
				}
			}
		}

		// commit our batch
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		err := batchSQL(ctx, c.label, c.db, c.sql, batch)

		// if we received an error, try again one at a time (in case it is one value hanging us up)
		if err != nil {
			for _, v := range batch {
				err = batchSQL(ctx, c.label, c.db, c.sql, []Value{v})
				if err != nil {
					if c.callback != nil {
						c.callback(errors.Wrapf(err, "%s: error committing value", c.label), v)
					}
				}
			}
		}

		values = dupes
	}

	logrus.WithField("elapsed", time.Since(start)).WithField("label", c.label).WithField("count", count).Debug("batch committed")
	return true
}

type committer struct {
	db       *sqlx.DB
	label    string
	sql      string
	timeout  time.Duration
	callback ErrorCallback

	wg     *sync.WaitGroup
	stop   chan bool
	buffer chan Value
}

func batchSQL[T any](ctx context.Context, label string, db *sqlx.DB, sql string, vs []T) error {
	// no values, nothing to do
	if len(vs) == 0 {
		return nil
	}

	start := time.Now()

	err := dbutil.BulkQuery(ctx, db, sql, vs)
	if err != nil {
		return err
	}

	logrus.WithField("elapsed", time.Since(start)).WithField("rows", len(vs)).Debugf("%s bulk sql complete", label)

	return nil
}
