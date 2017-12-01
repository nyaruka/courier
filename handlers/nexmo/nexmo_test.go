package nexmo

import (
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "NX", "2020", "US", nil),
}

var (
	statusURL  = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status"
	receiveURL = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"

	receiveValidMessage = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive?to=2020&msisdn=2349067554729&text=Join&messageId=external1"

	statusDelivered  = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?to=2020&messageId=external1&status=delivered"
	statusExpired    = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?to=2020&messageId=external1&status=expired"
	statusFailed     = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?to=2020&messageId=external1&status=failed"
	statusAccepted   = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?to=2020&messageId=external1&status=accepted"
	statusBuffered   = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?to=2020&messageId=external1&status=buffered"
	statusUnexpected = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?to=2020&messageId=external1&status=unexpected"
)

var testCases = []ChannelHandleTestCase{
	{Label: "Valid Receive", URL: receiveValidMessage, Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+2349067554729")},
	{Label: "Receive URL check", URL: receiveURL, Status: 200, Response: "no to parameter, ignored"},
	{Label: "Status URL check", URL: statusURL, Status: 200, Response: "no messageId parameter, ignored"},

	{Label: "Status delivered", URL: statusDelivered, Status: 200, Response: `"status":"D"`, ExternalID: Sp("external1")},
	{Label: "Status expired", URL: statusExpired, Status: 200, Response: `"status":"F"`, ExternalID: Sp("external1")},
	{Label: "Status failed", URL: statusFailed, Status: 200, Response: `"status":"F"`, ExternalID: Sp("external1")},
	{Label: "Status accepted", URL: statusAccepted, Status: 200, Response: `"status":"S"`, ExternalID: Sp("external1")},
	{Label: "Status buffered", URL: statusBuffered, Status: 200, Response: `"status":"S"`, ExternalID: Sp("external1")},
	{Label: "Status unexpected", URL: statusUnexpected, Status: 200, Response: "ignoring unknown status report", ExternalID: Sp("external1")},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, NewHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, NewHandler(), testCases)
}
