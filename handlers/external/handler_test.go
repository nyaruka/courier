package external

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/courier/utils"
)

const (
	receiveURL = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US", nil),
}

var gmChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "GM", nil),
}

var handleTestCases = []IncomingTestCase{
	{
		Label:                "Receive Valid Message",
		URL:                  receiveURL + "?sender=%2B2349067554729&text=Join",
		Data:                 "empty",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Join"),
		ExpectedURN:          "tel:+2349067554729",
	},
	{
		Label:                "Receive Valid Post",
		URL:                  receiveURL,
		Data:                 "sender=%2B2349067554729&text=Join",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Join"),
		ExpectedURN:          "tel:+2349067554729",
	},
	{
		Label:                "Receive Valid Post multipart form",
		URL:                  receiveURL,
		MultipartForm:        map[string]string{"sender": "2349067554729", "text": "Join"},
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Join"),
		ExpectedURN:          "tel:+2349067554729",
	},
	{
		Label:                "Receive Valid From",
		URL:                  receiveURL + "?from=%2B2349067554729&text=Join",
		Data:                 "empty",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Join"),
		ExpectedURN:          "tel:+2349067554729",
	},
	{
		Label:                "Receive Country Parse",
		URL:                  receiveURL + "?from=2349067554729&text=Join",
		Data:                 "empty",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Join"),
		ExpectedURN:          "tel:+2349067554729",
	},
	{
		Label:                "Receive Valid Message With Date",
		URL:                  receiveURL + "?sender=%2B2349067554729&text=Join&date=2017-06-23T12:30:00.500Z",
		Data:                 "empty",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Join"),
		ExpectedURN:          "tel:+2349067554729",
		ExpectedDate:         time.Date(2017, 6, 23, 12, 30, 0, int(500*time.Millisecond), time.UTC),
	},
	{
		Label:                "Receive Valid Message With Time",
		URL:                  receiveURL + "?sender=%2B2349067554729&text=Join&time=2017-06-23T12:30:00Z",
		Data:                 "empty",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Join"),
		ExpectedURN:          "tel:+2349067554729",
		ExpectedDate:         time.Date(2017, 6, 23, 12, 30, 0, 0, time.UTC),
	},
	{
		Label:                "Invalid URN",
		URL:                  receiveURL + "?sender=MTN&text=Join",
		Data:                 "empty",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "phone number supplied is not a number",
	},
	{
		Label:                "Receive No Params",
		URL:                  receiveURL,
		Data:                 "empty",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "must have one of 'sender' or 'from' set",
	},
	{
		Label:              "Receive No Sender",
		URL:                receiveURL + "?text=Join",
		Data:               "empty",
		ExpectedRespStatus: 400, ExpectedBodyContains: "must have one of 'sender' or 'from' set",
	},
	{
		Label:                "Receive Invalid Date",
		URL:                  receiveURL + "?sender=%2B2349067554729&text=Join&time=20170623T123000Z",
		Data:                 "empty",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "invalid date format, must be RFC 3339",
	},
	{
		Label:                "Failed No Params",
		URL:                  "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/failed/",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "field 'id' required",
	},
	{
		Label:                "Failed Valid",
		URL:                  "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/failed/?id=12345",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"F"`,
		ExpectedStatuses:     []ExpectedStatus{{MsgID: 12345, Status: courier.MsgStatusFailed}},
	},
	{
		Label:                "Invalid Status",
		URL:                  "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/wired/",
		ExpectedRespStatus:   404,
		ExpectedBodyContains: `page not found`,
		NoLogsExpected:       true,
	},
	{
		Label:                "Sent Valid",
		URL:                  "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/sent/?id=12345",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"S"`,
		ExpectedStatuses:     []ExpectedStatus{{MsgID: 12345, Status: courier.MsgStatusSent}},
	},
	{
		Label:                "Delivered Valid",
		URL:                  "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered/?id=12345",
		Data:                 "nothing",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses:     []ExpectedStatus{{MsgID: 12345, Status: courier.MsgStatusDelivered}},
	},
	{
		Label:                "Delivered Valid Post",
		URL:                  "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered/",
		Data:                 "id=12345",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses:     []ExpectedStatus{{MsgID: 12345, Status: courier.MsgStatusDelivered}},
	},
	{
		Label:                "Stopped Event",
		URL:                  "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/stopped/?from=%2B2349067554729",
		Data:                 "nothing",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedEvents: []ExpectedEvent{
			{Type: courier.EventTypeStopContact, URN: "tel:+2349067554729"},
		},
	},
	{
		Label:                "Stopped Event Post",
		URL:                  "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/stopped/",
		Data:                 "from=%2B2349067554729",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedEvents: []ExpectedEvent{
			{Type: courier.EventTypeStopContact, URN: "tel:+2349067554729"},
		},
	},
	{
		Label:                "Stopped Event Invalid URN",
		URL:                  "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/stopped/?from=MTN",
		Data:                 "empty",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "phone number supplied is not a number",
	},
	{
		Label:                "Stopped event No Params",
		URL:                  "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/stopped/",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "field 'from' required",
	},
}

var testSOAPReceiveChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]any{
			configTextXPath:             "//content",
			configFromXPath:             "//source",
			configMOResponse:            "<?xml version=“1.0”?><return>0</return>",
			configMOResponseContentType: "text/xml",
		},
	),
}

var handleSOAPReceiveTestCases = []IncomingTestCase{
	{
		Label:                "Receive Valid Post SOAP",
		URL:                  "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/",
		Data:                 `<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:com="com.hero"><soapenv:Header/><soapenv:Body><com:moRequest><source>2349067554729</source><content>Join</content></com:moRequest></soapenv:Body></soapenv:Envelope>`,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "<?xml version=“1.0”?><return>0</return>",
		ExpectedMsgText:      Sp("Join"),
		ExpectedURN:          "tel:+2349067554729",
	},
	{
		Label:                "Receive Invalid SOAP",
		URL:                  "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/",
		Data:                 `<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:com="com.hero"><soapenv:Header/><soapenv:Body></soapenv:Body></soapenv:Envelope>`,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "missing from",
	},
}

var gmTestCases = []IncomingTestCase{
	{
		Label:                "Receive Non Plus Message",
		URL:                  "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?sender=2207222333&text=Join",
		Data:                 "empty",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Join"),
		ExpectedURN:          "tel:+2207222333",
	},
}

var customChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]any{
			configMOFromField: "from_number",
			configMODateField: "timestamp",
			configMOTextField: "messageText",
		},
	),
}

var customTestCases = []IncomingTestCase{
	{
		Label:                "Receive Custom Message",
		URL:                  "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?from_number=12067799192&messageText=Join&timestamp=2017-06-23T12:30:00Z",
		Data:                 "empty",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Join"),
		ExpectedURN:          "tel:+12067799192",
		ExpectedDate:         time.Date(2017, 6, 23, 12, 30, 0, 0, time.UTC),
	},
	{
		Label:                "Receive Custom Missing",
		URL:                  "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?sent_from=12067799192&messageText=Join",
		Data:                 "empty",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "must have one of 'sender' or 'from' set",
	},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), handleTestCases)
	RunIncomingTestCases(t, testSOAPReceiveChannels, newHandler(), handleSOAPReceiveTestCases)
	RunIncomingTestCases(t, gmChannels, newHandler(), gmTestCases)
	RunIncomingTestCases(t, customChannels, newHandler(), customTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), handleTestCases)
	RunChannelBenchmarks(b, testSOAPReceiveChannels, newHandler(), handleSOAPReceiveTestCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	// this is actually a path, which we'll combine with the test server URL
	sendURL := c.StringConfigForKey("send_path", "")
	sendURL, _ = utils.AddURLPath(s.URL, sendURL)
	c.(*test.MockChannel).SetConfig(courier.ConfigSendURL, sendURL)
}

var longSendTestCases = []OutgoingTestCase{
	{
		Label:   "Long Send",
		MsgText: "This is a long message that will be longer than 30....... characters", MsgURN: "tel:+250788383383",
		MsgQuickReplies:    []string{"One"},
		MockResponseBody:   "0: Accepted for delivery",
		MockResponseStatus: 200,
		ExpectedURLParams:  map[string]string{"text": "characters", "to": "+250788383383", "from": "2020", "quick_reply": "One"},
		ExpectedMsgStatus:  "W",
		SendPrep:           setSendURL,
	},
}

