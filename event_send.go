package courier

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/utils"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/core/events"
)

// sendEventRequest is a request to send an engine event, e.g. a user typing indicator, to a channel's
// platform. Unlike message sends these are fire and forget - they aren't queued and there are no statuses
// or retries. The event's own channel/urn/msg_external_id fields say where it should go - channel_type is
// needed alongside because channels are looked up by type + UUID.
type sendEventRequest struct {
	ChannelType models.ChannelType `json:"channel_type" validate:"required"`
	Event       json.RawMessage    `json:"event"        validate:"required"`
}

type sendEventResponse struct {
	Supported bool `json:"supported"`
	Interval  int  `json:"interval,omitempty"` // seconds until the event should be resent to sustain its effect
}

// Handles an event send request. Callers should treat supported=false as "stop sending for this
// conversation" and any error response as "stop sending until a new typing session starts" - a failed
// send means no indicator is showing, and this bounds how many error logs a broken channel can generate.
//
// Sustained events (interval > 0) are throttled per conversation: repeats within the interval are
// reported as success without a send, so callers can send events at whatever cadence suits them and the
// platform still sees at most one send per interval.
func sendEvent(ctx context.Context, s *Server, r *http.Request) (*sendEventResponse, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading request body: %w", err)
	}

	req := &sendEventRequest{}
	if err := json.Unmarshal(body, req); err != nil {
		return nil, fmt.Errorf("error unmarshalling request: %w", err)
	}
	if err := utils.Validate(req); err != nil {
		return nil, err
	}

	event, err := events.Read(req.Event)
	if err != nil {
		return nil, fmt.Errorf("error reading event: %w", err)
	}

	// the event types we know how to send, and their routing fields
	var direction events.Direction
	var channelRef *assets.ChannelReference
	var urn urns.URN
	switch typed := event.(type) {
	case *events.TypingStarted:
		direction, channelRef, urn = typed.Direction, typed.Channel, typed.URN
	case *events.TypingStopped:
		direction, channelRef, urn = typed.Direction, typed.Channel, typed.URN
	default:
		return nil, fmt.Errorf("%s is not a sendable event type", event.Type())
	}

	// sending to a platform only makes sense for events originating from a user or bot
	if direction != events.DirectionOutgoing {
		return nil, fmt.Errorf("only outgoing events can be sent")
	}
	if channelRef == nil || urn == urns.NilURN {
		return nil, fmt.Errorf("event requires channel and urn to be sent")
	}

	ch, err := s.backend.GetChannel(ctx, req.ChannelType, models.ChannelUUID(channelRef.UUID))
	if err != nil {
		return nil, fmt.Errorf("error getting channel: %w", err)
	}

	handler := s.GetHandler(ch)
	if handler == nil {
		return &sendEventResponse{Supported: false}, nil
	}
	interval, supported := handler.SendableEvents(ch)[event.Type()]
	if !supported {
		return &sendEventResponse{Supported: false}, nil
	}

	intervalSecs := int(interval / time.Second)
	resp := &sendEventResponse{Supported: true, Interval: intervalSecs}

	// sustained events are throttled to their interval with a valkey key that expires when the platform
	// needs a resend - one-shot events (interval 0, or anything below our 1 second resolution) always
	// go through
	throttleKey := fmt.Sprintf("event-sends:%s|%s|%s", ch.UUID(), urn.Identity(), event.Type())
	if intervalSecs > 0 {
		rc := s.rt.VK.Get()
		reply, err := rc.Do("SET", throttleKey, "1", "EX", intervalSecs, "NX")
		rc.Close()
		if err != nil {
			// a valkey problem shouldn't break event sending so proceed unthrottled
			slog.Error("error checking event send throttle", "error", err, "key", throttleKey)
		} else if reply == nil {
			return resp, nil // already sent within the interval
		}
	}

	// a typing stopped event ends the typing session, so clear any typing started throttle - otherwise
	// a new session starting within the interval would have its first typing started suppressed
	if event.Type() == events.TypeTypingStopped {
		startedKey := fmt.Sprintf("event-sends:%s|%s|%s", ch.UUID(), urn.Identity(), events.TypeTypingStarted)
		rc := s.rt.VK.Get()
		if _, err := rc.Do("DEL", startedKey); err != nil {
			slog.Error("error clearing event send throttle", "error", err, "key", startedKey)
		}
		rc.Close()
	}

	clog := NewChannelLogForEventSend(ch, handler.RedactValues(ch))

	err = handler.SendEvent(ctx, ch, event, clog)

	// event sends are frequent and boring when they succeed so we only write logs for errors
	clog.End()
	if clog.IsError() {
		if logErr := s.backend.WriteChannelLog(ctx, clog); logErr != nil {
			slog.Error("error writing log", "error", logErr)
		}
	}

	if err != nil {
		// a failed send didn't show an indicator, so clear the throttle rather than suppressing the next
		// attempt - e.g. a new typing session starting within the interval
		if intervalSecs > 0 {
			rc := s.rt.VK.Get()
			if _, delErr := rc.Do("DEL", throttleKey); delErr != nil {
				slog.Error("error clearing event send throttle", "error", delErr, "key", throttleKey)
			}
			rc.Close()
		}

		return nil, fmt.Errorf("error sending %s event on channel %s: %w", event.Type(), ch.UUID(), err)
	}

	return resp, nil
}
