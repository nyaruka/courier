package courier

import (
	"time"

	"github.com/nyaruka/courier/v26/core/models"
)

// channelTypeInfo describes the capabilities of a channel type so that callers don't need to encode
// platform knowledge themselves. Chat action intervals are in seconds.
type channelTypeInfo struct {
	ChatActions map[ChatAction]int `json:"chat_actions,omitempty"`
}

// channelTypes returns info for every active channel type that has something to declare - callers should
// treat absent types as having no capabilities.
func channelTypes() map[models.ChannelType]*channelTypeInfo {
	infos := make(map[models.ChannelType]*channelTypeInfo)

	for typ, handler := range activeHandlers {
		actions := handler.ChatActions()
		if len(actions) > 0 {
			intervals := make(map[ChatAction]int, len(actions))
			for action, interval := range actions {
				intervals[action] = int(interval / time.Second)
			}
			infos[typ] = &channelTypeInfo{ChatActions: intervals}
		}
	}

	return infos
}