var getSendSmartEncodingTestCases = []OutgoingTestCase{
	{
		Label:              "Smart Encoding",
		MsgText:            "Fancy “Smart” Quotes",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   "0: Accepted for delivery",
		MockResponseStatus: 200,
		ExpectedURLParams:  map[string]string{"text": `Fancy "Smart" Quotes`, "to": "+250788383383", "from": "2020"},
		ExpectedHeaders:    map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		ExpectedMsgStatus:  "W",
		SendPrep:           setSendURL,
	},
}

var postSendSmartEncodingTestCases = []OutgoingTestCase{
	{
		Label:              "Smart Encoding",
		MsgText:            "Fancy “Smart” Quotes",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   "0: Accepted for delivery",
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"text": `Fancy "Smart" Quotes`, "to": "+250788383383", "from": "2020"},
		ExpectedHeaders:    map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		ExpectedMsgStatus:  "W",
		SendPrep:           setSendURL,
	},
}

var getSendTestCases = []OutgoingTestCase{
	{
		Label:              "Plain Send",
		MsgText:            "Simple Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   "0: Accepted for delivery",
		MockResponseStatus: 200,
		ExpectedURLParams:  map[string]string{"text": "Simple Message", "to": "+250788383383", "from": "2020"},
		ExpectedHeaders:    map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		ExpectedMsgStatus:  "W",
		SendPrep:           setSendURL,
	},
	{
		Label:   "Unicode Send",
		MsgText: "☺", MsgURN: "tel:+250788383383",
		MockResponseBody:   "0: Accepted for delivery",
		MockResponseStatus: 200,
		ExpectedURLParams:  map[string]string{"text": "☺", "to": "+250788383383", "from": "2020"},
		ExpectedHeaders:    map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		ExpectedMsgStatus:  "W",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Error Sending",
		MsgText:            "Error Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   "1: Unknown channel",
		MockResponseStatus: 401,
		ExpectedURLParams:  map[string]string{"text": `Error Message`, "to": "+250788383383"},
		ExpectedHeaders:    map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Send Attachment",
		MsgText:            "My pic!",
		MsgURN:             "tel:+250788383383",
		MsgAttachments:     []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody:   `0: Accepted for delivery`,
		MockResponseStatus: 200,
		ExpectedURLParams:  map[string]string{"text": "My pic!\nhttps://foo.bar/image.jpg", "to": "+250788383383", "from": "2020"},
		ExpectedHeaders:    map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		ExpectedMsgStatus:  "W",
		SendPrep:           setSendURL,
	},
}

var postSendTestCases = []OutgoingTestCase{
	{
		Label:              "Plain Send",
		MsgText:            "Simple Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   "0: Accepted for delivery",
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"text": "Simple Message", "to": "+250788383383", "from": "2020"},
		ExpectedHeaders:    map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		ExpectedMsgStatus:  "W",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Unicode Send",
		MsgText:            "☺",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   "0: Accepted for delivery",
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"text": "☺", "to": "+250788383383", "from": "2020"},
		ExpectedHeaders:    map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		ExpectedMsgStatus:  "W",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Error Sending",
		MsgText:            "Error Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   "1: Unknown channel",
		MockResponseStatus: 401,
		ExpectedPostParams: map[string]string{"text": `Error Message`, "to": "+250788383383"},
		ExpectedHeaders:    map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
	{
		Label:            "Send Attachment",
		MsgText:          "My pic!",
		MsgURN:           "tel:+250788383383",
		MsgAttachments:   []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody: `0: Accepted for delivery`, MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"text": "My pic!\nhttps://foo.bar/image.jpg", "to": "+250788383383", "from": "2020"},
		ExpectedHeaders:    map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		ExpectedMsgStatus:  "W",
		SendPrep:           setSendURL,
	},
}

var postSendCustomContentTypeTestCases = []OutgoingTestCase{
	{
		Label:              "Plain Send",
		MsgText:            "Simple Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   "0: Accepted for delivery",
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"text": "Simple Message", "to": "250788383383", "from": "2020"},
		ExpectedHeaders:    map[string]string{"Content-Type": "application/x-www-form-urlencoded; charset=utf-8"},
		ExpectedMsgStatus:  "W",
		SendPrep:           setSendURL,
	},
}

