package external

import (
	"net/http/httptest"
	"testing"
	"time"

	"net/http"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/courier/utils"
)

var (
	receiveValidMessage         = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?sender=%2B2349067554729&text=Join"
	receiveValidMessageFrom     = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?from=%2B2349067554729&text=Join"
	receiveValidNoPlus          = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?from=2349067554729&text=Join"
	receiveValidMessageWithDate = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?sender=%2B2349067554729&text=Join&date=2017-06-23T12:30:00.500Z"
	receiveValidMessageWithTime = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?sender=%2B2349067554729&text=Join&time=2017-06-23T12:30:00Z"
	receiveNoParams             = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	invalidURN                  = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?sender=MTN&text=Join"
	receiveNoSender             = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?text=Join"
	receiveInvalidDate          = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?sender=%2B2349067554729&text=Join&time=20170623T123000Z"
	failedNoParams              = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/failed/"
	failedValid                 = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/failed/?id=12345"
	sentValid                   = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/sent/?id=12345"
	invalidStatus               = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/wired/"
	deliveredValid              = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered/?id=12345"
	deliveredValidPost          = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered/"
	stoppedEvent                = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/stopped/?from=%2B2349067554729"
	stoppedEventPost            = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/stopped/"
	stoppedEventInvalidURN      = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/stopped/?from=MTN"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US", nil),
}

var gmChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "GM", nil),
}

var handleTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveValidMessage, Data: "empty", ExpectedStatus: 200, ExpectedResponse: "Accepted",
		ExpectedMsgText: Sp("Join"), ExpectedURN: Sp("tel:+2349067554729")},
	{Label: "Receive Valid Post", URL: receiveNoParams, Data: "sender=%2B2349067554729&text=Join", ExpectedStatus: 200, ExpectedResponse: "Accepted",
		ExpectedMsgText: Sp("Join"), ExpectedURN: Sp("tel:+2349067554729")},

	{Label: "Receive Valid Post multipart form", URL: receiveNoParams, MultipartFormFields: map[string]string{"sender": "2349067554729", "text": "Join"}, ExpectedStatus: 200, ExpectedResponse: "Accepted",
		ExpectedMsgText: Sp("Join"), ExpectedURN: Sp("tel:+2349067554729")},
	{Label: "Receive Valid From", URL: receiveValidMessageFrom, Data: "empty", ExpectedStatus: 200, ExpectedResponse: "Accepted",
		ExpectedMsgText: Sp("Join"), ExpectedURN: Sp("tel:+2349067554729")},
	{Label: "Receive Country Parse", URL: receiveValidNoPlus, Data: "empty", ExpectedStatus: 200, ExpectedResponse: "Accepted",
		ExpectedMsgText: Sp("Join"), ExpectedURN: Sp("tel:+2349067554729")},
	{Label: "Receive Valid Message With Date", URL: receiveValidMessageWithDate, Data: "empty", ExpectedStatus: 200, ExpectedResponse: "Accepted",
		ExpectedMsgText: Sp("Join"), ExpectedURN: Sp("tel:+2349067554729"), ExpectedDate: time.Date(2017, 6, 23, 12, 30, 0, int(500*time.Millisecond), time.UTC)},
	{Label: "Receive Valid Message With Time", URL: receiveValidMessageWithTime, Data: "empty", ExpectedStatus: 200, ExpectedResponse: "Accepted",
		ExpectedMsgText: Sp("Join"), ExpectedURN: Sp("tel:+2349067554729"), ExpectedDate: time.Date(2017, 6, 23, 12, 30, 0, 0, time.UTC)},
	{Label: "Invalid URN", URL: invalidURN, Data: "empty", ExpectedStatus: 400, ExpectedResponse: "phone number supplied is not a number"},
	{Label: "Receive No Params", URL: receiveNoParams, Data: "empty", ExpectedStatus: 400, ExpectedResponse: "must have one of 'sender' or 'from' set"},
	{Label: "Receive No Sender", URL: receiveNoSender, Data: "empty", ExpectedStatus: 400, ExpectedResponse: "must have one of 'sender' or 'from' set"},
	{Label: "Receive Invalid Date", URL: receiveInvalidDate, Data: "empty", ExpectedStatus: 400, ExpectedResponse: "invalid date format, must be RFC 3339"},
	{Label: "Failed No Params", URL: failedNoParams, ExpectedStatus: 400, ExpectedResponse: "field 'id' required"},
	{Label: "Failed Valid", URL: failedValid, ExpectedStatus: 200, ExpectedResponse: `"status":"F"`},
	{Label: "Invalid Status", URL: invalidStatus, ExpectedStatus: 404, ExpectedResponse: `page not found`},
	{Label: "Sent Valid", URL: sentValid, ExpectedStatus: 200, ExpectedResponse: `"status":"S"`},
	{Label: "Delivered Valid", URL: deliveredValid, ExpectedStatus: 200, Data: "nothing", ExpectedResponse: `"status":"D"`},
	{Label: "Delivered Valid Post", URL: deliveredValidPost, Data: "id=12345", ExpectedStatus: 200, ExpectedResponse: `"status":"D"`},
	{Label: "Stopped Event", URL: stoppedEvent, ExpectedStatus: 200, Data: "nothing", ExpectedResponse: "Accepted"},
	{Label: "Stopped Event Post", URL: stoppedEventPost, Data: "from=%2B2349067554729", ExpectedStatus: 200, ExpectedResponse: "Accepted"},
	{Label: "Stopped Event Invalid URN", URL: stoppedEventInvalidURN, Data: "empty", ExpectedStatus: 400, ExpectedResponse: "phone number supplied is not a number"},
	{Label: "Stopped event No Params", URL: stoppedEventPost, ExpectedStatus: 400, ExpectedResponse: "field 'from' required"},
}

var testSOAPReceiveChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]interface{}{
			configTextXPath:             "//content",
			configFromXPath:             "//source",
			configMOResponse:            "<?xml version=“1.0”?><return>0</return>",
			configMOResponseContentType: "text/xml",
		})}

var handleSOAPReceiveTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Post SOAP", URL: receiveNoParams, Data: `<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:com="com.hero"><soapenv:Header/><soapenv:Body><com:moRequest><source>2349067554729</source><content>Join</content></com:moRequest></soapenv:Body></soapenv:Envelope>`,
		ExpectedStatus: 200, ExpectedResponse: "<?xml version=“1.0”?><return>0</return>",
		ExpectedMsgText: Sp("Join"), ExpectedURN: Sp("tel:+2349067554729")},
	{Label: "Receive Invalid SOAP", URL: receiveNoParams, Data: `<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:com="com.hero"><soapenv:Header/><soapenv:Body></soapenv:Body></soapenv:Envelope>`,
		ExpectedStatus: 400, ExpectedResponse: "missing from"},
}

var gmTestCases = []ChannelHandleTestCase{
	{Label: "Receive Non Plus Message", URL: "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?sender=2207222333&text=Join", Data: "empty", ExpectedStatus: 200, ExpectedResponse: "Accepted",
		ExpectedMsgText: Sp("Join"), ExpectedURN: Sp("tel:+2207222333")},
}

var customChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]interface{}{
			configMOFromField: "from_number",
			configMODateField: "timestamp",
			configMOTextField: "messageText",
		})}

var customTestCases = []ChannelHandleTestCase{
	{Label: "Receive Custom Message", URL: "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?from_number=12067799192&messageText=Join&timestamp=2017-06-23T12:30:00Z", Data: "empty", ExpectedStatus: 200, ExpectedResponse: "Accepted",
		ExpectedMsgText: Sp("Join"), ExpectedURN: Sp("tel:+12067799192"), ExpectedDate: time.Date(2017, 6, 23, 12, 30, 0, 0, time.UTC)},
	{Label: "Receive Custom Missing", URL: "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?sent_from=12067799192&messageText=Join", Data: "empty", ExpectedStatus: 400, ExpectedResponse: "must have one of 'sender' or 'from' set"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), handleTestCases)
	RunChannelTestCases(t, testSOAPReceiveChannels, newHandler(), handleSOAPReceiveTestCases)
	RunChannelTestCases(t, gmChannels, newHandler(), gmTestCases)
	RunChannelTestCases(t, customChannels, newHandler(), customTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), handleTestCases)
	RunChannelBenchmarks(b, testSOAPReceiveChannels, newHandler(), handleSOAPReceiveTestCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	// this is actually a path, which we'll combine with the test server URL
	sendURL := c.StringConfigForKey("send_path", "")
	sendURL, _ = utils.AddURLPath(s.URL, sendURL)
	c.(*test.MockChannel).SetConfig(courier.ConfigSendURL, sendURL)
}

var longSendTestCases = []ChannelSendTestCase{
	{
		Label:   "Long Send",
		MsgText: "This is a long message that will be longer than 30....... characters", MsgURN: "tel:+250788383383",
		MsgQuickReplies:    []string{"One"},
		MockResponseBody:   "0: Accepted for delivery",
		MockResponseStatus: 200,
		ExpectedURLParams:  map[string]string{"text": "characters", "to": "+250788383383", "from": "2020", "quick_reply": "One"},
		ExpectedStatus:     "W",
		SendPrep:           setSendURL,
	},
}

