// Package channels is what a channel handler is written against: the handler contract itself, the routes a
// handler registers, the responses and errors it can write, and - in Incoming and WriteIncoming - the one
// place that writes what a handler parsed out of a request.
//
// That last part is the convention worth knowing. Incoming events are written here, never by a handler: a
// handler turns a provider's payload into an Incoming and hands it over. A handler that writes for itself
// opts out of everything WriteIncoming does - writing a request's events in order, and reporting what became
// of each one so that the response and our stats describe what actually happened rather than what was
// attempted - and it opts out silently, which is what makes it worth stating.
//
// Most handlers don't hand anything over explicitly, because handlers.Receive does it for them: a receive
// function is given an Incoming, fills it in from the request, and returns. Writing it, answering the request
// and logging what the request was are all the seam's. The routes that stay on the older HandleFunc are the
// ones whose provider dictates a response body the standard ones can't express - a verification handshake
// echoing a challenge back, Viber's welcome message, a chat widget's CORS preflight.
//
// What a request is being handled as travels on the Incoming rather than on the channel log. It starts as
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
