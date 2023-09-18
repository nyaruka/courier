package start

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ST", "2020", "UA", map[string]any{"username": "st-username", "password": "st-password"}),
}

const (
	receiveURL = "/c/st/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"

	validReceive = `<message>
	<service type="sms" timestamp="1450450974" auth="asdfasdf" request_id="msg1"/>
	<from>+250788123123</from>
	<to>1515</to>
	<body content-type="content-type" encoding="utf8">Hello World</body>
	</message>`

	invalidURNReceive = `<message>
	<service type="sms" timestamp="1450450974" auth="asdfasdf" request_id="msg1"/>
	<from>MTN</from>
	<to>1515</to>
	<body content-type="content-type" encoding="utf8">Hello World</body>
	</message>`

	validReceiveEncoded = "<message>" +
		"<service type='sms' timestamp='1450450974' auth='10c5cfa4d8111e8523681fdbacd32d0b' request_id='43473486'/>" +
		"<from>380501529999</from>" +
		"<to>4224</to>" +
		"<body>\xD0\x9A\xD0\xBE\xD1\x85\xD0\xB0\xD0\xBD\xD0\xBD\xD1\x8F</body>" +
		"</message>`"

	validReceiveEmptyText = `<message>
	<service type="sms" timestamp="1450450974" auth="asdfasdf" request_id="msg1"/>
	<from>+250788123123</from>
	<to>1515</to>
	<body content-type="content-type" encoding="utf8"></body>
	</message>`

	missingRequestID = `<message>
	<service type="sms" timestamp="1450450974" auth="asdfasdf" />
	<from>+250788123123</from>
	<to>1515</to>
	<body content-type="content-type" encoding="utf8">Hello World</body>
	</message>`

	missingFrom = `<message>
	<service type="sms" timestamp="1450450974" auth="asdfasdf" request_id="msg1"/>
	<to>1515</to>
	<body content-type="content-type" encoding="utf8">Hello World</body>
	</message>`

	missingTo = `<message>
	<service type="sms" timestamp="1450450974" auth="asdfasdf" request_id="msg1"/>
	<from>+250788123123</from>
	<body content-type="content-type" encoding="utf8">Hello World</body>
	</message>`

	validMissingBody = `<message>
	<service type="sms" timestamp="1450450974" auth="asdfasdf" request_id="msg1"/>
	<from>+250788123123</from>
	<to>1515</to>
	</message>`
)

var testCases = []IncomingTestCase{
	{
		Label:                "Receive Valid",
		URL:                  receiveURL,
		Data:                 validReceive,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "<state>Accepted</state>",
		ExpectedMsgText:      Sp("Hello World"),
		ExpectedURN:          "tel:+250788123123",
		ExpectedDate:         time.Date(2015, 12, 18, 15, 02, 54, 0, time.UTC),
		ExpectedExternalID:   "msg1",
	},
	{
		Label:                "Receive Valid Encoded",
		URL:                  receiveURL,
		Data:                 validReceiveEncoded,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "<state>Accepted</state>",
		ExpectedMsgText:      Sp("Кохання"),
		ExpectedURN:          "tel:+380501529999",
		ExpectedDate:         time.Date(2015, 12, 18, 15, 02, 54, 0, time.UTC),
		ExpectedExternalID:   "43473486",
	},
	{
		Label:                "Receive Valid with empty Text",
		URL:                  receiveURL,
		Data:                 validReceiveEmptyText,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "<state>Accepted</state>",
		ExpectedMsgText:      Sp(""),
		ExpectedURN:          "tel:+250788123123",
		ExpectedExternalID:   "msg1",
	},
	{
		Label:                "Receive Valid missing body",
		URL:                  receiveURL,
		Data:                 validMissingBody,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "<state>Accepted</state>",
		ExpectedMsgText:      Sp(""),
		ExpectedURN:          "tel:+250788123123",
	},
	{
		Label:                "Receive invalidURN",
		URL:                  receiveURL,
		Data:                 invalidURNReceive,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "phone number supplied is not a number",
	},
	{
		Label:                "Receive missing Request ID",
		URL:                  receiveURL,
		Data:                 missingRequestID,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "Error",
	},
	{
		Label:                "Receive missing From",
		URL:                  receiveURL,
		Data:                 missingFrom,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "Error",
	},
	{
		Label:                "Receive missing To",
		URL:                  receiveURL,
		Data:                 missingTo,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "Error",
	},
	{
		Label:                "Invalid XML",
		URL:                  receiveURL,
		Data:                 "empty",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "Error",
	},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

// setSendURL takes care of setting the sendURL to call
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	sendURL = s.URL
}

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:              "Plain Send",
		MsgText:            "Simple Message ☺",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `<status date='Wed, 25 May 2016 17:29:56 +0300'><id>380502535130309161501</id><state>Accepted</state></status>`,
		MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type":  "application/xml; charset=utf8",
			"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ=",
		},
		ExpectedRequestBody: `<message><service id="single" source="2020" validity="+12 hours"></service><to>+250788383383</to><body content-type="plain/text" encoding="plain">Simple Message ☺</body></message>`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "380502535130309161501",
		SendPrep:            setSendURL,
	},
	{
		Label:              "Long Send",
		MsgText:            "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `<status date='Wed, 25 May 2016 17:29:56 +0300'><id>380502535130309161501</id><state>Accepted</state></status>`,
		MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type":  "application/xml; charset=utf8",
			"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ=",
		},
		ExpectedRequestBody: `<message><service id="single" source="2020" validity="+12 hours"></service><to>+250788383383</to><body content-type="plain/text" encoding="plain">I need to keep adding more things to make it work</body></message>`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "380502535130309161501",
		SendPrep:            setSendURL,
	},
	{
		Label:              "Send Attachment",
		MsgText:            "My pic!",
		MsgAttachments:     []string{"image/jpeg:https://foo.bar/image.jpg"},
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `<status date='Wed, 25 May 2016 17:29:56 +0300'><id>380502535130309161501</id><state>Accepted</state></status>`,
		MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type":  "application/xml; charset=utf8",
			"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ=",
		},
		ExpectedRequestBody: `<message><service id="single" source="2020" validity="+12 hours"></service><to>+250788383383</to><body content-type="plain/text" encoding="plain">My pic!&#xA;https://foo.bar/image.jpg</body></message>`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "380502535130309161501",
		SendPrep:            setSendURL,
	},
	{
		Label:              "Error Response",
		MsgText:            "Simple Message ☺",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `<error>This is an error</error>`,
		MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type":  "application/xml; charset=utf8",
			"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ=",
		},
		ExpectedRequestBody: `<message><service id="single" source="2020" validity="+12 hours"></service><to>+250788383383</to><body content-type="plain/text" encoding="plain">Simple Message ☺</body></message>`,
		ExpectedMsgStatus:   "E",
		SendPrep:            setSendURL,
	},
	{
		Label:              "Error Sending",
		MsgText:            "Error Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `Error`,
		MockResponseStatus: 401,
		ExpectedHeaders: map[string]string{
			"Content-Type":  "application/xml; charset=utf8",
			"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ=",
		},
		ExpectedRequestBody: `<message><service id="single" source="2020" validity="+12 hours"></service><to>+250788383383</to><body content-type="plain/text" encoding="plain">Error Message</body></message>`,
		ExpectedMsgStatus:   "E",
		SendPrep:            setSendURL,
	},
}

func TestOutgoing(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ST", "2020", "UA", map[string]any{"username": "Username", "password": "Password"})
	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{httpx.BasicAuth("Username", "Password")}, nil)
}