var getSendSmartEncodingTestCases = []ChannelSendTestCase{
	{
		Label:              "Smart Encoding",
		MsgText:            "Fancy “Smart” Quotes",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   "0: Accepted for delivery",
		MockResponseStatus: 200,
		ExpectedURLParams:  map[string]string{"text": `Fancy "Smart" Quotes`, "to": "+250788383383", "from": "2020"},
		ExpectedHeaders:    map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		ExpectedStatus:     "W",
		SendPrep:           setSendURL,
	},
}

var postSendSmartEncodingTestCases = []ChannelSendTestCase{
	{
		Label:              "Smart Encoding",
		MsgText:            "Fancy “Smart” Quotes",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   "0: Accepted for delivery",
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"text": `Fancy "Smart" Quotes`, "to": "+250788383383", "from": "2020"},
		ExpectedHeaders:    map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		ExpectedStatus:     "W",
		SendPrep:           setSendURL,
	},
}

var getSendTestCases = []ChannelSendTestCase{
	{
		Label:              "Plain Send",
		MsgText:            "Simple Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   "0: Accepted for delivery",
		MockResponseStatus: 200,
		ExpectedURLParams:  map[string]string{"text": "Simple Message", "to": "+250788383383", "from": "2020"},
		ExpectedHeaders:    map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		ExpectedStatus:     "W",
		SendPrep:           setSendURL,
	},
	{
		Label:   "Unicode Send",
		MsgText: "☺", MsgURN: "tel:+250788383383",
		MockResponseBody:   "0: Accepted for delivery",
		MockResponseStatus: 200,
		ExpectedURLParams:  map[string]string{"text": "☺", "to": "+250788383383", "from": "2020"},
		ExpectedHeaders:    map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		ExpectedStatus:     "W",
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
		ExpectedStatus:     "E",
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
		ExpectedStatus:     "W",
		SendPrep:           setSendURL,
	},
}

var postSendTestCases = []ChannelSendTestCase{
	{
		Label:              "Plain Send",
		MsgText:            "Simple Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   "0: Accepted for delivery",
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"text": "Simple Message", "to": "+250788383383", "from": "2020"},
		ExpectedHeaders:    map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		ExpectedStatus:     "W",
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
		ExpectedStatus:     "W",
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
		ExpectedStatus:     "E",
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
		ExpectedStatus:     "W",
		SendPrep:           setSendURL,
	},
}

var postSendCustomContentTypeTestCases = []ChannelSendTestCase{
	{
		Label:              "Plain Send",
		MsgText:            "Simple Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   "0: Accepted for delivery",
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"text": "Simple Message", "to": "250788383383", "from": "2020"},
		ExpectedHeaders:    map[string]string{"Content-Type": "application/x-www-form-urlencoded; charset=utf-8"},
		ExpectedStatus:     "W",
		SendPrep:           setSendURL,
	},
}

var jsonSendTestCases = []ChannelSendTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message",
		MsgURN:              "tel:+250788383383",
		MockResponseBody:    "0: Accepted for delivery",
		MockResponseStatus:  200,
		ExpectedRequestBody: `{ "to":"+250788383383", "text":"Simple Message", "from":"2020", "quick_replies":[] }`,
		ExpectedHeaders:     map[string]string{"Authorization": "Token ABCDEF", "Content-Type": "application/json"},
		ExpectedStatus:      "W",
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
		ExpectedStatus:      "W",
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
		ExpectedStatus:      "E",
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
		ExpectedStatus:      "W",
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
		ExpectedStatus:      "W",
		SendPrep:            setSendURL,
	},
}

var jsonLongSendTestCases = []ChannelSendTestCase{
	{
		Label:               "Send Quick Replies",
		MsgText:             "This is a long message that will be longer than 30....... characters",
		MsgURN:              "tel:+250788383383",
		MsgQuickReplies:     []string{"One", "Two", "Three"},
		MockResponseBody:    "0: Accepted for delivery",
		MockResponseStatus:  200,
		ExpectedRequestBody: `{ "to":"+250788383383", "text":"characters", "from":"2020", "quick_replies":["One","Two","Three"] }`,
		ExpectedHeaders:     map[string]string{"Authorization": "Token ABCDEF", "Content-Type": "application/json"},
		ExpectedStatus:      "W",
		SendPrep:            setSendURL,
	},
}

