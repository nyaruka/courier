package courier

import (
	"log/slog"
	"net/http"
	"time"
)

// LogMsgStatusReceived logs our that we received a new MsgStatus
func LogMsgStatusReceived(r *http.Request, status StatusUpdate) {
	if slog.Default().Enabled(r.Context(), slog.LevelDebug) {
		slog.Debug("status updated",
			"channel_uuid", status.ChannelUUID(),
			"url", r.Context().Value(contextRequestURL),
			"elapsed_ms", getElapsedMS(r),
			"status", status.Status(),
			"msg_id", status.MsgID(),
			"msg_external_id", status.ExternalID(),
		)
	}

}

// LogMsgReceived logs that we received the passed in message
func LogMsgReceived(r *http.Request, msg MsgIn) {
	if slog.Default().Enabled(r.Context(), slog.LevelDebug) {
		slog.Debug("msg received",
			"channel_uuid", msg.Channel().UUID(),
			"url", r.Context().Value(contextRequestURL),
			"elapsed_ms", getElapsedMS(r),
			"msg_uuid", msg.UUID(),
			"msg_id", msg.ID(),
			"msg_urn", msg.URN().Identity(),
			"msg_text", msg.Text(),
			"msg_attachments", msg.Attachments(),
		)
	}

}

// LogChannelEventReceived logs that we received the passed in channel event
func LogChannelEventReceived(r *http.Request, event ChannelEvent) {
	if slog.Default().Enabled(r.Context(), slog.LevelDebug) {
		slog.Debug("event received",
			"channel_uuid", event.ChannelUUID(),
			"url", r.Context().Value(contextRequestURL),
			"elapsed_ms", getElapsedMS(r),
			"event_type", event.EventType(),
			"event_urn", event.URN().Identity(),
		)
	}
}

// LogRequestIgnored logs that we ignored the passed in request
func LogRequestIgnored(r *http.Request, channel Channel, details string) {
	if slog.Default().Enabled(r.Context(), slog.LevelDebug) {
		slog.Debug("request ignored",
			"channel_uuid", channel.UUID(),
			"url", r.Context().Value(contextRequestURL),
			"elapsed_ms", getElapsedMS(r),
			"details", details,
		)
	}
}

// LogRequestHandled logs that we handled the passed in request but didn't create any events
func LogRequestHandled(r *http.Request, channel Channel, details string) {
	if slog.Default().Enabled(r.Context(), slog.LevelDebug) {
		slog.Debug("request handled",
			"channel_uuid", channel.UUID(),
			"url", r.Context().Value(contextRequestURL),
			"elapsed_ms", getElapsedMS(r),
			"details", details,
		)
	}
}

// LogRequestError logs that errored during parsing (this is logged as an info as it isn't an error on our side)
func LogRequestError(r *http.Request, channel Channel, err error) {
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
