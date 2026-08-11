package handlers

import (
	"context"
	"net/http"

	"github.com/nyaruka/courier/v26"
	"github.com/nyaruka/courier/v26/core/models"
)

// WriteMsgsAndResponse writes the passed in message to our backend
func WriteMsgsAndResponse(ctx context.Context, h courier.ChannelHandler, msgs []*models.MsgIn, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]courier.Event, error) {
	events := make([]courier.Event, len(msgs))
	for i, m := range msgs {
		err := h.Backend().WriteMsg(ctx, m, clog)
		if err != nil {
			return nil, err
		}
		events[i] = m
	}

	return events, h.WriteMsgSuccessResponse(ctx, w, msgs)
}

// WriteMsgStatusAndResponse write the passed in status to our backend
func WriteMsgStatusAndResponse(ctx context.Context, h courier.ChannelHandler, channel *models.Channel, status *models.StatusUpdate, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	err := h.Backend().WriteStatusUpdate(ctx, status)
	if err != nil {
		return nil, err
	}

	return []courier.Event{status}, h.WriteStatusSuccessResponse(ctx, w, []*models.StatusUpdate{status})
}

// WriteAndLogRequestError logs the passed in error and writes the response to the response writer
func WriteAndLogRequestError(ctx context.Context, h courier.ChannelHandler, channel *models.Channel, w http.ResponseWriter, r *http.Request, err error) error {
	courier.LogRequestError(r, channel, err)
	return h.WriteRequestError(ctx, w, err)
}

// WriteAndLogRequestIgnored logs that the passed in request was ignored and writes the response to the response writer
func WriteAndLogRequestIgnored(ctx context.Context, h courier.ChannelHandler, channel *models.Channel, w http.ResponseWriter, r *http.Request, details string) error {
	courier.LogRequestIgnored(r, channel, details)
	return h.WriteRequestIgnored(ctx, w, details)
}