var jsonSendTestCases = []OutgoingTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message",
		MsgURN:              "tel:+250788383383",
		MockResponseBody:    "0: Accepted for delivery",
		MockResponseStatus:  200,
		ExpectedRequestBody: `{ "to":"+250788383383", "text":"Simple Message", "from":"2020", "quick_replies":[] }`,
		ExpectedHeaders:     map[string]string{"Authorization": "Token ABCDEF", "Content-Type": "application/json"},
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Unicode Send",
		MsgText:             `☺ "hi!"`,
		MsgURN:              "tel:+250788383383",
		MockResponseBody:    "0: Accepted for delivery",
		MockResponseStatus:  200,
		ExpectedRequestBody: `{ "to":"+250788383383", "text":"☺ \"hi!\"", "from":"2020", "quick_replies":[] }`,
		ExpectedHeaders:     map[string]string{"Authorization": "Token ABCDEF", "Content-Type": "application/json"},
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Error Sending",
		MsgText:             "Error Message",
		MsgURN:              "tel:+250788383383",
		MockResponseBody:    "1: Unknown channel",
		MockResponseStatus:  401,
		ExpectedRequestBody: `{ "to":"+250788383383", "text":"Error Message", "from":"2020", "quick_replies":[] }`,
		ExpectedHeaders:     map[string]string{"Authorization": "Token ABCDEF", "Content-Type": "application/json"},
		ExpectedMsgStatus:   "E",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Send Attachment",
		MsgText:             "My pic!",
		MsgURN:              "tel:+250788383383",
		MsgAttachments:      []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody:    `0: Accepted for delivery`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{ "to":"+250788383383", "text":"My pic!\nhttps://foo.bar/image.jpg", "from":"2020", "quick_replies":[] }`,
		ExpectedHeaders:     map[string]string{"Authorization": "Token ABCDEF", "Content-Type": "application/json"},
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL},
	{
		Label:               "Send Quick Replies",
		MsgText:             "Some message",
		MsgURN:              "tel:+250788383383",
		MsgQuickReplies:     []string{"One", "Two", "Three"},
		MockResponseBody:    "0: Accepted for delivery",
		MockResponseStatus:  200,
		ExpectedRequestBody: `{ "to":"+250788383383", "text":"Some message", "from":"2020", "quick_replies":["One","Two","Three"] }`,
		ExpectedHeaders:     map[string]string{"Authorization": "Token ABCDEF", "Content-Type": "application/json"},
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
}

var jsonLongSendTestCases = []OutgoingTestCase{
	{
		Label:               "Send Quick Replies",
		MsgText:             "This is a long message that will be longer than 30....... characters",
		MsgURN:              "tel:+250788383383",
		MsgQuickReplies:     []string{"One", "Two", "Three"},
		MockResponseBody:    "0: Accepted for delivery",
		MockResponseStatus:  200,
		ExpectedRequestBody: `{ "to":"+250788383383", "text":"characters", "from":"2020", "quick_replies":["One","Two","Three"] }`,
		ExpectedHeaders:     map[string]string{"Authorization": "Token ABCDEF", "Content-Type": "application/json"},
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
}

var xmlSendTestCases = []OutgoingTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message",
		MsgURN:              "tel:+250788383383",
		MockResponseBody:    "0: Accepted for delivery",
		MockResponseStatus:  200,
		ExpectedRequestBody: `<msg><to>+250788383383</to><text>Simple Message</text><from>2020</from><quick_replies></quick_replies></msg>`,
		ExpectedHeaders:     map[string]string{"Content-Type": "text/xml; charset=utf-8"},
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Unicode Send",
		MsgText:             `☺`,
		MsgURN:              "tel:+250788383383",
		MockResponseBody:    "0: Accepted for delivery",
		MockResponseStatus:  200,
		ExpectedRequestBody: `<msg><to>+250788383383</to><text>☺</text><from>2020</from><quick_replies></quick_replies></msg>`,
		ExpectedHeaders:     map[string]string{"Content-Type": "text/xml; charset=utf-8"},
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Error Sending",
		MsgText:             "Error Message",
		MsgURN:              "tel:+250788383383",
		MockResponseBody:    "1: Unknown channel",
		MockResponseStatus:  401,
		ExpectedRequestBody: `<msg><to>+250788383383</to><text>Error Message</text><from>2020</from><quick_replies></quick_replies></msg>`,
		ExpectedHeaders:     map[string]string{"Content-Type": "text/xml; charset=utf-8"},
		ExpectedMsgStatus:   "E",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Send Attachment",
		MsgText:             "My pic!",
		MsgURN:              "tel:+250788383383",
		MsgAttachments:      []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody:    `0: Accepted for delivery`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `<msg><to>+250788383383</to><text>My pic!&#xA;https://foo.bar/image.jpg</text><from>2020</from><quick_replies></quick_replies></msg>`,
		ExpectedHeaders:     map[string]string{"Content-Type": "text/xml; charset=utf-8"},
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Send Quick Replies",
		MsgText:             "Some message",
		MsgURN:              "tel:+250788383383",
		MsgQuickReplies:     []string{"One", "Two", "Three"},
		MockResponseBody:    "0: Accepted for delivery",
		MockResponseStatus:  200,
		ExpectedRequestBody: "<msg><to>+250788383383</to><text>Some message</text><from>2020</from><quick_replies><item>One</item><item>Two</item><item>Three</item></quick_replies></msg>",
		ExpectedHeaders:     map[string]string{"Content-Type": "text/xml; charset=utf-8"},
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
}

