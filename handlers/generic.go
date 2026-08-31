package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/gocommon/urns"
)

// NewTelReceiveHandler creates a new receive function given the passed in text and from fields
func NewTelReceiveHandler(fromField string, bodyField string) channels.ReceiveFunc {
	return func(ctx context.Context, c *models.Channel, r *http.Request, in *channels.Incoming, clog *models.ChannelLog) error {
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
	return func(ctx context.Context, c *models.Channel, r *http.Request, in *channels.Incoming, clog *models.ChannelLog) error {
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
	}
}

// JSONReceiveFunc is a receive function for a provider that sends JSON, which is decoded and validated for it
type JSONReceiveFunc[T any] func(context.Context, *models.Channel, *http.Request, *T, *channels.Incoming, *models.ChannelLog) error

// JSONPayload adapts a JSONReceiveFunc into a plain receive function
func JSONPayload[T any](fn JSONReceiveFunc[T]) channels.ReceiveFunc {
	return func(ctx context.Context, c *models.Channel, r *http.Request, in *channels.Incoming, clog *models.ChannelLog) error {
		payload := new(T)
		if err := DecodeAndValidateJSON(payload, r); err != nil {
			return err
		}
		return fn(ctx, c, r, payload, in, clog)
	}
}
