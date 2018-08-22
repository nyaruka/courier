package junebug

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "JN", "2020", "US", map[string]interface{}{
	"username": "user1",
	"password": "pass1",
	"send_url": "https://foo.bar/",
})

var authenticatedTestChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "JN", "2020", "US", map[string]interface{}{
	"username": "user1",
	"password": "pass1",
	"send_url": "https://foo.bar/",
	"secret":   "sesame",
})

var (
	inboundURL = "/c/jn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/inbound"
	validMsg   = `{
		"from": "+250788383383",
		"timestamp": "2017-01-01 01:02:03.05",
		"content": "hello world",
		"to": "2020",
		"message_id": "external-id"
	}
	`

	invalidURN = `{
		"from": "MTN",
		"timestamp": "2017-01-01 01:02:03.05",
		"content": "hello world",
		"to": "2020",
		"message_id": "external-id"
	}
	`

	invalidTimestamp = `{
		"from": "+250788383383",
		"timestamp": "20170101T01:02:03.05",
		"content": "hello world",
		"to": "2020",
		"message_id": "external-id"
	}
	`

	missingMessageID = `{
		"from": "+250788383383",
		"timestamp": "2017-01-01 01:02:03.05",
		"content": "hello world",
		"to": "2020"
	}
	`

	eventURL = "/c/jn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/event"

	pendingEvent = `{
		"event_type": "delivery_pending",
		"message_id": "xx12345"
	}`

	sentEvent = `{
		"event_type": "submitted",
		"message_id": "xx12345"
	}`

	deliveredEvent = `{
		"event_type": "delivery_succeeded",
		"message_id": "xx12345"
	}`

	failedEvent = `{
		"event_type": "rejected",
		"message_id": "xx12345"
	}`

	unknownEvent = `{
		"event_type": "unknown",
		"message_id": "xx12345"
	}`

	missingEventType = `{
		"message_id": "xx12345"
	}`
)

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: inboundURL, Data: validMsg, Status: 200, Response: "Accepted",
		Text: Sp("hello world"), URN: Sp("tel:+250788383383"),
		Date: Tp(time.Date(2017, 01, 01, 1, 2, 3, 50000000, time.UTC))},

	{Label: "Invalid URN", URL: inboundURL, Data: invalidURN,
		Status: 400, Response: "phone number supplied is not a number"},
	{Label: "Invalid Timestamp", URL: inboundURL, Data: invalidTimestamp,
		Status: 400, Response: "unable to parse date"},
	{Label: "Missing Message ID", URL: inboundURL, Data: missingMessageID,
		Status: 400, Response: "'MessageID' failed on the 'required'"},

	{Label: "Receive Pending Event", URL: eventURL, Data: pendingEvent, Status: 200, Response: "Ignored"},
	{Label: "Receive Sent Event", URL: eventURL, Data: sentEvent, Status: 200, Response: "Accepted",
		ExternalID: Sp("xx12345"), MsgStatus: Sp("S")},
	{Label: "Receive Delivered Event", URL: eventURL, Data: deliveredEvent, Status: 200, Response: "Accepted",
		ExternalID: Sp("xx12345"), MsgStatus: Sp("D")},
	{Label: "Receive Failed Event", URL: eventURL, Data: failedEvent, Status: 200, Response: "Accepted",
		ExternalID: Sp("xx12345"), MsgStatus: Sp("F")},
	{Label: "Receive Unknown Event", URL: eventURL, Data: unknownEvent, Status: 200, Response: "Ignored"},

	{Label: "Receive Invalid JSON", URL: eventURL, Data: "not json", Status: 400, Response: "Error"},
	{Label: "Receive Missing Event Type", URL: eventURL, Data: missingEventType, Status: 400, Response: "Error"},
}

var authenticatedTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: inboundURL, Data: validMsg, Headers: map[string]string{"Authorization": "Token sesame"},
		Status: 200, Response: "Accepted",
		Text: Sp("hello world"), URN: Sp("tel:+250788383383"),
		Date: Tp(time.Date(2017, 01, 01, 1, 2, 3, 50000000, time.UTC))},

	{Label: "Invalid Incoming Authorization", URL: inboundURL, Data: validMsg, Headers: map[string]string{"Authorization": "Token foo"},
		Status: 401, Response: "Unauthorized"},

	{Label: "Receive Sent Event", URL: eventURL, Data: sentEvent, Headers: map[string]string{"Authorization": "Token sesame"},
		Status: 200, Response: "Accepted",
		ExternalID: Sp("xx12345"), MsgStatus: Sp("S")},
	{Label: "Invalid Incoming Authorization", URL: eventURL, Data: sentEvent, Headers: map[string]string{"Authorization": "Token foo"},
		Status: 401, Response: "Unauthorized"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, []courier.Channel{testChannel}, newHandler(), testCases)
	RunChannelTestCases(t, []courier.Channel{authenticatedTestChannel}, newHandler(), authenticatedTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, []courier.Channel{testChannel}, newHandler(), testCases)
}

func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	c.(*courier.MockChannel).SetConfig("send_url", s.URL)
}

var sendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text:           "Simple Message",
		URN:            "tel:+250788383383",
		Status:         "W",
		ExternalID:     "externalID",
		Headers:        map[string]string{"Authorization": "Basic dXNlcjE6cGFzczE="},
		RequestBody:    `{"event_url":"https://localhost/c/jn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/event","content":"Simple Message","from":"2020","to":"+250788383383"}`,
		ResponseBody:   `{"result":{"message_id":"externalID"}}`,
		ResponseStatus: 200,
		SendPrep:       setSendURL},
	{Label: "Send Attachement",
		Text:           "My pic!",
		Attachments:    []string{"image/jpeg:https://foo.bar/image.jpg"},
		URN:            "tel:+250788383383",
		Status:         "W",
		ExternalID:     "externalID",
		RequestBody:    `{"event_url":"https://localhost/c/jn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/event","content":"My pic!\nhttps://foo.bar/image.jpg","from":"2020","to":"+250788383383"}`,
		ResponseBody:   `{"result":{"message_id":"externalID"}}`,
		ResponseStatus: 200,
		SendPrep:       setSendURL},
	{Label: "Invalid JSON Response",
		Text: "Error Sending", URN: "tel:+250788383383",
		Status:         "E",
		ResponseStatus: 200,
		ResponseBody:   "not json",
		SendPrep:       setSendURL},
	{Label: "Missing External ID",
		Text: "Error Sending", URN: "tel:+250788383383",
		Status:         "E",
		ResponseStatus: 200,
		ResponseBody:   "{}",
		SendPrep:       setSendURL},
	{Label: "Error Sending",
		Text: "Error Sending", URN: "tel:+250788383383",
		Status:         "E",
		ResponseStatus: 403,
		SendPrep:       setSendURL},
}

var authenticatedSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text:           "Simple Message",
		URN:            "tel:+250788383383",
		Status:         "W",
		ExternalID:     "externalID",
		Headers:        map[string]string{"Authorization": "Basic dXNlcjE6cGFzczE="},
		RequestBody:    `{"event_url":"https://localhost/c/jn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/event","content":"Simple Message","from":"2020","to":"+250788383383","event_auth_token":"sesame"}`,
		ResponseBody:   `{"result":{"message_id":"externalID"}}`,
		ResponseStatus: 200,
		SendPrep:       setSendURL},
}

func TestSending(t *testing.T) {
	RunChannelSendTestCases(t, testChannel, newHandler(), sendTestCases, nil)
	RunChannelSendTestCases(t, authenticatedTestChannel, newHandler(), authenticatedSendTestCases, nil)
}
