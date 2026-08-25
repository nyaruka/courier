package runtime_test

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/nyaruka/courier/v26/runtime"
	"github.com/stretchr/testify/assert"
)

func TestLogPanic(t *testing.T) {
	buf := &bytes.Buffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelError})))
	defer slog.SetDefault(prev)

	func() {
		defer func() {
			if val := recover(); val != nil {
				runtime.LogPanic(val, map[string]string{"listener": "internal", "cron": "test"})
			}
		}()

		panic("oh no")
	}()

	out := buf.String()

	assert.Contains(t, out, `msg="recovered from panic"`)
	assert.Contains(t, out, `cron=test listener=internal panic="oh no"`) // tags sorted, before panic value
	assert.Contains(t, out, `runtime_test.TestLogPanic`)                 // stack includes the panicking function
}