var xmlSendTestCases = []ChannelSendTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message",
		MsgURN:              "tel:+250788383383",
		MockResponseBody:    "0: Accepted for delivery",
		MockResponseStatus:  200,
		ExpectedRequestBody: `<msg><to>+250788383383</to><text>Simple Message</text><from>2020</from><quick_replies></quick_replies></msg>`,
		ExpectedHeaders:     map[string]string{"Content-Type": "text/xml; charset=utf-8"},
		ExpectedStatus:      "W",
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
		ExpectedStatus:      "W",
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
		ExpectedStatus:      "E",
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
		ExpectedStatus:      "W",
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
		ExpectedStatus:      "W",
		SendPrep:            setSendURL,
	},
}

var xmlLongSendTestCases = []ChannelSendTestCase{
	{
		Label:               "Send Quick Replies",
		MsgText:             "This is a long message that will be longer than 30....... characters",
		MsgURN:              "tel:+250788383383",
		MsgQuickReplies:     []string{"One", "Two", "Three"},
		MockResponseBody:    "0: Accepted for delivery",
		MockResponseStatus:  200,
		ExpectedRequestBody: "<msg><to>+250788383383</to><text>characters</text><from>2020</from><quick_replies><item>One</item><item>Two</item><item>Three</item></quick_replies></msg>",
		ExpectedHeaders:     map[string]string{"Content-Type": "text/xml; charset=utf-8"},
		ExpectedStatus:      "W",
		SendPrep:            setSendURL,
	},
}

var xmlSendWithResponseContentTestCases = []ChannelSendTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message",
		MsgURN:              "tel:+250788383383",
		ExpectedStatus:      "W",
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
		ExpectedStatus:      "W",
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
		ExpectedStatus:      "E",
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
		ExpectedStatus:      "E",
		ExpectedErrors:      []string{"Received invalid response content: <return>1</return>"},
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
		ExpectedStatus:      "W",
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
		ExpectedStatus:      "W",
		SendPrep:            setSendURL,
	},
}

var nationalGetSendTestCases = []ChannelSendTestCase{
	{
		Label:              "Plain Send",
		MsgText:            "Simple Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   "0: Accepted for delivery",
		MockResponseStatus: 200,
		ExpectedURLParams:  map[string]string{"text": "Simple Message", "to": "788383383", "from": "2020"},
		ExpectedHeaders:    map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		ExpectedStatus:     "W",
		SendPrep:           setSendURL,
	},
}

