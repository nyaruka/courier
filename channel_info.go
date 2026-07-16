package courier

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/nyaruka/courier/v26/core/models"
)

// channelInfo describes the capabilities of a channel so that callers don't need to encode platform
// knowledge themselves. Capabilities can vary between channels of the same type, e.g. by config.
// Sendable events are the engine event types that can be sent to the channel's platform, mapped to
// their resend intervals in seconds. Intended to grow other capability info over time, e.g. messaging
// window durations.
type channelInfo struct {
	SendableEvents map[string]int `json:"sendable_events,omitempty"`
}

func getChannelInfo(ctx context.Context, s *Server, r *http.Request) (*channelInfo, error) {
	uuid := models.ChannelUUID(r.PathValue("uuid"))

	ch, err := s.backend.GetChannel(ctx, models.AnyChannelType, uuid)
	if err != nil {
		return nil, fmt.Errorf("error getting channel: %w", err)
	}

	info := &channelInfo{}

	handler := s.GetHandler(ch)
	if handler != nil {
		if sendable := handler.SendableEvents(ch); len(sendable) > 0 {
			info.SendableEvents = make(map[string]int, len(sendable))
			for eventType, interval := range sendable {
				info.SendableEvents[eventType] = int(interval / time.Second)
			}
		}
	}

	return info, nil
}
