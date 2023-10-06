// Structured logging handler for logrus so we can rewrite code to use slog package incrementally. Once all logging is
// happening via slog, we just need to hook up Sentry directly to that, and then we can get rid of this file.
package utils

import (
	"context"
	"log/slog"
	"slices"
	"strings"

	"github.com/sirupsen/logrus"
)

var levels = map[slog.Level]logrus.Level{
	slog.LevelError: logrus.ErrorLevel,
	slog.LevelWarn:  logrus.WarnLevel,
	slog.LevelInfo:  logrus.InfoLevel,
	slog.LevelDebug: logrus.DebugLevel,
}

type LogrusHandler struct {
	logger *logrus.Logger
	groups []string
	attrs  []slog.Attr
}

func NewLogrusHandler(logger *logrus.Logger) *LogrusHandler {
	return &LogrusHandler{logger: logger}
}

func (l *LogrusHandler) clone() *LogrusHandler {
	return &LogrusHandler{
		logger: l.logger,
		groups: slices.Clip(l.groups),
		attrs:  slices.Clip(l.attrs),
	}
}

func (l *LogrusHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return levels[level] <= l.logger.GetLevel()
}

func (l *LogrusHandler) Handle(ctx context.Context, r slog.Record) error {
	log := logrus.NewEntry(l.logger)
	if r.Time.IsZero() {
		log = log.WithTime(r.Time)
	}

	f := logrus.Fields{}
	for _, a := range l.attrs {
		if a.Key != "" {
			f[a.Key] = a.Value
		}
	}
	log = log.WithFields(f)

	r.Attrs(func(attr slog.Attr) bool {
		if attr.Key == "" {
			return true
		}
		log = log.WithField(attr.Key, attr.Value)
		return true
	})
	log.Logf(levels[r.Level], r.Message)
	return nil
}

func (l *LogrusHandler) groupPrefix() string {
	if len(l.groups) > 0 {
		return strings.Join(l.groups, ":") + ":"
	}
	return ""
}

func (l *LogrusHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandler := l.clone()
	for _, a := range attrs {
		newHandler.attrs = append(newHandler.attrs, slog.Attr{
			Key:   l.groupPrefix() + a.Key,
			Value: a.Value,
		})
	}
	return newHandler
}

func (l *LogrusHandler) WithGroup(name string) slog.Handler {
	newHandler := l.clone()
	newHandler.groups = append(newHandler.groups, name)
	return newHandler
}

var _ slog.Handler = &LogrusHandler{}
