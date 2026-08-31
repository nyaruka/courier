package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/gocommon/urns"
)

// NewTelReceiveHandler creates a new receive handler given the passed in text and from fields
func NewTelReceiveHandler(h channels.Handler, fromField string, bodyField string) channels.HandleFunc {
	return Receive(h, func(ctx context.Context, c *models.Channel, r *http.Request, in *channels.Incoming, clog *models.ChannelLog) error {
		if err := r.ParseForm(); err != nil {
			return err
		}

		body := r.Form.Get(bodyField)
		from := r.Form.Get(fromField)
		if from == "" {
			return fmt.Errorf("missing required field '%s'", fromField)
		}

		urn, err := urns.ParsePhone(from, c.Country(), true, false)
		if err != nil {
			return err
		}

		in.Msg(models.NewIncomingMsg(c, urn, body, "", clog).WithReceivedOn(time.Now().UTC()))
		return nil
	})
}

// NewExternalIDStatusHandler creates a new status handler given the passed in status map and fields
func NewExternalIDStatusHandler(h channels.Handler, statuses map[string]models.MsgStatus, externalIDField string, statusField string) channels.HandleFunc {
	return Receive(h, func(ctx context.Context, c *models.Channel, r *http.Request, in *channels.Incoming, clog *models.ChannelLog) error {
		if err := r.ParseForm(); err != nil {
			return err
		}

		externalID := r.Form.Get(externalIDField)
		if externalID == "" {
			return fmt.Errorf("missing required field '%s'", externalIDField)
		}

		s := r.Form.Get(statusField)
		sValue, found := statuses[s]
		if !found {
			return fmt.Errorf("unknown status value '%s'", s)
		}

		in.Status(models.NewStatusUpdateByExternalID(c, externalID, sValue, clog))
		return nil
	})
}

type JSONHandlerFunc[T any] func(context.Context, *models.Channel, http.ResponseWriter, *http.Request, *T, *models.ChannelLog) ([]channels.Event, error)

func JSONPayload[T any](h channels.Handler, handlerFunc JSONHandlerFunc[T]) channels.HandleFunc {
	return func(ctx context.Context, c *models.Channel, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
		payload := new(T)

		err := DecodeAndValidateJSON(payload, r)
		if err != nil {
			return nil, WriteAndLogRequestError(ctx, h, c, w, r, err)
		}

		return handlerFunc(ctx, c, w, r, payload, clog)
	}
}

// ReceiveFunc is how a handler receives an incoming request: it parses what the request contained into the
// given batch and returns. Writing that batch, answering the request and logging it are all the server's,
// which is what keeps every handler's incoming path the same shape.
//
// Returning an error answers the request as an error. Returning channels.Ignore answers it as ignored,
// channels.Unauthenticated as unauthorized, and channels.Reply with the exact body the provider expects -
// which is what lets a route with one verification branch keep the rest of itself on this seam. A handler
// that parses its way to nothing just returns, and the empty batch says the same thing.
type ReceiveFunc func(context.Context, *models.Channel, *http.Request, *channels.Incoming, *models.ChannelLog) error

// writes a batch without answering for it, for the paths that answer some other way
func writeIncoming(ctx context.Context, h channels.Handler, in *channels.Incoming, clog *models.ChannelLog) ([]channels.Event, error) {
	if in.Len() == 0 {
		return nil, nil
	}
	results, err := channels.WriteIncoming(ctx, h.Runtime(), in, clog)
	return channels.IncomingEvents(results), err
}

// Receive adapts a ReceiveFunc into a route the server can serve
func Receive(h channels.Handler, fn ReceiveFunc) channels.HandleFunc {
	return func(ctx context.Context, c *models.Channel, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
		// the batch starts out as whatever the route was registered as, which the handler can change once it
		// knows what it's actually dealing with
		in := channels.NewIncoming(c).As(clog.Type)

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

			var reply *channels.RequestReply
			if errors.As(err, &reply) {
				return events, reply.Write(w)
			}
			var ignored *channels.IgnoredRequest
			if errors.As(err, &ignored) {
				return events, WriteAndLogRequestIgnored(ctx, h, c, w, r, ignored.Details)
			}
			var unauthenticated *channels.UnauthenticatedRequest
			if errors.As(err, &unauthenticated) {
				return events, channels.WriteAndLogUnauthorized(w, r, c, unauthenticated.Err)
			}
			return events, WriteAndLogRequestError(ctx, h, c, w, r, err)
		}

		return writeIncomingAndResponse(ctx, h, in, w, r, clog)
	}
}

// JSONReceiveFunc is a ReceiveFunc for a provider that sends JSON, which is decoded and validated for it
type JSONReceiveFunc[T any] func(context.Context, *models.Channel, *http.Request, *T, *channels.Incoming, *models.ChannelLog) error

// ReceiveJSON adapts a JSONReceiveFunc into a route the server can serve
func ReceiveJSON[T any](h channels.Handler, fn JSONReceiveFunc[T]) channels.HandleFunc {
	return Receive(h, func(ctx context.Context, c *models.Channel, r *http.Request, in *channels.Incoming, clog *models.ChannelLog) error {
		payload := new(T)
		if err := DecodeAndValidateJSON(payload, r); err != nil {
			return err
		}
		return fn(ctx, c, r, payload, in, clog)
	})
}
