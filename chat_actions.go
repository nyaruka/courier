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

const (
	// ChatActionTypingStarted shows a typing indicator to the contact. It expires on the platform's own
	// schedule so should be resent at the interval the handler declares via ChatActions to sustain it.
	ChatActionTypingStarted ChatAction = "typing_started"

	// ChatActionTypingStopped clears a typing indicator - not yet implemented by any handler and only
	// expressible on some platforms (elsewhere indicators can only expire on their own)
	ChatActionTypingStopped ChatAction = "typing_stopped"

	// ChatActionMarkRead shows the contact that their messages have been read
	ChatActionMarkRead ChatAction = "mark_read"
)

// ChatActionSend is a request to send a chat action to a contact. MsgExternalID is the platform's own
// identifier for the newest incoming message (as seen on msg_received events) and is required by channels
// whose actions reference a message, e.g. WhatsApp.
type ChatActionSend struct {
	Action        ChatAction `json:"action"                    validate:"required,oneof=typing_started typing_stopped mark_read"`
	URN           urns.URN   `json:"urn"                       validate:"required"`
	MsgExternalID string     `json:"msg_external_id,omitempty"`
}

type sendChatActionRequest struct {
	ChatActionSend
	ChannelType models.ChannelType `json:"channel_type" validate:"required"`
	ChannelUUID models.ChannelUUID `json:"channel_uuid" validate:"required,uuid"`
}

type sendChatActionResponse struct {
	Supported bool `json:"supported"`
	Interval  int  `json:"interval,omitempty"` // seconds until the action should be resent to sustain it
}

// Handles a chat action send request. Callers should treat supported=false as "stop sending for this
// conversation" and any error response as "stop sending until a new typing session starts" - a failed send
// means no indicator is showing, and this bounds how many error logs a broken channel can generate.
//
// Sustained actions (interval > 0) are throttled per conversation: repeats within the interval are
// reported as success without a send, so callers can relay actions at whatever cadence suits them and the
// platform still sees at most one send per interval.
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
	interval, supported := handler.ChatActions(ch)[sa.Action]
	if !supported {
		return &sendChatActionResponse{Supported: false}, nil
	}

	resp := &sendChatActionResponse{Supported: true, Interval: int(interval / time.Second)}

	// sustained actions are throttled to their interval with a valkey key that expires when the platform
	// needs a resend - one-shot actions (interval 0) always go through
	throttleKey := fmt.Sprintf("chat-actions:%s|%s|%s", ch.UUID(), sa.URN.Identity(), sa.Action)
	if interval > 0 {
		rc := s.rt.VK.Get()
		reply, err := rc.Do("SET", throttleKey, "1", "EX", int(interval/time.Second), "NX")
		rc.Close()
		if err != nil {
			// a valkey problem shouldn't break chat actions so proceed unthrottled
			slog.Error("error checking chat action throttle", "error", err, "key", throttleKey)
		} else if reply == nil {
			return resp, nil // already sent within the interval
		}
	}

	clog := NewChannelLogForChatActionSend(ch, handler.RedactValues(ch))

	err = handler.SendChatAction(ctx, ch, &sa.ChatActionSend, clog)

	// chat actions are frequent and boring when they succeed so we only write logs for errors
	clog.End()
	if clog.IsError() {
		if logErr := s.backend.WriteChannelLog(ctx, clog); logErr != nil {
			slog.Error("error writing log", "error", logErr)
		}
	}

	if err != nil {
		// a failed send didn't show an indicator, so clear the throttle rather than suppressing the next
		// attempt - e.g. a new typing session starting within the interval
		if interval > 0 {
			rc := s.rt.VK.Get()
			if _, delErr := rc.Do("DEL", throttleKey); delErr != nil {
				slog.Error("error clearing chat action throttle", "error", delErr, "key", throttleKey)
			}
			rc.Close()
		}

		return nil, fmt.Errorf("error sending %s action on channel %s: %w", sa.Action, ch.UUID(), err)
	}

	return resp, nil
}
