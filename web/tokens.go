package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/utils"
)

type refreshTokenRequest struct {
	ChannelType models.ChannelType `json:"channel_type" validate:"required"`
	ChannelUUID models.ChannelUUID `json:"channel_uuid" validate:"required,uuid"`
}

type refreshTokenResponse struct {
	Token string `json:"token"`
}

// refreshToken fetches a new access token for a channel whose handler uses short-lived tokens, replacing the one the
// handler uses for sends, so that other services can call the channel's API without obtaining tokens themselves.
func refreshToken(ctx context.Context, rt *runtime.Runtime, r *http.Request) (*refreshTokenResponse, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading request body: %w", err)
	}

	req := &refreshTokenRequest{}
	if err := json.Unmarshal(body, req); err != nil {
		return nil, fmt.Errorf("error unmarshalling request: %w", err)
	}
	if err := utils.Validate(req); err != nil {
		return nil, err
	}

	ch, err := models.GetChannel(ctx, req.ChannelType, req.ChannelUUID)
	if err != nil {
		return nil, fmt.Errorf("error getting channel: %w", err)
	}

	handler := channels.GetHandler(ch.ChannelType())
	refresher, ok := handler.(channels.AccessTokenRefresher)
	if !ok {
		return nil, fmt.Errorf("channel type %s doesn't support access tokens", ch.ChannelType())
	}

	clog := models.NewChannelLog(models.ChannelLogTypeTokenRefresh, ch, nil, handler.RedactValues(ch))

	token, err := refresher.RefreshAccessToken(ctx, ch, clog)

	clog.End()
	if handler.StoreChannelLogs() {
		models.WriteChannelLog(rt, clog)
	}

	if err != nil {
		return nil, fmt.Errorf("error refreshing access token for channel %s: %w", ch.UUID(), err)
	}

	return &refreshTokenResponse{Token: token}, nil
}
