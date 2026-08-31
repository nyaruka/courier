package channels

import (
	"context"
	"errors"
	"net/http"

	"github.com/nyaruka/courier/v26/core/models"
)

// ReceiveFunc is how a handler receives an incoming request: it parses what the request contained into the
// given batch and returns. Writing that batch, answering the request and logging it are all the server's,
// which is what keeps every handler's incoming path the same shape.
//
// Returning an error answers the request as an error. Returning Ignore answers it as ignored, Unauthenticated
// as unauthorized, and Reply with the exact body the provider expects - which is what lets a route with one
// verification branch keep the rest of itself on this seam. A handler that parses its way to nothing just
// returns, and the empty batch says the same thing.
type ReceiveFunc func(context.Context, *models.Channel, *http.Request, *Received, *models.ChannelLog) error

// AddReceive adds a route served by a receive function. Routes that need to own their whole response - a
// verification handshake, a CORS preflight - use Add with a HandleFunc instead.
//
// The kind is what the route serves, and is where the batch starts out - so a route serving one purpose names
// it, and a route serving several registers KindAny and narrows it with Received.As once it knows which it's
// dealing with. KindAny isn't a placeholder: it's what a request that never got as far as being classified -
// one that failed to parse, or named an event we don't handle - is left as.
func (r *Routes) AddReceive(handler Handler, method string, action string, kind Kind, fn ReceiveFunc) {
	r.Add(handler, method, action, kind.LogType(), Receive(handler, kind, fn))
}

// Receive adapts a ReceiveFunc into a HandleFunc, for a route serving the given kind. AddReceive is how routes
// normally get this; it's exported for the rare route that wraps its handling in something else, like the chat
// widget's CORS headers.
//
// The lifecycle is two steps: write what the receive function parsed, then answer the request. They're
// separate because they always both happen - even a request that ends in a parse error keeps the good
// messages ahead of the failure, since a provider that batches several messages into one request won't send
// them again just because a later one was malformed.
func Receive(h Handler, kind Kind, fn ReceiveFunc) HandleFunc {
	return func(ctx context.Context, c *models.Channel, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]Event, error) {
		// the batch starts out as whatever the route serves, which the handler can change once it knows what
		// it's actually dealing with
		in := NewReceived(c).As(kind)

		ferr := fn(ctx, c, r, in, clog)

		// whatever the handler worked out it was dealing with is what this gets logged as, however it turned
		// out - a request we understood and then ignored is still a request of that kind
		clog.Type = in.Kind().LogType()

		var results []WriteResult
		if in.Len() > 0 {
			var err error
			if results, err = WriteReceived(ctx, h.Runtime(), in, clog); err != nil {
				// a write failure is courier's problem rather than the request's, so it goes back to the
				// server to log and answer as an error - reporting whatever was written before the failure
				// so it isn't lost from our logging and stats
				return AcceptedEvents(results), err
			}
		}

		return AcceptedEvents(results), respond(ctx, h, w, r, in, results, ferr)
	}
}

// respond answers the request, now that everything it contained has been written. A failure in the request
// itself is answered here and logged at request level - it's the provider's problem, which is what separates
// it from a write failure, which Receive hands back to the server as courier's own.
func respond(ctx context.Context, h Handler, w http.ResponseWriter, r *http.Request, in *Received, results []WriteResult, ferr error) error {
	c := in.Channel()

	if ferr != nil {
		var reply *RequestReply
		if errors.As(ferr, &reply) {
			return reply.Write(w)
		}
		var ignored *IgnoredRequest
		if errors.As(ferr, &ignored) {
			LogRequestIgnored(r, c, ignored.Details)
			return h.RespondIgnored(ctx, w, ignored.Details)
		}
		var unauthenticated *UnauthenticatedRequest
		if errors.As(ferr, &unauthenticated) {
			LogRequestError(r, c, unauthenticated.Err)
			return RespondData(w, http.StatusUnauthorized, "Unauthorized", []any{NewErrorData(unauthenticated.Err.Error())})
		}
		LogRequestError(r, c, ferr)
		return h.RespondError(ctx, w, ferr)
	}

	// a request we found nothing in is one we ignored, rather than one we handled emptily - which saves
	// every handler that can parse its way to nothing from checking for it
	if in.Len() == 0 {
		LogRequestIgnored(r, c, "ignoring request, nothing to handle")
		return h.RespondIgnored(ctx, w, "ignoring request, nothing to handle")
	}

	// which response a written batch gets comes from what the request is being handled as: a receive answers
	// as a receive, a status callback as a status. The shape a provider expects belongs to the endpoint it
	// called rather than to whatever the request happened to carry, which is why this reads the batch's
	// declared kind instead of inspecting its contents - a status callback that also stopped a contact still
	// answers as a status callback.
	events := AcceptedEvents(results)

	switch in.Kind() {
	case KindMsg:
		msgs := make([]*models.MsgIn, 0, len(events))
		for _, e := range events {
			if m, ok := e.(*models.MsgIn); ok {
				msgs = append(msgs, m)
			}
		}
		return h.RespondMsgs(ctx, w, msgs)

	case KindStatus:
		statuses := make([]*models.StatusUpdate, 0, len(events))
		for _, e := range events {
			if s, ok := e.(*models.StatusUpdate); ok {
				statuses = append(statuses, s)
			}
		}
		return h.RespondStatuses(ctx, w, statuses)

	case KindEvent:
		chEvents := make([]*models.ChannelEvent, 0, len(events))
		for _, e := range events {
			if ce, ok := e.(*models.ChannelEvent); ok {
				chEvents = append(chEvents, ce)
			}
		}
		return RespondEvents(w, chEvents)

	default:
		return RespondReceived(w, results)
	}
}
