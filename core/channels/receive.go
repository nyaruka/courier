package channels

import (
	"context"
	"errors"
	"net/http"

	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/gocommon/svclogs"
)

// ReceiveFunc is how a handler receives an incoming request: it parses what the request contained into the
// given batch and returns. Writing that batch, answering the request and logging it are all the server's,
// which is what keeps every handler's incoming path the same shape.
//
// Returning an error answers the request as an error. Returning Ignore answers it as ignored, Unauthenticated
// as unauthorized, and Reply with the exact body the provider expects - which is what lets a route with one
// verification branch keep the rest of itself on this seam. A handler that parses its way to nothing just
// returns, and the empty batch says the same thing.
type ReceiveFunc func(context.Context, *models.Channel, *http.Request, *Incoming, *models.ChannelLog) error

// AddReceive adds a route served by a receive function. Routes that need to own their whole response - a
// verification handshake, a CORS preflight - use Add with a HandleFunc instead.
func (r *Routes) AddReceive(handler Handler, method string, action string, logType svclogs.Type, fn ReceiveFunc) {
	r.Add(handler, method, action, logType, Receive(handler, fn))
}

// Receive adapts a ReceiveFunc into a HandleFunc. AddReceive is how routes normally get this; it's exported
// for the rare route that wraps its handling in something else, like the chat widget's CORS headers.
func Receive(h Handler, fn ReceiveFunc) HandleFunc {
	return func(ctx context.Context, c *models.Channel, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]Event, error) {
		// the batch starts out as whatever the route was registered as, which the handler can change once it
		// knows what it's actually dealing with
		in := NewIncoming(c).As(clog.Type)

		err := fn(ctx, c, r, in, clog)

		// whatever the handler worked out it was dealing with is what this gets logged as, however it turned
		// out - a request we understood and then ignored is still a request of that kind
		clog.Type = in.Kind()

		if err != nil {
			// whatever the handler parsed before it gave up is still ours to keep - a provider that batches
			// several messages into one request won't send the good ones again just because a later one was
			// malformed, so they're written before the failure is reported
			events, werr := writeIncoming(ctx, h, in, clog)
			if werr != nil {
				return events, werr
			}

			var reply *RequestReply
			if errors.As(err, &reply) {
				return events, reply.Write(w)
			}
			var ignored *IgnoredRequest
			if errors.As(err, &ignored) {
				return events, WriteAndLogRequestIgnored(ctx, h, w, r, c, ignored.Details)
			}
			var unauthenticated *UnauthenticatedRequest
			if errors.As(err, &unauthenticated) {
				return events, WriteAndLogUnauthorized(w, r, c, unauthenticated.Err)
			}
			return events, WriteAndLogRequestError(ctx, h, w, r, c, err)
		}

		return writeIncomingAndResponse(ctx, h, in, w, r, clog)
	}
}

// writes a batch without answering for it, for the paths that answer some other way
func writeIncoming(ctx context.Context, h Handler, in *Incoming, clog *models.ChannelLog) ([]Event, error) {
	if in.Len() == 0 {
		return nil, nil
	}
	results, err := WriteIncoming(ctx, h.Runtime(), in, clog)
	return IncomingEvents(results), err
}

// writeIncomingAndResponse writes everything a request contained and then answers it.
//
// Which response that is comes from what the request is being handled as: a receive answers as a receive, a
// status callback as a status. The shape a provider expects belongs to the endpoint it called rather than to
// whatever the request happened to carry, which is why this reads the batch's declared kind instead of
// inspecting its contents - a status callback that also stopped a contact still answers as a status callback.
//
// That declaration is also what the request is logged as, so the two can't drift apart.
func writeIncomingAndResponse(ctx context.Context, h Handler, in *Incoming, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]Event, error) {
	// a request we found nothing in is one we ignored, rather than one we handled emptily - which saves every
	// handler that can parse its way to nothing from checking for it
	if in.Len() == 0 {
		return nil, WriteAndLogRequestIgnored(ctx, h, w, r, in.Channel(), "ignoring request, nothing to handle")
	}

	results, err := WriteIncoming(ctx, h.Runtime(), in, clog)
	if err != nil {
		// whatever was written before the failure still happened, so report it rather than losing it from our
		// logging and stats
		return IncomingEvents(results), err
	}

	events := IncomingEvents(results)

	switch in.Kind() {
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
		return events, WriteChannelEventsSuccess(w, chEvents)

	default:
		return events, WriteIncomingResponse(w, results)
	}
}
