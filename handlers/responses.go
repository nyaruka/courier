package handlers

import (
	"context"
	"net/http"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
)

// WriteMsgsAndResponse writes the passed in message to our backend
func WriteMsgsAndResponse(ctx context.Context, h channels.Handler, msgs []*models.MsgIn, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
	events := make([]channels.Event, len(msgs))
	for i, m := range msgs {
		err := models.WriteMsg(ctx, h.Runtime(), m, clog)
		if err != nil {
			return nil, err
		}
		events[i] = m
	}

	return events, h.WriteMsgSuccessResponse(ctx, w, msgs)
}

// WriteMsgStatusAndResponse write the passed in status to our backend
func WriteMsgStatusAndResponse(ctx context.Context, h channels.Handler, channel *models.Channel, status *models.StatusUpdate, w http.ResponseWriter, r *http.Request) ([]channels.Event, error) {
	err := models.WriteStatusUpdate(ctx, h.Runtime(), status)
	if err != nil {
		return nil, err
	}

	return []channels.Event{status}, h.WriteStatusSuccessResponse(ctx, w, []*models.StatusUpdate{status})
}

// WriteAndLogRequestError logs the passed in error and writes the response to the response writer
func WriteAndLogRequestError(ctx context.Context, h channels.Handler, channel *models.Channel, w http.ResponseWriter, r *http.Request, err error) error {
	channels.LogRequestError(r, channel, err)
	return h.WriteRequestError(ctx, w, err)
}

// WriteAndLogRequestIgnored logs that the passed in request was ignored and writes the response to the response writer
func WriteAndLogRequestIgnored(ctx context.Context, h channels.Handler, channel *models.Channel, w http.ResponseWriter, r *http.Request, details string) error {
	channels.LogRequestIgnored(r, channel, details)
	return h.WriteRequestIgnored(ctx, w, details)
}
