package courier

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/nyaruka/courier/v26/core/models"
)

// channelInfo describes the capabilities of a channel so that callers don't need to encode platform
// knowledge themselves. Capabilities can vary between channels of the same type, e.g. by config. Chat
// action intervals are in seconds. Intended to grow other capability info over time, e.g. messaging
// window durations.
type channelInfo struct {
	ChatActions map[ChatAction]int `json:"chat_actions,omitempty"`
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
		if actions := handler.ChatActions(ch); len(actions) > 0 {
			info.ChatActions = make(map[ChatAction]int, len(actions))
			for action, interval := range actions {
				info.ChatActions[action] = int(interval / time.Second)
			}
		}
	}

	return info, nil
}
