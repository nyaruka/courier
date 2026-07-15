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
// Relayable events are the engine event types that can be relayed to the channel's platform, mapped to
// their resend intervals in seconds. Intended to grow other capability info over time, e.g. messaging
// window durations.
type channelInfo struct {
	RelayableEvents map[string]int `json:"relayable_events,omitempty"`
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
		if relayable := handler.RelayableEvents(ch); len(relayable) > 0 {
			info.RelayableEvents = make(map[string]int, len(relayable))
			for eventType, interval := range relayable {
				info.RelayableEvents[eventType] = int(interval / time.Second)
			}
		}
	}

	return info, nil
}
