package handlers

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/gocommon/urns"
)

// UnknownStatusError creates the error for a status value that isn't in the given mapping, listing the values
// that are - from the mapping itself, so the message can't drift from the map.
func UnknownStatusError[K cmp.Ordered](statuses map[K]models.MsgStatus, value K) error {
	vals := make([]string, 0, len(statuses))
	for _, k := range slices.Sorted(maps.Keys(statuses)) {
		vals = append(vals, fmt.Sprintf("'%v'", k))
	}
	return fmt.Errorf("unknown status '%v', must be one of %s", value, strings.Join(vals, ", "))
}

// NewTelReceiveHandler creates a new receive function given the passed in text and from fields
func NewTelReceiveHandler(fromField string, bodyField string) channels.ReceiveFunc {
	return func(ctx context.Context, c *models.Channel, r *http.Request, in *channels.Received, clog *models.ChannelLog) error {
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
	}
}

// NewExternalIDStatusHandler creates a new status receive function given the passed in status map and fields
func NewExternalIDStatusHandler(statuses map[string]models.MsgStatus, externalIDField string, statusField string) channels.ReceiveFunc {
	return func(ctx context.Context, c *models.Channel, r *http.Request, in *channels.Received, clog *models.ChannelLog) error {
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
			return UnknownStatusError(statuses, s)
		}

		in.Status(models.NewStatusUpdateByExternalID(c, externalID, sValue, clog))
		return nil
	}
}

// PayloadReceiveFunc is a receive function for a provider whose request body is decoded and validated into a
// payload struct before the handler sees it. JSONPayload, FormPayload and XMLPayload each adapt one into a
// plain receive function, differing only in how they decode.
type PayloadReceiveFunc[T any] func(context.Context, *models.Channel, *http.Request, *T, *channels.Received, *models.ChannelLog) error

// JSONPayload adapts a receive function for a provider that sends JSON, decoding and validating it first
func JSONPayload[T any](fn PayloadReceiveFunc[T]) channels.ReceiveFunc {
	return withPayload(fn, DecodeAndValidateJSON)
}

// FormPayload adapts a receive function for a provider that sends form values, decoding and validating them
// first. Handlers that must check a signature over the raw body decode for themselves instead.
func FormPayload[T any](fn PayloadReceiveFunc[T]) channels.ReceiveFunc {
	return withPayload(fn, DecodeAndValidateForm)
}

// XMLPayload adapts a receive function for a provider that sends XML, decoding and validating it first
func XMLPayload[T any](fn PayloadReceiveFunc[T]) channels.ReceiveFunc {
	return withPayload(fn, DecodeAndValidateXML)
}

// withPayload is the shared shape of the three adapters above: decode into a new T, then hand it to the
// receive function - which never has to consider a request it couldn't be parsed from.
func withPayload[T any](fn PayloadReceiveFunc[T], decode func(any, *http.Request) error) channels.ReceiveFunc {
	return func(ctx context.Context, c *models.Channel, r *http.Request, in *channels.Received, clog *models.ChannelLog) error {
		payload := new(T)
		if err := decode(payload, r); err != nil {
			return err
		}
		return fn(ctx, c, r, payload, in, clog)
	}
}
