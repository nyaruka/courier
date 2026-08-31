package channels

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/nyaruka/courier/v26/core/models"
)

// for use in request.Context
type contextKey int

const (
	contextRequestURL contextKey = iota
	contextRequestStart
)

// WithRequestContext returns a context carrying the request details that the logging below reports. The web server
// puts them there before invoking a handler, so that the seam can log without being passed them explicitly.
func WithRequestContext(ctx context.Context, url string, start time.Time) context.Context {
	ctx = context.WithValue(ctx, contextRequestURL, url)
	return context.WithValue(ctx, contextRequestStart, start)
}

// LogRequestIgnored logs that we ignored the passed in request
func LogRequestIgnored(r *http.Request, channel *models.Channel, details string) {
	if slog.Default().Enabled(r.Context(), slog.LevelDebug) {
		slog.Debug("request ignored",
			"channel_uuid", channel.UUID(),
			"url", r.Context().Value(contextRequestURL),
			"elapsed_ms", getElapsedMS(r),
			"details", details,
		)
	}
}

// LogRequestError logs a request that errored during parsing (this is logged as an info as it isn't an error on our side)
func LogRequestError(r *http.Request, channel *models.Channel, err error) {
	log := slog.With(
		"url", r.Context().Value(contextRequestURL),
		"elapsed_ms", getElapsedMS(r),
		"error", err,
	)

	if channel != nil {
		log = log.With("channel_uuid", channel.UUID())
	}
	log.Info("request errored")
}

func getElapsedMS(r *http.Request) float64 {
	start := r.Context().Value(contextRequestStart)
	if start == nil {
		return -1
	}
	startTime, isTime := start.(time.Time)
	if !isTime {
		return -1
	}
	return float64(time.Since(startTime)) / float64(time.Millisecond)
}
