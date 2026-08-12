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
		return WriteMsgsAndResponse(ctx, h, []*models.MsgIn{msg}, w, r, clog)
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
		return WriteMsgStatusAndResponse(ctx, h, c, status, w, r)
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