var xmlLongSendTestCases = []OutgoingTestCase{
	{
		Label:               "Send Quick Replies",
		MsgText:             "This is a long message that will be longer than 30....... characters",
		MsgURN:              "tel:+250788383383",
		MsgQuickReplies:     []string{"One", "Two", "Three"},
		MockResponseBody:    "0: Accepted for delivery",
		MockResponseStatus:  200,
		ExpectedRequestBody: "<msg><to>+250788383383</to><text>characters</text><from>2020</from><quick_replies><item>One</item><item>Two</item><item>Three</item></quick_replies></msg>",
		ExpectedHeaders:     map[string]string{"Content-Type": "text/xml; charset=utf-8"},
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
}

var xmlSendWithResponseContentTestCases = []OutgoingTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message",
		MsgURN:              "tel:+250788383383",
		ExpectedMsgStatus:   "W",
		MockResponseBody:    "<return>0</return>",
		MockResponseStatus:  200,
		ExpectedRequestBody: `<msg><to>+250788383383</to><text>Simple Message</text><from>2020</from><quick_replies></quick_replies></msg>`,
		ExpectedHeaders:     map[string]string{"Content-Type": "text/xml; charset=utf-8"},
		SendPrep:            setSendURL,
	},
	{
		Label:               "Unicode Send",
		MsgText:             `☺`,
		MsgURN:              "tel:+250788383383",
		MockResponseBody:    "<return>0</return>",
		MockResponseStatus:  200,
		ExpectedRequestBody: `<msg><to>+250788383383</to><text>☺</text><from>2020</from><quick_replies></quick_replies></msg>`,
		ExpectedHeaders:     map[string]string{"Content-Type": "text/xml; charset=utf-8"},
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Error Sending",
		MsgText:             "Error Message",
		MsgURN:              "tel:+250788383383",
		MockResponseBody:    "<return>0</return>",
		MockResponseStatus:  401,
		ExpectedRequestBody: `<msg><to>+250788383383</to><text>Error Message</text><from>2020</from><quick_replies></quick_replies></msg>`,
		ExpectedHeaders:     map[string]string{"Content-Type": "text/xml; charset=utf-8"},
		ExpectedMsgStatus:   "E",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Error Sending with 200 status code",
		MsgText:             "Error Message",
		MsgURN:              "tel:+250788383383",
		MockResponseBody:    "<return>1</return>",
		MockResponseStatus:  200,
		ExpectedRequestBody: `<msg><to>+250788383383</to><text>Error Message</text><from>2020</from><quick_replies></quick_replies></msg>`,
		ExpectedHeaders:     map[string]string{"Content-Type": "text/xml; charset=utf-8"},
		ExpectedMsgStatus:   "E",
		ExpectedErrors:      []*courier.ChannelError{courier.ErrorResponseUnexpected("<return>0</return>")},
		SendPrep:            setSendURL,
	},
	{
		Label:               "Send Attachment",
		MsgText:             "My pic!",
		MsgURN:              "tel:+250788383383",
		MsgAttachments:      []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody:    `<return>0</return>`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `<msg><to>+250788383383</to><text>My pic!&#xA;https://foo.bar/image.jpg</text><from>2020</from><quick_replies></quick_replies></msg>`,
		ExpectedHeaders:     map[string]string{"Content-Type": "text/xml; charset=utf-8"},
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Send Quick Replies",
		MsgText:             "Some message",
		MsgURN:              "tel:+250788383383",
		MsgQuickReplies:     []string{"One", "Two", "Three"},
		MockResponseBody:    "<return>0</return>",
		MockResponseStatus:  200,
		ExpectedRequestBody: `<msg><to>+250788383383</to><text>Some message</text><from>2020</from><quick_replies><item>One</item><item>Two</item><item>Three</item></quick_replies></msg>`,
		ExpectedHeaders:     map[string]string{"Content-Type": "text/xml; charset=utf-8"},
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
}

