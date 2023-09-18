package handlers

import (
	"context"
	"net/http"

	"github.com/nyaruka/courier"
)

// WriteMsgsAndResponse writes the passed in message to our backend
func WriteMsgsAndResponse(ctx context.Context, h courier.ChannelHandler, msgs []courier.MsgIn, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	events := make([]courier.Event, len(msgs))
	for i, m := range msgs {
		err := h.Server().Backend().WriteMsg(ctx, m, clog)
		if err != nil {
			return nil, err
		}
		events[i] = m
	}

	return events, h.WriteMsgSuccessResponse(ctx, w, msgs)
}

// WriteMsgStatusAndResponse write the passed in status to our backend
func WriteMsgStatusAndResponse(ctx context.Context, h courier.ChannelHandler, channel courier.Channel, status courier.StatusUpdate, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	err := h.Server().Backend().WriteStatusUpdate(ctx, status)
	if err != nil {
		return nil, err
	}

	return []courier.Event{status}, h.WriteStatusSuccessResponse(ctx, w, []courier.StatusUpdate{status})
}

// WriteAndLogRequestError logs the passed in error and writes the response to the response writer
func WriteAndLogRequestError(ctx context.Context, h courier.ChannelHandler, channel courier.Channel, w http.ResponseWriter, r *http.Request, err error) error {
	courier.LogRequestError(r, channel, err)
	return h.WriteRequestError(ctx, w, err)
}

// WriteAndLogRequestIgnored logs that the passed in request was ignored and writes the response to the response writer
func WriteAndLogRequestIgnored(ctx context.Context, h courier.ChannelHandler, channel courier.Channel, w http.ResponseWriter, r *http.Request, details string) error {
	courier.LogRequestIgnored(r, channel, details)
	return h.WriteRequestIgnored(ctx, w, details)
}
