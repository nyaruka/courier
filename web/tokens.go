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

type fetchTokenRequest struct {
	ChannelType models.ChannelType `json:"channel_type" validate:"required"`
	ChannelUUID models.ChannelUUID `json:"channel_uuid" validate:"required,uuid"`
}

type fetchTokenResponse struct {
	Token string `json:"token"`
}

// fetchToken returns an access token for a channel whose handler manages short-lived tokens, so that other services can
// call the channel's API without obtaining tokens themselves.
func fetchToken(ctx context.Context, rt *runtime.Runtime, r *http.Request) (*fetchTokenResponse, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading request body: %w", err)
	}

	req := &fetchTokenRequest{}
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
	provider, ok := handler.(channels.AccessTokenProvider)
	if !ok {
		return nil, fmt.Errorf("channel type %s doesn't support access tokens", ch.ChannelType())
	}

	clog := models.NewChannelLog(models.ChannelLogTypeTokenRefresh, ch, nil, handler.RedactValues(ch))

	token, err := provider.GetAccessToken(ctx, ch, clog)

	// only a token fetch that actually called the provider is worth logging
	clog.End()
	if len(clog.HttpLogs) > 0 && handler.StoreChannelLogs() {
		models.WriteChannelLog(rt, clog)
	}

	if err != nil {
		return nil, fmt.Errorf("error getting access token for channel %s: %w", ch.UUID(), err)
	}

	return &fetchTokenResponse{Token: token}, nil
}
