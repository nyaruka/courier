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
	"github.com/nyaruka/courier/v26/utils/clogs"
	"github.com/nyaruka/gocommon/urns"
)

// ChatAction is an ephemeral action performed against a chat, e.g. a typing indicator. Unlike message
// sends these are fire and forget - they aren't queued and there are no statuses or retries.
type ChatAction string

// ChatActionTypingStarted shows a typing indicator to the contact. It expires on the platform's own
// schedule so should be resent every ChatActionInterval to sustain it.
const ChatActionTypingStarted ChatAction = "typing_started"

// ChatActionSender is the interface handlers should satisfy if they can send chat actions to contacts.
type ChatActionSender interface {
	// SendChatAction sends the given action to the given URN
	SendChatAction(context.Context, Channel, ChatAction, urns.URN, *ChannelLog) error

	// ChatActionInterval returns how often the given action should be resent to sustain it - i.e. the
	// platform's display TTL minus a safety margin - or zero if it doesn't need resending
	ChatActionInterval(ChatAction) time.Duration
}

type sendChatActionRequest struct {
	Action      ChatAction         `json:"action"       validate:"required,eq=typing_started"`
	ChannelType models.ChannelType `json:"channel_type" validate:"required"`
	ChannelUUID models.ChannelUUID `json:"channel_uuid" validate:"required,uuid"`
	URN         urns.URN           `json:"urn"          validate:"required"`
}

type sendChatActionResponse struct {
	Supported bool       `json:"supported"`
	Interval  int        `json:"interval,omitempty"` // seconds until the action should be resent to sustain it
	LogUUID   clogs.UUID `json:"log_uuid,omitempty"`
}

func sendChatAction(ctx context.Context, b Backend, r *http.Request) (*sendChatActionResponse, error) {
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

	ch, err := b.GetChannel(ctx, sa.ChannelType, sa.ChannelUUID)
	if err != nil {
		return nil, fmt.Errorf("error getting channel: %w", err)
	}

	handler := GetHandler(ch.ChannelType())
	sender, canSend := handler.(ChatActionSender)
	if !canSend {
		return &sendChatActionResponse{Supported: false}, nil
	}

	clog := NewChannelLogForChatActionSend(ch, handler.RedactValues(ch))

	err = sender.SendChatAction(ctx, ch, sa.Action, sa.URN, clog)

	// try to write channel log even if we have an error
	clog.End()
	if logErr := b.WriteChannelLog(ctx, clog); logErr != nil {
		slog.Error("error writing log", "error", logErr)
	}

	if err != nil {
		return nil, fmt.Errorf("error sending %s action on channel %s: %w", sa.Action, ch.UUID(), err)
	}

	return &sendChatActionResponse{
		Supported: true,
		Interval:  int(sender.ChatActionInterval(sa.Action) / time.Second),
		LogUUID:   clog.UUID,
	}, nil
}
