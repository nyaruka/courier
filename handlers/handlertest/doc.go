// Package handlertest runs the tests for channel handlers. Test cases live in JSON files under a handler's testdata
// directory, each file holding the cases for one channel configuration: a file of incoming cases is run with
// RunIncomingTests and a file of outgoing cases with RunOutgoingTests. A case is its inputs (the request or message,
// and any mocked HTTP responses) followed by what the handler did with them, which is written by running the tests
// with -update and checked against on every other run:
//
//	go test ./handlers/telegram/ -update
//
// Only requests answered by a case's mocks are captured, and any other request a handler makes fails, so a case
// has to mock every call its handler makes. Time, UUIDs and random values are derived from the case's label, so
// labels must be unique within a file.
package handlertest
