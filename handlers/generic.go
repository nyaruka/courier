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
	return func(ctx context.Context, c *models.Channel, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
		err := r.ParseForm()
		if err != nil {
			return nil, WriteAndLogRequestError(ctx, h, c, w, r, err)
		}

		body := r.Form.Get(bodyField)
		from := r.Form.Get(fromField)
		if from == "" {
			return nil, WriteAndLogRequestError(ctx, h, c, w, r, fmt.Errorf("missing required field '%s'", fromField))
		}
		// create our URN
		urn, err := urns.ParsePhone(from, c.Country(), true, false)
		if err != nil {
			return nil, WriteAndLogRequestError(ctx, h, c, w, r, err)
		}
		// build our msg
		msg := models.NewIncomingMsg(c, urn, body, "", clog).WithReceivedOn(time.Now().UTC())
		in := channels.NewIncoming(c)
		in.Msg(msg)
		return WriteIncomingAndResponse(ctx, h, in, w, r, clog)
	}
}

// NewExternalIDStatusHandler creates a new status handler given the passed in status map and fields
func NewExternalIDStatusHandler(h channels.Handler, statuses map[string]models.MsgStatus, externalIDField string, statusField string) channels.HandleFunc {
	return func(ctx context.Context, c *models.Channel, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
		err := r.ParseForm()
		if err != nil {
			return nil, WriteAndLogRequestError(ctx, h, c, w, r, err)
		}

		externalID := r.Form.Get(externalIDField)
		if externalID == "" {
			return nil, WriteAndLogRequestError(ctx, h, c, w, r, fmt.Errorf("missing required field '%s'", externalIDField))
		}

		s := r.Form.Get(statusField)
		sValue, found := statuses[s]
		if !found {
			return nil, WriteAndLogRequestError(ctx, h, c, w, r, fmt.Errorf("unknown status value '%s'", s))
		}

		// create our status
		status := models.NewStatusUpdateByExternalID(c, externalID, sValue, clog)
		in := channels.NewIncoming(c)
		in.Status(status)
		return WriteIncomingAndResponse(ctx, h, in, w, r, clog)
	}
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
// Returning an error answers the request as an error. Returning channels.Ignore answers it as ignored. A
// handler that parses its way to nothing just returns, and the empty batch says the same thing.
type ReceiveFunc func(context.Context, *models.Channel, *http.Request, *channels.Incoming, *models.ChannelLog) error

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
			var ignored *channels.IgnoredRequest
			if errors.As(err, &ignored) {
				return nil, WriteAndLogRequestIgnored(ctx, h, c, w, r, ignored.Details)
			}
			return nil, WriteAndLogRequestError(ctx, h, c, w, r, err)
		}

		return WriteIncomingAndResponse(ctx, h, in, w, r, clog)
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
