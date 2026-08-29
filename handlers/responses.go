package handlers

import (
	"context"
	"net/http"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
)

// WriteMsgsAndResponse writes the passed in messages and responds with the handler's message success response
func WriteMsgsAndResponse(ctx context.Context, h channels.Handler, msgs []*models.MsgIn, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
	// a request with nothing in it has no channel to speak of, and nothing to write
	if len(msgs) == 0 {
		return nil, h.WriteMsgSuccessResponse(ctx, w, msgs)
	}

	// every message in a request is for the channel the server resolved before calling the handler
	in := channels.NewIncoming(msgs[0].Channel())
	for _, m := range msgs {
		in.Msg(m)
	}

	results, err := channels.WriteIncoming(ctx, h.Runtime(), in, clog)
	if err != nil {
		return nil, err
	}

	return channels.IncomingEvents(results), h.WriteMsgSuccessResponse(ctx, w, msgs)
}

// WriteMsgStatusAndResponse writes the passed in status and responds with the handler's status success response
func WriteMsgStatusAndResponse(ctx context.Context, h channels.Handler, channel *models.Channel, status *models.StatusUpdate, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
	in := channels.NewIncoming(channel)
	in.Status(status)

	results, err := channels.WriteIncoming(ctx, h.Runtime(), in, clog)
	if err != nil {
		return nil, err
	}

	return channels.IncomingEvents(results), h.WriteStatusSuccessResponse(ctx, w, []*models.StatusUpdate{status})
}

// WriteChannelEventAndResponse writes the passed in channel event and responds with the standard event success
// response
func WriteChannelEventAndResponse(ctx context.Context, h channels.Handler, event *models.ChannelEvent, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
	in := channels.NewIncoming(event.Channel())
	in.Event(event)

	results, err := channels.WriteIncoming(ctx, h.Runtime(), in, clog)
	if err != nil {
		return nil, err
	}

	return channels.IncomingEvents(results), channels.WriteChannelEventSuccess(w, event)
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
