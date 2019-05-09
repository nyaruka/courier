package courier

import (
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

// LogMsgStatusReceived logs our that we received a new MsgStatus
func LogMsgStatusReceived(r *http.Request, status MsgStatus) {
	log := logrus.WithFields(logrus.Fields{
		"channel_uuid": status.ChannelUUID(),
		"url":          r.Context().Value(contextRequestURL),
		"elapsed_ms":   getElapsedMS(r),
		"status":       status.Status(),
	})

	if status.ID() != NilMsgID {
		log = log.WithField("msg_id", status.ID())
	} else {
		log = log.WithField("msg_external_id", status.ExternalID())
	}
	log.Info("status updated")
}

// LogMsgReceived logs that we received the passed in message
func LogMsgReceived(r *http.Request, msg Msg) {
	logrus.WithFields(logrus.Fields{
		"channel_uuid":    msg.Channel().UUID(),
		"url":             r.Context().Value(contextRequestURL),
		"elapsed_ms":      getElapsedMS(r),
		"msg_uuid":        msg.UUID(),
		"msg_id":          msg.ID(),
		"msg_urn":         msg.URN().Identity(),
		"msg_text":        msg.Text(),
		"msg_attachments": msg.Attachments(),
	}).Info("msg received")
}

// LogChannelEventReceived logs that we received the passed in channel event
func LogChannelEventReceived(r *http.Request, event ChannelEvent) {
	logrus.WithFields(logrus.Fields{
		"channel_uuid": event.ChannelUUID(),
		"url":          r.Context().Value(contextRequestURL),
		"elapsed_ms":   getElapsedMS(r),
		"event_type":   event.EventType(),
		"event_urn":    event.URN().Identity(),
	}).Info("evt received")
}

// LogRequestIgnored logs that we ignored the passed in request
func LogRequestIgnored(r *http.Request, channel Channel, details string) {
	logrus.WithFields(logrus.Fields{
		"channel_uuid": channel.UUID(),
		"url":          r.Context().Value(contextRequestURL),
		"elapsed_ms":   getElapsedMS(r),
		"details":      details,
	}).Info("request ignored")
}

// LogRequestHandled logs that we handled the passed in request but didn't create any events
func LogRequestHandled(r *http.Request, channel Channel, details string) {
	logrus.WithFields(logrus.Fields{
		"channel_uuid": channel.UUID(),
		"url":          r.Context().Value(contextRequestURL),
		"elapsed_ms":   getElapsedMS(r),
		"details":      details,
	}).Info("request handled")
}

// LogRequestError logs that errored during parsing (this is logged as an info as it isn't an error on our side)
func LogRequestError(r *http.Request, channel Channel, err error) {
	log := logrus.WithFields(logrus.Fields{
		"url":        r.Context().Value(contextRequestURL),
		"elapsed_ms": getElapsedMS(r),
		"error":      err.Error(),
	})
	if channel != nil {
		log = log.WithField("channel_uuid", channel.UUID())
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
	return float64(time.Now().Sub(startTime)) / float64(time.Millisecond)
}
