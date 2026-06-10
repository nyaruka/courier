package courier

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/utils"
	"github.com/nyaruka/courier/v26/utils/clogs"
	"github.com/nyaruka/gocommon/urns"
)

// EventOutType is the type of a transient event that can be sent to a contact on a channel
type EventOutType string

// EventOutTypeTyping is a typing indicator shown to the contact
const EventOutTypeTyping EventOutType = "typing"

// EventSender is the interface handlers should satisfy if they can send transient events such as typing
// indicators to contacts. Unlike message sends these are fire and forget - they aren't queued and there
// are no statuses or retries.
type EventSender interface {
	SendEvent(context.Context, Channel, EventOutType, urns.URN, *ChannelLog) error
}

type sendEventRequest struct {
	Type        EventOutType       `json:"type"         validate:"required,eq=typing"`
	ChannelType models.ChannelType `json:"channel_type" validate:"required"`
	ChannelUUID models.ChannelUUID `json:"channel_uuid" validate:"required,uuid"`
	URN         urns.URN           `json:"urn"          validate:"required"`
}

type sendEventResponse struct {
	LogUUID clogs.UUID `json:"log_uuid"`
}

func sendEvent(ctx context.Context, b Backend, r *http.Request) (*sendEventResponse, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading request body: %w", err)
	}

	se := &sendEventRequest{}
	if err := json.Unmarshal(body, se); err != nil {
		return nil, fmt.Errorf("error unmarshalling request: %w", err)
	}
	if err := utils.Validate(se); err != nil {
		return nil, err
	}

	ch, err := b.GetChannel(ctx, se.ChannelType, se.ChannelUUID)
	if err != nil {
		return nil, fmt.Errorf("error getting channel: %w", err)
	}

	handler := GetHandler(ch.ChannelType())
	sender, canSend := handler.(EventSender)
	if !canSend {
		return nil, fmt.Errorf("channel type %s can't send %s events", ch.ChannelType(), se.Type)
	}

	clog := NewChannelLogForEventSend(ch, handler.RedactValues(ch))

	err = sender.SendEvent(ctx, ch, se.Type, se.URN, clog)

	// try to write channel log even if we have an error
	clog.End()
	if logErr := b.WriteChannelLog(ctx, clog); logErr != nil {
		slog.Error("error writing log", "error", logErr)
	}

	if err != nil {
		return nil, fmt.Errorf("error sending %s event on channel %s: %w", se.Type, ch.UUID(), err)
	}

	return &sendEventResponse{LogUUID: clog.UUID}, nil
}
