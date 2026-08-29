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
// Everything else in core/models is a handler's to use directly. Models owns the data - the types, the
// constants and the database access - and handlers lean on it constantly, building a models.MsgIn, reading
// models.ConfigAuthToken, looking a channel up with models.GetChannel. The line is drawn around incoming
// events rather than around side effects in general: webchat creating a contact when a chat starts isn't an
// incoming event, and calls models.GetContact for itself.
package channels
