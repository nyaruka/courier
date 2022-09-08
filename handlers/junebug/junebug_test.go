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
	{Label: "Receive Valid Message", URL: inboundURL, Data: validMsg, ExpectedRespStatus: 200, ExpectedRespBody: "Accepted",
		ExpectedMsgText: Sp("hello world"), ExpectedURN: "tel:+250788383383",
		ExpectedDate: time.Date(2017, 01, 01, 1, 2, 3, 50000000, time.UTC)},

	{Label: "Invalid URN", URL: inboundURL, Data: invalidURN,
		ExpectedRespStatus: 400, ExpectedRespBody: "phone number supplied is not a number"},
	{Label: "Invalid Timestamp", URL: inboundURL, Data: invalidTimestamp,
		ExpectedRespStatus: 400, ExpectedRespBody: "unable to parse date"},
	{Label: "Missing Message ID", URL: inboundURL, Data: missingMessageID,
		ExpectedRespStatus: 400, ExpectedRespBody: "'MessageID' failed on the 'required'"},

	{Label: "Receive Pending Event", URL: eventURL, Data: pendingEvent, ExpectedRespStatus: 200, ExpectedRespBody: "Ignored"},
	{Label: "Receive Sent Event", URL: eventURL, Data: sentEvent, ExpectedRespStatus: 200, ExpectedRespBody: "Accepted",
		ExpectedExternalID: "xx12345", ExpectedMsgStatus: "S"},
	{Label: "Receive Delivered Event", URL: eventURL, Data: deliveredEvent, ExpectedRespStatus: 200, ExpectedRespBody: "Accepted",
		ExpectedExternalID: "xx12345", ExpectedMsgStatus: "D"},
	{Label: "Receive Failed Event", URL: eventURL, Data: failedEvent, ExpectedRespStatus: 200, ExpectedRespBody: "Accepted",
		ExpectedExternalID: "xx12345", ExpectedMsgStatus: "F"},
	{Label: "Receive Unknown Event", URL: eventURL, Data: unknownEvent, ExpectedRespStatus: 200, ExpectedRespBody: "Ignored"},

	{Label: "Receive Invalid JSON", URL: eventURL, Data: "not json", ExpectedRespStatus: 400, ExpectedRespBody: "Error"},
	{Label: "Receive Missing Event Type", URL: eventURL, Data: missingEventType, ExpectedRespStatus: 400, ExpectedRespBody: "Error"},
}

var authenticatedTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: inboundURL, Data: validMsg, Headers: map[string]string{"Authorization": "Token sesame"},
		ExpectedRespStatus: 200, ExpectedRespBody: "Accepted",
		ExpectedMsgText: Sp("hello world"), ExpectedURN: "tel:+250788383383",
		ExpectedDate: time.Date(2017, 01, 01, 1, 2, 3, 50000000, time.UTC)},

	{Label: "Invalid Incoming Authorization", URL: inboundURL, Data: validMsg, Headers: map[string]string{"Authorization": "Token foo"},
		ExpectedRespStatus: 401, ExpectedRespBody: "Unauthorized"},

	{Label: "Receive Sent Event", URL: eventURL, Data: sentEvent, Headers: map[string]string{"Authorization": "Token sesame"},
		ExpectedRespStatus: 200, ExpectedRespBody: "Accepted",
		ExpectedExternalID: "xx12345", ExpectedMsgStatus: "S"},
	{Label: "Invalid Incoming Authorization", URL: eventURL, Data: sentEvent, Headers: map[string]string{"Authorization": "Token foo"},
		ExpectedRespStatus: 401, ExpectedRespBody: "Unauthorized"},
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
		ExpectedMsgStatus:   "W",
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
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "externalID",
		SendPrep:            setSendURL,
	},
	{
		Label:              "Invalid JSON Response",
		MsgText:            "Error Sending",
		MsgURN:             "tel:+250788383383",
		MockResponseStatus: 200,
		MockResponseBody:   "not json",
		ExpectedMsgStatus:  "E",
		ExpectedErrors:     []courier.ChannelError{courier.NewChannelError("unable to get result.message_id from body", "")},
		SendPrep:           setSendURL,
	},
	{
		Label:              "Missing External ID",
		MsgText:            "Error Sending",
		MsgURN:             "tel:+250788383383",
		MockResponseStatus: 200,
		MockResponseBody:   "{}",
		ExpectedMsgStatus:  "E",
		ExpectedErrors:     []courier.ChannelError{courier.NewChannelError("unable to get result.message_id from body", "")},
		SendPrep:           setSendURL,
	},
	{
		Label:              "Error Sending",
		MsgText:            "Error Sending",
		MsgURN:             "tel:+250788383383",
		MockResponseStatus: 403,
		ExpectedMsgStatus:  "E",
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
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "externalID",
		SendPrep:            setSendURL,
	},
}

func TestSending(t *testing.T) {
	RunChannelSendTestCases(t, testChannel, newHandler(), sendTestCases, []string{BasicAuth("user1", "pass1"), "sesame"}, nil)
	RunChannelSendTestCases(t, authenticatedTestChannel, newHandler(), authenticatedSendTestCases, []string{BasicAuth("user1", "pass1"), "sesame"}, nil)
}
