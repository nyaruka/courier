package channels

import (
	"context"
	"fmt"
	"net/http"

	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/runtime"
)

// Incoming is the ordered set of things one incoming request contained - the messages, status updates and
// channel events a handler parsed out of a provider's payload, plus notes about the parts of it that were
// understood but not acted on. Handlers accumulate one of these while parsing and pass it to WriteIncoming.
//
// Collecting the whole request before writing any of it is what makes a mixed batch describable. A single
// provider webhook can carry messages, statuses and events together, and handlers that receive those have
// had to write each part as they parsed it and hand-roll their own response, which leaves no single place
// to apply anything that spans the request.
type Incoming struct {
	channel *models.Channel
	items   []incomingItem
}

// one thing an incoming request contained: an event to write, a message the provider has deleted, or a note
// that we ignored part of the request - which isn't written anywhere but is still described in the response
type incomingItem struct {
	event   Event
	deleted *deletedMsg
	ignored string
}

// a message we'd previously received which the provider has since deleted
type deletedMsg struct {
	externalID string
}

// NewIncoming creates an empty set of incoming items for a handler to add to. Everything one request
// contained is for the same channel - the one the server resolved before the handler was called.
func NewIncoming(ch *models.Channel) *Incoming { return &Incoming{channel: ch} }

// Msg adds a message parsed from the request
func (i *Incoming) Msg(m *models.MsgIn) { i.items = append(i.items, incomingItem{event: m}) }

// Status adds a message status update parsed from the request
func (i *Incoming) Status(s *models.StatusUpdate) { i.items = append(i.items, incomingItem{event: s}) }

// Event adds a channel event parsed from the request
func (i *Incoming) Event(e *models.ChannelEvent) { i.items = append(i.items, incomingItem{event: e}) }

// DeletedMsg notes that a message we'd received has since been deleted at the provider, which some of them
// report on the same webhook that delivered it. It's part of the batch rather than something the handler acts
// on as it parses, so that it happens in order with the rest of the request and can report a failure.
func (i *Incoming) DeletedMsg(externalID string) {
	i.items = append(i.items, incomingItem{deleted: &deletedMsg{externalID: externalID}})
}

// Ignored notes a part of the request we understood but didn't act on, such as a message echoed back to us.
// Nothing is written for it, but it appears in the response so that what we saw is visible to the provider.
func (i *Incoming) Ignored(details string) { i.items = append(i.items, incomingItem{ignored: details}) }

// Len returns how many items have been added
func (i *Incoming) Len() int { return len(i.items) }

// Channel returns the channel the request this describes was for
func (i *Incoming) Channel() *models.Channel { return i.channel }

// what the response says for a message the provider deleted
const msgDeletedInfo = "msg deleted"

// Outcome is what became of a single item of an incoming request
type Outcome string

const (
	// OutcomeWritten means we accepted the item - it was written to the database, or handed to the status
	// writer, or spooled because the database was unavailable. Those are the same promise to the provider:
	// we have it, and they don't need to send it again.
	OutcomeWritten Outcome = "written"

	// OutcomeDuplicate means the message had already been received, so it wasn't written a second time
	OutcomeDuplicate Outcome = "duplicate"

	// OutcomeIgnored means there was nothing to write
	OutcomeIgnored Outcome = "ignored"

	// OutcomeFailed means writing the item was attempted and failed
	OutcomeFailed Outcome = "failed"
)

// IncomingResult is what became of one item of an incoming request
type IncomingResult struct {
	Event   Event  // what was written, nil for an ignored item
	Details string // why it was ignored, for an ignored item
	Outcome Outcome
	Err     error // set when Outcome is OutcomeFailed
}

// WriteIncoming writes everything in the given set of incoming items, in the order the handler added them,
// and returns what became of each. Order is preserved because messages and events are queued to mailroom as
// they're written, and mailroom handles a contact's tasks in the order they were queued.
//
// It stops at the first item that fails, so the results describe the prefix it got through - which is what
// lets a caller report what was actually accepted rather than discarding a half-written batch.
func WriteIncoming(ctx context.Context, rt *runtime.Runtime, in *Incoming, clog *models.ChannelLog) ([]IncomingResult, error) {
	results := make([]IncomingResult, 0, len(in.items))

	for _, item := range in.items {
		if item.deleted != nil {
			if err := models.DeleteMsgByExternalID(ctx, rt, in.channel, item.deleted.externalID); err != nil {
				results = append(results, IncomingResult{Details: msgDeletedInfo, Outcome: OutcomeFailed, Err: err})
				return results, err
			}
			results = append(results, IncomingResult{Details: msgDeletedInfo, Outcome: OutcomeWritten})
			continue
		}
		if item.event == nil {
			results = append(results, IncomingResult{Details: item.ignored, Outcome: OutcomeIgnored})
			continue
		}

		var err error
		outcome := OutcomeWritten

		switch e := item.event.(type) {
		case *models.MsgIn:
			if err = models.WriteMsg(ctx, rt, e, clog); err == nil && e.Duplicate_ {
				outcome = OutcomeDuplicate
			}
		case *models.StatusUpdate:
			err = models.WriteStatusUpdate(ctx, rt, e)
		case *models.ChannelEvent:
			err = models.WriteChannelEvent(ctx, rt, e, clog)
		default:
			err = fmt.Errorf("unknown incoming item type %T", e)
		}

		if err != nil {
			results = append(results, IncomingResult{Event: item.event, Outcome: OutcomeFailed, Err: err})
			return results, err
		}

		results = append(results, IncomingResult{Event: item.event, Outcome: outcome})
	}

	return results, nil
}

// IncomingEvents returns the events from the given results that we accepted, which is what the server records
// stats for. Duplicate messages are included because they're still something we received and recognized - the
// server counts them separately by looking at the message itself.
func IncomingEvents(results []IncomingResult) []Event {
	events := make([]Event, 0, len(results))

	for _, r := range results {
		if r.Event != nil && r.Outcome != OutcomeFailed {
			events = append(events, r.Event)
		}
	}

	return events
}

// WriteIncomingResponse writes the standard JSON response describing each part of what an incoming request
// contained. Handlers whose provider requires a particular response body write their own instead.
func WriteIncomingResponse(w http.ResponseWriter, results []IncomingResult) error {
	data := make([]any, 0, len(results))

	for _, r := range results {
		switch e := r.Event.(type) {
		case *models.MsgIn:
			data = append(data, NewMsgReceiveData(e))
		case *models.StatusUpdate:
			data = append(data, NewStatusData(e))
		case *models.ChannelEvent:
			data = append(data, NewEventReceiveData(e))
		case nil:
			data = append(data, NewInfoData(r.Details))
		}
	}

	return WriteDataResponse(w, http.StatusOK, "Events Handled", data)
}