var nationalGetSendTestCases = []OutgoingTestCase{
	{
		Label:              "Plain Send",
		MsgText:            "Simple Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   "0: Accepted for delivery",
		MockResponseStatus: 200,
		ExpectedURLParams:  map[string]string{"text": "Simple Message", "to": "788383383", "from": "2020"},
		ExpectedHeaders:    map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		ExpectedMsgStatus:  "W",
		SendPrep:           setSendURL,
	},
}

func TestOutgoing(t *testing.T) {
	var getChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]any{
			"send_path":              "?to={{to}}&text={{text}}&from={{from}}{{quick_replies}}",
			courier.ConfigSendMethod: http.MethodGet})

	var getSmartChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]any{
			"send_path":              "?to={{to}}&text={{text}}&from={{from}}{{quick_replies}}",
			configEncoding:           encodingSmart,
			courier.ConfigSendMethod: http.MethodGet})

	var postChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]any{
			"send_path":              "",
			courier.ConfigSendBody:   "to={{to}}&text={{text}}&from={{from}}{{quick_replies}}",
			courier.ConfigSendMethod: http.MethodPost})

	var postChannelCustomContentType = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]any{
			"send_path":               "",
			courier.ConfigSendBody:    "to={{to_no_plus}}&text={{text}}&from={{from_no_plus}}{{quick_replies}}",
			courier.ConfigContentType: "application/x-www-form-urlencoded; charset=utf-8",
			courier.ConfigSendMethod:  http.MethodPost})

	var postSmartChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]any{
			"send_path":              "",
			courier.ConfigSendBody:   "to={{to}}&text={{text}}&from={{from}}{{quick_replies}}",
			configEncoding:           encodingSmart,
			courier.ConfigSendMethod: http.MethodPost})

	var jsonChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]any{
			"send_path":               "",
			courier.ConfigSendBody:    `{ "to":{{to}}, "text":{{text}}, "from":{{from}}, "quick_replies":{{quick_replies}} }`,
			courier.ConfigContentType: contentJSON,
			courier.ConfigSendMethod:  http.MethodPost,
			courier.ConfigSendHeaders: map[string]any{"Authorization": "Token ABCDEF", "foo": "bar"},
		})

	var xmlChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]any{
			"send_path":               "",
			courier.ConfigSendBody:    `<msg><to>{{to}}</to><text>{{text}}</text><from>{{from}}</from><quick_replies>{{quick_replies}}</quick_replies></msg>`,
			courier.ConfigContentType: contentXML,
			courier.ConfigSendMethod:  http.MethodPut,
		})

	var xmlChannelWithResponseContent = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]any{
			"send_path":               "",
			courier.ConfigSendBody:    `<msg><to>{{to}}</to><text>{{text}}</text><from>{{from}}</from><quick_replies>{{quick_replies}}</quick_replies></msg>`,
			configMTResponseCheck:     "<return>0</return>",
			courier.ConfigContentType: contentXML,
			courier.ConfigSendMethod:  http.MethodPut,
		})

	RunOutgoingTestCases(t, getChannel, newHandler(), getSendTestCases, nil, nil)
	RunOutgoingTestCases(t, getSmartChannel, newHandler(), getSendTestCases, nil, nil)
	RunOutgoingTestCases(t, getSmartChannel, newHandler(), getSendSmartEncodingTestCases, nil, nil)
	RunOutgoingTestCases(t, postChannel, newHandler(), postSendTestCases, nil, nil)
	RunOutgoingTestCases(t, postChannelCustomContentType, newHandler(), postSendCustomContentTypeTestCases, nil, nil)
	RunOutgoingTestCases(t, postSmartChannel, newHandler(), postSendTestCases, nil, nil)
	RunOutgoingTestCases(t, postSmartChannel, newHandler(), postSendSmartEncodingTestCases, nil, nil)
	RunOutgoingTestCases(t, jsonChannel, newHandler(), jsonSendTestCases, nil, nil)
	RunOutgoingTestCases(t, xmlChannel, newHandler(), xmlSendTestCases, nil, nil)
	RunOutgoingTestCases(t, xmlChannelWithResponseContent, newHandler(), xmlSendWithResponseContentTestCases, nil, nil)

	var getChannel30IntLength = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]any{
			"max_length":             30,
			"send_path":              "?to={{to}}&text={{text}}&from={{from}}{{quick_replies}}",
			courier.ConfigSendMethod: http.MethodGet})

	var getChannel30StrLength = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]any{
			"max_length":             "30",
			"send_path":              "?to={{to}}&text={{text}}&from={{from}}{{quick_replies}}",
			courier.ConfigSendMethod: http.MethodGet})

	var jsonChannel30IntLength = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]any{
			"send_path":               "",
			"max_length":              30,
			courier.ConfigSendBody:    `{ "to":{{to}}, "text":{{text}}, "from":{{from}}, "quick_replies":{{quick_replies}} }`,
			courier.ConfigContentType: contentJSON,
			courier.ConfigSendMethod:  http.MethodPost,
			courier.ConfigSendHeaders: map[string]any{"Authorization": "Token ABCDEF", "foo": "bar"},
		})

	var xmlChannel30IntLength = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]any{
			"send_path":               "",
			"max_length":              30,
			courier.ConfigSendBody:    `<msg><to>{{to}}</to><text>{{text}}</text><from>{{from}}</from><quick_replies>{{quick_replies}}</quick_replies></msg>`,
			courier.ConfigContentType: contentXML,
			courier.ConfigSendMethod:  http.MethodPost,
			courier.ConfigSendHeaders: map[string]any{"Authorization": "Token ABCDEF", "foo": "bar"},
		})

	RunOutgoingTestCases(t, getChannel30IntLength, newHandler(), longSendTestCases, nil, nil)
	RunOutgoingTestCases(t, getChannel30StrLength, newHandler(), longSendTestCases, nil, nil)
	RunOutgoingTestCases(t, jsonChannel30IntLength, newHandler(), jsonLongSendTestCases, nil, nil)
	RunOutgoingTestCases(t, xmlChannel30IntLength, newHandler(), xmlLongSendTestCases, nil, nil)

	var nationalChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]any{
			"send_path":              "?to={{to}}&text={{text}}&from={{from}}{{quick_replies}}",
			"use_national":           true,
			courier.ConfigSendMethod: http.MethodGet})

	RunOutgoingTestCases(t, nationalChannel, newHandler(), nationalGetSendTestCases, nil, nil)

	var jsonChannelWithSendAuthorization = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]any{
			"send_path":                     "",
			courier.ConfigSendBody:          `{ "to":{{to}}, "text":{{text}}, "from":{{from}}, "quick_replies":{{quick_replies}} }`,
			courier.ConfigContentType:       contentJSON,
			courier.ConfigSendMethod:        http.MethodPost,
			courier.ConfigSendAuthorization: "Token ABCDEF",
		})
	RunOutgoingTestCases(t, jsonChannelWithSendAuthorization, newHandler(), jsonSendTestCases, []string{"Token ABCDEF"}, nil)

}
