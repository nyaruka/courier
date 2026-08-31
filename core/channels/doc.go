// Package channels is what a channel handler is written against: the handler contract itself, the routes a
// handler registers, the responses and errors it can write, and - in Received and WriteReceived - the one
// place that writes what a handler parsed out of a request.
//
// That last part is the convention worth knowing. Received events are written here, never by a handler: a
// handler turns a provider's payload into a Received and hands it over. A handler that writes for itself
// opts out of everything WriteReceived does - writing a request's events in order, and reporting what became
// of each one so that the response and our stats describe what actually happened rather than what was
// attempted - and it opts out silently, which is what makes it worth stating.
//
// Most handlers don't hand anything over explicitly, because their routes are registered with AddReceive: a
// receive function is given a Received, fills it in from the request, and returns. Writing it, answering the
// request and logging what the request was are all the seam's. A branch whose provider dictates the response
// body - a verification challenge echoed back, Viber's welcome message - returns Reply with that body, and
// the batch is still written first. The routes registered with Add and a raw HandleFunc are the ones that
// aren't receiving anything at all: the GET verification handshakes, a chat widget's CORS preflight,
// Firebase's contact registration.
//
// What a request is being handled as travels on the Received rather than on the channel log. It starts as
// whatever the route was registered as, and a route serving more than one purpose says which with As(). That
// one declaration decides both how the request is answered and what it's logged as, so the two can't disagree
// - which they silently did, for as long as each was set separately.
//
// Everything else in core/models is a handler's to use directly. Models owns the data - the types, the
// constants and the database access - and handlers lean on it constantly, building a models.MsgIn, reading
// models.ConfigAuthToken, looking a channel up with models.GetChannel. The line is drawn around incoming
// events rather than around side effects in general: webchat creating a contact when a chat starts isn't an
// incoming event, and calls models.GetContact for itself.
package channels