func TestSending(t *testing.T) {
	var getChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]interface{}{
			"send_path":              "?to={{to}}&text={{text}}&from={{from}}{{quick_replies}}",
			courier.ConfigSendMethod: http.MethodGet})

	var getSmartChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]interface{}{
			"send_path":              "?to={{to}}&text={{text}}&from={{from}}{{quick_replies}}",
			configEncoding:           encodingSmart,
			courier.ConfigSendMethod: http.MethodGet})

	var postChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]interface{}{
			"send_path":              "",
			courier.ConfigSendBody:   "to={{to}}&text={{text}}&from={{from}}{{quick_replies}}",
			courier.ConfigSendMethod: http.MethodPost})

	var postChannelCustomContentType = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]interface{}{
			"send_path":               "",
			courier.ConfigSendBody:    "to={{to_no_plus}}&text={{text}}&from={{from_no_plus}}{{quick_replies}}",
			courier.ConfigContentType: "application/x-www-form-urlencoded; charset=utf-8",
			courier.ConfigSendMethod:  http.MethodPost})

	var postSmartChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]interface{}{
			"send_path":              "",
			courier.ConfigSendBody:   "to={{to}}&text={{text}}&from={{from}}{{quick_replies}}",
			configEncoding:           encodingSmart,
			courier.ConfigSendMethod: http.MethodPost})

	var jsonChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]interface{}{
			"send_path":               "",
			courier.ConfigSendBody:    `{ "to":{{to}}, "text":{{text}}, "from":{{from}}, "quick_replies":{{quick_replies}} }`,
			courier.ConfigContentType: contentJSON,
			courier.ConfigSendMethod:  http.MethodPost,
			courier.ConfigSendHeaders: map[string]interface{}{"Authorization": "Token ABCDEF", "foo": "bar"},
		})

	var xmlChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]interface{}{
			"send_path":               "",
			courier.ConfigSendBody:    `<msg><to>{{to}}</to><text>{{text}}</text><from>{{from}}</from><quick_replies>{{quick_replies}}</quick_replies></msg>`,
			courier.ConfigContentType: contentXML,
			courier.ConfigSendMethod:  http.MethodPut,
		})

	var xmlChannelWithResponseContent = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]interface{}{
			"send_path":               "",
			courier.ConfigSendBody:    `<msg><to>{{to}}</to><text>{{text}}</text><from>{{from}}</from><quick_replies>{{quick_replies}}</quick_replies></msg>`,
			configMTResponseCheck:     "<return>0</return>",
			courier.ConfigContentType: contentXML,
			courier.ConfigSendMethod:  http.MethodPut,
		})

	RunChannelSendTestCases(t, getChannel, newHandler(), getSendTestCases, nil)
	RunChannelSendTestCases(t, getSmartChannel, newHandler(), getSendTestCases, nil)
	RunChannelSendTestCases(t, getSmartChannel, newHandler(), getSendSmartEncodingTestCases, nil)
	RunChannelSendTestCases(t, postChannel, newHandler(), postSendTestCases, nil)
	RunChannelSendTestCases(t, postChannelCustomContentType, newHandler(), postSendCustomContentTypeTestCases, nil)
	RunChannelSendTestCases(t, postSmartChannel, newHandler(), postSendTestCases, nil)
	RunChannelSendTestCases(t, postSmartChannel, newHandler(), postSendSmartEncodingTestCases, nil)
	RunChannelSendTestCases(t, jsonChannel, newHandler(), jsonSendTestCases, nil)
	RunChannelSendTestCases(t, xmlChannel, newHandler(), xmlSendTestCases, nil)
	RunChannelSendTestCases(t, xmlChannelWithResponseContent, newHandler(), xmlSendWithResponseContentTestCases, nil)

	var getChannel30IntLength = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]interface{}{
			"max_length":             30,
			"send_path":              "?to={{to}}&text={{text}}&from={{from}}{{quick_replies}}",
			courier.ConfigSendMethod: http.MethodGet})

	var getChannel30StrLength = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]interface{}{
			"max_length":             "30",
			"send_path":              "?to={{to}}&text={{text}}&from={{from}}{{quick_replies}}",
			courier.ConfigSendMethod: http.MethodGet})

	var jsonChannel30IntLength = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]interface{}{
			"send_path":               "",
			"max_length":              30,
			courier.ConfigSendBody:    `{ "to":{{to}}, "text":{{text}}, "from":{{from}}, "quick_replies":{{quick_replies}} }`,
			courier.ConfigContentType: contentJSON,
			courier.ConfigSendMethod:  http.MethodPost,
			courier.ConfigSendHeaders: map[string]interface{}{"Authorization": "Token ABCDEF", "foo": "bar"},
		})

	var xmlChannel30IntLength = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]interface{}{
			"send_path":               "",
			"max_length":              30,
			courier.ConfigSendBody:    `<msg><to>{{to}}</to><text>{{text}}</text><from>{{from}}</from><quick_replies>{{quick_replies}}</quick_replies></msg>`,
			courier.ConfigContentType: contentXML,
			courier.ConfigSendMethod:  http.MethodPost,
			courier.ConfigSendHeaders: map[string]interface{}{"Authorization": "Token ABCDEF", "foo": "bar"},
		})

	RunChannelSendTestCases(t, getChannel30IntLength, newHandler(), longSendTestCases, nil)
	RunChannelSendTestCases(t, getChannel30StrLength, newHandler(), longSendTestCases, nil)
	RunChannelSendTestCases(t, jsonChannel30IntLength, newHandler(), jsonLongSendTestCases, nil)
	RunChannelSendTestCases(t, xmlChannel30IntLength, newHandler(), xmlLongSendTestCases, nil)

	var nationalChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]interface{}{
			"send_path":              "?to={{to}}&text={{text}}&from={{from}}{{quick_replies}}",
			"use_national":           true,
			courier.ConfigSendMethod: http.MethodGet})

	RunChannelSendTestCases(t, nationalChannel, newHandler(), nationalGetSendTestCases, nil)

	var jsonChannelWithSendAuthorization = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US",
		map[string]interface{}{
			"send_path":                     "",
			courier.ConfigSendBody:          `{ "to":{{to}}, "text":{{text}}, "from":{{from}}, "quick_replies":{{quick_replies}} }`,
			courier.ConfigContentType:       contentJSON,
			courier.ConfigSendMethod:        http.MethodPost,
			courier.ConfigSendAuthorization: "Token ABCDEF",
		})
	RunChannelSendTestCases(t, jsonChannelWithSendAuthorization, newHandler(), jsonSendTestCases, nil)

}
