package batch

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const batchSize = 100

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
	go func() {
		c.wg.Add(1)
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
	// our buffer is full, log an error but continue
	if len(c.buffer) >= cap(c.buffer) {
		logrus.WithField("label", c.label).Error("buffer full, you may want to decrease your timeout")
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
		batch := make([]interface{}, 0, len(values))

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
				err = batchSQL(ctx, c.label, c.db, c.sql, []interface{}{v})
				if err != nil {
					if c.callback != nil {
						c.callback(errors.Wrapf(err, "%s: error comitting value", c.label), v.(Value))
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

func batchSQL(ctx context.Context, label string, db *sqlx.DB, sql string, vs []interface{}) error {
	// no values, nothing to do
	if len(vs) == 0 {
		return nil
	}

	start := time.Now()

	// this will be our SQL placeholders ($1, $2,..) for values in our final query, built dynamically
	values := strings.Builder{}
	values.Grow(7 * len(vs))

	// this will be each of the arguments to match the positional values above
	args := make([]interface{}, 0, len(vs)*5)

	// for each value we build a bound SQL statement, then extract the values clause
	for i, value := range vs {
		valueSQL, valueArgs, err := sqlx.Named(sql, value)
		if err != nil {
			return errors.Wrapf(err, "error converting bulk insert args")
		}

		args = append(args, valueArgs...)
		argValues, err := extractValues(valueSQL)
		if err != nil {
			return errors.Wrapf(err, "error extracting values from sql: %s", valueSQL)
		}

		// append to our global values, adding comma if necessary
		values.WriteString(argValues)
		if i+1 < len(vs) {
			values.WriteString(",")
		}
	}

	valuesSQL, err := extractValues(sql)
	if err != nil {
		return errors.Wrapf(err, "error extracting values from sql: %s", sql)
	}

	bulkInsert := db.Rebind(strings.Replace(sql, valuesSQL, values.String(), -1))

	// insert them all at once
	rows, err := db.QueryxContext(ctx, bulkInsert, args...)
	if err != nil {
		return errors.Wrapf(err, "error during bulk insert")
	}
	defer rows.Close()

	// iterate our remaining rows
	for rows.Next() {
	}

	// check for any error
	if rows.Err() != nil {
		return errors.Wrapf(rows.Err(), "error in row cursor")
	}

	logrus.WithField("elapsed", time.Since(start)).WithField("rows", len(vs)).Infof("%s bulk sql complete", label)

	return nil
}

// extractValues extracts the portion between `VALUE(` and `)` in the passed in string. (leaving VALUE but not the parentheses)
func extractValues(sql string) (string, error) {
	startValues := strings.Index(sql, "VALUES(")
	if startValues <= 0 {
		return "", errors.Errorf("unable to find VALUES( in bulk insert SQL: %s", sql)
	}

	// find the matching end parentheses, we need to count balanced parentheses here
	openCount := 1
	endValues := -1
	for i, r := range sql[startValues+7:] {
		if r == '(' {
			openCount++
		} else if r == ')' {
			openCount--
			if openCount == 0 {
				endValues = i + startValues + 7
				break
			}
		}
	}

	if endValues <= 0 {
		return "", errors.Errorf("unable to find end of VALUES() in bulk insert sql: %s", sql)
	}

	return sql[startValues+6 : endValues+1], nil
}
