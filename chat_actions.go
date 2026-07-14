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
)

// ChatAction is an ephemeral action performed against a chat, e.g. a typing indicator. Unlike message
// sends these are fire and forget - they aren't queued and there are no statuses or retries.
type ChatAction string

// ChatActionTypingStarted shows a typing indicator to the contact. It expires on the platform's own
// schedule so should be resent at the interval the handler declares via ChatActionSupport to sustain it.
const ChatActionTypingStarted ChatAction = "typing_started"

type sendChatActionRequest struct {
	Action      ChatAction         `json:"action"       validate:"required,eq=typing_started"`
	ChannelType models.ChannelType `json:"channel_type" validate:"required"`
	ChannelUUID models.ChannelUUID `json:"channel_uuid" validate:"required,uuid"`
	URN         urns.URN           `json:"urn"          validate:"required"`
}

type sendChatActionResponse struct {
	Supported bool `json:"supported"`
	Interval  int  `json:"interval,omitempty"` // seconds until the action should be resent to sustain it
}

// Handles a chat action send request. Callers should treat supported=false as "stop sending for this
// conversation" and any error response as "stop sending until a new typing session starts" - a failed send
// means no indicator is showing, and this bounds how many error logs a broken channel can generate.
func sendChatAction(ctx context.Context, s *Server, r *http.Request) (*sendChatActionResponse, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading request body: %w", err)
	}

	sa := &sendChatActionRequest{}
	if err := json.Unmarshal(body, sa); err != nil {
		return nil, fmt.Errorf("error unmarshalling request: %w", err)
	}
	if err := utils.Validate(sa); err != nil {
		return nil, err
	}

	ch, err := s.backend.GetChannel(ctx, sa.ChannelType, sa.ChannelUUID)
	if err != nil {
		return nil, fmt.Errorf("error getting channel: %w", err)
	}

	handler := s.GetHandler(ch)
	if handler == nil {
		return &sendChatActionResponse{Supported: false}, nil
	}
	interval, supported := handler.ChatActions()[sa.Action]
	if !supported {
		return &sendChatActionResponse{Supported: false}, nil
	}

	clog := NewChannelLogForChatActionSend(ch, handler.RedactValues(ch))

	err = handler.SendChatAction(ctx, ch, sa.Action, sa.URN, clog)

	// chat actions are frequent and boring when they succeed so we only write logs for errors
	clog.End()
	if clog.IsError() {
		if logErr := s.backend.WriteChannelLog(ctx, clog); logErr != nil {
			slog.Error("error writing log", "error", logErr)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("error sending %s action on channel %s: %w", sa.Action, ch.UUID(), err)
	}

	return &sendChatActionResponse{
		Supported: true,
		Interval:  int(interval / time.Second),
	}, nil
}
