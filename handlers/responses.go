package handlers

import (
	"context"
	"net/http"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
)

// WriteIncomingAndResponse writes everything a request contained and then answers it.
//
// Which response that is comes from the log type its route was registered with: a receive route answers as a
// receive, a status callback as a status. The shape a provider expects belongs to the endpoint it called
// rather than to whatever the request happened to carry, which is why this reads the route's intent instead of
// inspecting the batch - a status callback that also stopped a contact still answers as a status callback.
func WriteIncomingAndResponse(ctx context.Context, h channels.Handler, in *channels.Incoming, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
	// a request we found nothing in is one we ignored, rather than one we handled emptily - which saves every
	// handler that can parse its way to nothing from checking for it
	if in.Len() == 0 {
		return nil, WriteAndLogRequestIgnored(ctx, h, in.Channel(), w, r, "ignoring request, nothing to handle")
	}

	results, err := channels.WriteIncoming(ctx, h.Runtime(), in, clog)
	if err != nil {
		// whatever was written before the failure still happened, so report it rather than losing it from our
		// logging and stats
		return channels.IncomingEvents(results), err
	}

	events := channels.IncomingEvents(results)

	switch clog.Type {
	case models.ChannelLogTypeMsgReceive:
		msgs := make([]*models.MsgIn, 0, len(events))
		for _, e := range events {
			if m, ok := e.(*models.MsgIn); ok {
				msgs = append(msgs, m)
			}
		}
		return events, h.WriteMsgSuccessResponse(ctx, w, msgs)

	case models.ChannelLogTypeMsgStatus:
		statuses := make([]*models.StatusUpdate, 0, len(events))
		for _, e := range events {
			if s, ok := e.(*models.StatusUpdate); ok {
				statuses = append(statuses, s)
			}
		}
		return events, h.WriteStatusSuccessResponse(ctx, w, statuses)

	case models.ChannelLogTypeEventReceive:
		chEvents := make([]*models.ChannelEvent, 0, len(events))
		for _, e := range events {
			if ce, ok := e.(*models.ChannelEvent); ok {
				chEvents = append(chEvents, ce)
			}
		}
		return events, channels.WriteChannelEventsSuccess(w, chEvents)

	default:
		return events, channels.WriteIncomingResponse(w, results)
	}
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
