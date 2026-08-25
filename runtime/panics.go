package runtime

import (
	"log/slog"
	"maps"
	"runtime/debug"
	"slices"
)

// PanicHandler is called with a recovered panic value and tags describing where it happened. Deployments can replace
// this to additionally report panics to an error tracking service.
var PanicHandler = LogPanic

// LogPanic is the default panic handler which logs the panic value along with a stack trace.
func LogPanic(val any, tags map[string]string) {
	attrs := make([]any, 0, len(tags)*2+4)
	for _, k := range slices.Sorted(maps.Keys(tags)) {
		attrs = append(attrs, k, tags[k])
	}
	attrs = append(attrs, "panic", val, "stack", string(debug.Stack()))

	slog.Error("recovered from panic", attrs...)
}
