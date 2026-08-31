package channels

import (
	"context"
	"fmt"
	"net/http"

	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/gocommon/svclogs"
)

// Kind is what an incoming request is being handled as. A route declares one when it registers, and a route
// serving several declares KindAny and narrows it with Received.As once it knows which it's dealing with.
//
// It answers two questions that used to be one field between them. Which response the provider gets comes from
// here, because the shape a provider expects belongs to the endpoint it called - which is why handlers whose
// provider demands a particular body override RespondMsgs or RespondStatuses and rely on this to pick between
// them. What the request is logged as also comes from here, via LogType. Keeping both derived from a single
// declaration is what stops them disagreeing; keeping them separate derivations is what lets the log stay
// coarse while the response stays exact.
type Kind string

const (
	KindMsg    Kind = "msg"    // a message from a contact
	KindStatus Kind = "status" // a provider reporting on a message we sent
	KindEvent  Kind = "event"  // something a contact did that isn't a message
	KindVerify Kind = "verify" // a provider checking the endpoint is ours
	KindAny    Kind = "any"    // a route that serves several of the above, before it knows which
)

// LogType is what a request of this kind is logged as. Everything about a contact shares one type - what it
// was is already visible in the log's request and response - while a status callback keeps its own, and a
// verification handshake keeps its own.
func (k Kind) LogType() svclogs.Type {
	switch k {
	case KindStatus:
		return models.ChannelLogTypeMsgStatus
	case KindVerify:
		return models.ChannelLogTypeWebhookVerify
	default:
		return models.ChannelLogTypeReceive
	}
}

// Received is the ordered set of things one incoming request contained - the messages, status updates and
// channel events a handler parsed out of a provider's payload, plus notes about the parts of it that were
// understood but not acted on. A receive function fills one in while parsing, and the seam writes it.
//
// Collecting the whole request before writing any of it is what makes a mixed batch describable. A single
// provider webhook can carry messages, statuses and events together, and handlers that receive those have
// had to write each part as they parsed it and hand-roll their own response, which leaves no single place
// to apply anything that spans the request.
type Received struct {
	channel *models.Channel
	kind    Kind
	items   []receivedItem
}

// one thing an incoming request contained: an event to write, a message the provider has deleted, or a note
// that we ignored part of the request - which isn't written anywhere but is still described in the response
type receivedItem struct {
	event   Event
	deleted *deletedMsg
	ignored string
}

// a message we'd previously received which the provider has since deleted
type deletedMsg struct {
	externalID string
}

// NewReceived creates an empty set of incoming items for a handler to add to. Everything one request
// contained is for the same channel - the one the server resolved before the handler was called.
func NewReceived(ch *models.Channel) *Received { return &Received{channel: ch} }

// As declares what this request is being handled as, which decides how it's answered and how it's logged.
//
// It starts as whatever the route was registered as, which is right for the routes that serve one purpose. A
// route that serves more than one - a provider that delivers messages and statuses through a single URL, or a
// receive route where a particular message means a contact started a conversation - says so with this, at the
// point it works out which it's dealing with.
func (i *Received) As(kind Kind) *Received { i.kind = kind; return i }

// Kind returns what this request is being handled as
func (i *Received) Kind() Kind { return i.kind }

// Msg adds a message parsed from the request
func (i *Received) Msg(m *models.MsgIn) { i.items = append(i.items, receivedItem{event: m}) }

// Status adds a message status update parsed from the request
func (i *Received) Status(s *models.StatusUpdate) { i.items = append(i.items, receivedItem{event: s}) }

// Event adds a channel event parsed from the request
func (i *Received) Event(e *models.ChannelEvent) { i.items = append(i.items, receivedItem{event: e}) }

// DeletedMsg notes that a message we'd received has since been deleted at the provider, which some of them
// report on the same webhook that delivered it. It's part of the batch rather than something the handler acts
// on as it parses, so that it happens in order with the rest of the request and can report a failure.
func (i *Received) DeletedMsg(externalID string) {
	i.items = append(i.items, receivedItem{deleted: &deletedMsg{externalID: externalID}})
}

// Ignored notes a part of the request we understood but didn't act on, such as a message echoed back to us.
// Nothing is written for it, but it appears in the response so that what we saw is visible to the provider.
func (i *Received) Ignored(details string) { i.items = append(i.items, receivedItem{ignored: details}) }

// Len returns how many items have been added
func (i *Received) Len() int { return len(i.items) }

// Channel returns the channel the request this describes was for
func (i *Received) Channel() *models.Channel { return i.channel }

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

// WriteResult is what became of one item of an incoming request
type WriteResult struct {
	Event   Event  // what was written, nil for an ignored item
	Details string // why it was ignored, for an ignored item
	Outcome Outcome
	Err     error // set when Outcome is OutcomeFailed
}

// WriteReceived writes everything in the given set of incoming items, in the order the handler added them,
// and returns what became of each. Order is preserved because messages and events are queued to mailroom as
// they're written, and mailroom handles a contact's tasks in the order they were queued.
//
// It stops at the first item that fails, so the results describe the prefix it got through - which is what
// lets a caller report what was actually accepted rather than discarding a half-written batch.
func WriteReceived(ctx context.Context, rt *runtime.Runtime, in *Received, clog *models.ChannelLog) ([]WriteResult, error) {
	results := make([]WriteResult, 0, len(in.items))

	for _, item := range in.items {
		if item.deleted != nil {
			if err := models.DeleteMsgByExternalID(ctx, rt, in.channel, item.deleted.externalID); err != nil {
				results = append(results, WriteResult{Details: msgDeletedInfo, Outcome: OutcomeFailed, Err: err})
				return results, err
			}
			results = append(results, WriteResult{Details: msgDeletedInfo, Outcome: OutcomeWritten})
			continue
		}
		if item.event == nil {
			results = append(results, WriteResult{Details: item.ignored, Outcome: OutcomeIgnored})
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
			results = append(results, WriteResult{Event: item.event, Outcome: OutcomeFailed, Err: err})
			return results, err
		}

		results = append(results, WriteResult{Event: item.event, Outcome: outcome})
	}

	return results, nil
}

// AcceptedEvents returns the events from the given results that we accepted, which is what the server records
// stats for. Duplicate messages are included because they're still something we received and recognized - the
// server counts them separately by looking at the message itself.
func AcceptedEvents(results []WriteResult) []Event {
	events := make([]Event, 0, len(results))

	for _, r := range results {
		if r.Event != nil && r.Outcome != OutcomeFailed {
			events = append(events, r.Event)
		}
	}

	return events
}

// RespondReceived writes the standard JSON response describing each part of what an incoming request
// contained. Handlers whose provider requires a particular response body write their own instead.
func RespondReceived(w http.ResponseWriter, results []WriteResult) error {
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

	return RespondData(w, http.StatusOK, "Events Handled", data)
}
