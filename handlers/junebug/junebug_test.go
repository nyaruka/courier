package junebug

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

var testChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "JN", "2020", "US", map[string]interface{}{
	"username": "user1",
	"password": "pass1",
	"send_url": "https://foo.bar/",
})

var authenticatedTestChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "JN", "2020", "US", map[string]interface{}{
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
	{Label: "Receive Valid Message", URL: inboundURL, Data: validMsg, ExpectedStatus: 200, ExpectedResponse: "Accepted",
		ExpectedMsgText: Sp("hello world"), ExpectedURN: Sp("tel:+250788383383"),
		ExpectedDate: time.Date(2017, 01, 01, 1, 2, 3, 50000000, time.UTC)},

	{Label: "Invalid URN", URL: inboundURL, Data: invalidURN,
		ExpectedStatus: 400, ExpectedResponse: "phone number supplied is not a number"},
	{Label: "Invalid Timestamp", URL: inboundURL, Data: invalidTimestamp,
		ExpectedStatus: 400, ExpectedResponse: "unable to parse date"},
	{Label: "Missing Message ID", URL: inboundURL, Data: missingMessageID,
		ExpectedStatus: 400, ExpectedResponse: "'MessageID' failed on the 'required'"},

	{Label: "Receive Pending Event", URL: eventURL, Data: pendingEvent, ExpectedStatus: 200, ExpectedResponse: "Ignored"},
	{Label: "Receive Sent Event", URL: eventURL, Data: sentEvent, ExpectedStatus: 200, ExpectedResponse: "Accepted",
		ExpectedExternalID: Sp("xx12345"), ExpectedMsgStatus: Sp("S")},
	{Label: "Receive Delivered Event", URL: eventURL, Data: deliveredEvent, ExpectedStatus: 200, ExpectedResponse: "Accepted",
		ExpectedExternalID: Sp("xx12345"), ExpectedMsgStatus: Sp("D")},
	{Label: "Receive Failed Event", URL: eventURL, Data: failedEvent, ExpectedStatus: 200, ExpectedResponse: "Accepted",
		ExpectedExternalID: Sp("xx12345"), ExpectedMsgStatus: Sp("F")},
	{Label: "Receive Unknown Event", URL: eventURL, Data: unknownEvent, ExpectedStatus: 200, ExpectedResponse: "Ignored"},

	{Label: "Receive Invalid JSON", URL: eventURL, Data: "not json", ExpectedStatus: 400, ExpectedResponse: "Error"},
	{Label: "Receive Missing Event Type", URL: eventURL, Data: missingEventType, ExpectedStatus: 400, ExpectedResponse: "Error"},
}

var authenticatedTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: inboundURL, Data: validMsg, Headers: map[string]string{"Authorization": "Token sesame"},
		ExpectedStatus: 200, ExpectedResponse: "Accepted",
		ExpectedMsgText: Sp("hello world"), ExpectedURN: Sp("tel:+250788383383"),
		ExpectedDate: time.Date(2017, 01, 01, 1, 2, 3, 50000000, time.UTC)},

	{Label: "Invalid Incoming Authorization", URL: inboundURL, Data: validMsg, Headers: map[string]string{"Authorization": "Token foo"},
		ExpectedStatus: 401, ExpectedResponse: "Unauthorized"},

	{Label: "Receive Sent Event", URL: eventURL, Data: sentEvent, Headers: map[string]string{"Authorization": "Token sesame"},
		ExpectedStatus: 200, ExpectedResponse: "Accepted",
		ExpectedExternalID: Sp("xx12345"), ExpectedMsgStatus: Sp("S")},
	{Label: "Invalid Incoming Authorization", URL: eventURL, Data: sentEvent, Headers: map[string]string{"Authorization": "Token foo"},
		ExpectedStatus: 401, ExpectedResponse: "Unauthorized"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, []courier.Channel{testChannel}, newHandler(), testCases)
	RunChannelTestCases(t, []courier.Channel{authenticatedTestChannel}, newHandler(), authenticatedTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, []courier.Channel{testChannel}, newHandler(), testCases)
}

func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	c.(*test.MockChannel).SetConfig("send_url", s.URL)
}

var sendTestCases = []ChannelSendTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message",
		MsgURN:              "tel:+250788383383",
		MockResponseBody:    `{"result":{"message_id":"externalID"}}`,
		MockResponseStatus:  200,
		ExpectedHeaders:     map[string]string{"Authorization": "Basic dXNlcjE6cGFzczE="},
		ExpectedRequestBody: `{"event_url":"https://localhost/c/jn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/event","content":"Simple Message","from":"2020","to":"+250788383383"}`,
		ExpectedStatus:      "W",
		ExpectedExternalID:  "externalID",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Send Attachement",
		MsgText:             "My pic!",
		MsgAttachments:      []string{"image/jpeg:https://foo.bar/image.jpg"},
		MsgURN:              "tel:+250788383383",
		MockResponseBody:    `{"result":{"message_id":"externalID"}}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"event_url":"https://localhost/c/jn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/event","content":"My pic!\nhttps://foo.bar/image.jpg","from":"2020","to":"+250788383383"}`,
		ExpectedStatus:      "W",
		ExpectedExternalID:  "externalID",
		SendPrep:            setSendURL,
	},
	{
		Label:              "Invalid JSON Response",
		MsgText:            "Error Sending",
		MsgURN:             "tel:+250788383383",
		MockResponseStatus: 200,
		MockResponseBody:   "not json",
		ExpectedStatus:     "E",
		ExpectedErrors:     []courier.ChannelError{courier.NewChannelError("unable to get result.message_id from body", "")},
		SendPrep:           setSendURL,
	},
	{
		Label:              "Missing External ID",
		MsgText:            "Error Sending",
		MsgURN:             "tel:+250788383383",
		MockResponseStatus: 200,
		MockResponseBody:   "{}",
		ExpectedStatus:     "E",
		ExpectedErrors:     []courier.ChannelError{courier.NewChannelError("unable to get result.message_id from body", "")},
		SendPrep:           setSendURL,
	},
	{
		Label:              "Error Sending",
		MsgText:            "Error Sending",
		MsgURN:             "tel:+250788383383",
		MockResponseStatus: 403,
		ExpectedStatus:     "E",
		SendPrep:           setSendURL,
	},
}

var authenticatedSendTestCases = []ChannelSendTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message",
		MsgURN:              "tel:+250788383383",
		MockResponseBody:    `{"result":{"message_id":"externalID"}}`,
		MockResponseStatus:  200,
		ExpectedHeaders:     map[string]string{"Authorization": "Basic dXNlcjE6cGFzczE="},
		ExpectedRequestBody: `{"event_url":"https://localhost/c/jn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/event","content":"Simple Message","from":"2020","to":"+250788383383","event_auth_token":"sesame"}`,
		ExpectedStatus:      "W",
		ExpectedExternalID:  "externalID",
		SendPrep:            setSendURL,
	},
}

func TestSending(t *testing.T) {
	RunChannelSendTestCases(t, testChannel, newHandler(), sendTestCases, nil)
	RunChannelSendTestCases(t, authenticatedTestChannel, newHandler(), authenticatedSendTestCases, nil)
}
