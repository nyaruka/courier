package start

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ST", "2020", "UA", map[string]interface{}{"username": "st-username", "password": "st-password"}),
}

var (
	receiveURL = "/c/st/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"

	notXML = "empty"

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

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: validReceive, Status: 200, Response: "<state>Accepted</state>",
		Text: Sp("Hello World"), URN: Sp("tel:+250788123123"), Date: Tp(time.Date(2015, 12, 18, 15, 02, 54, 0, time.UTC))},
	{Label: "Receive Valid Encoded", URL: receiveURL, Data: validReceiveEncoded, Status: 200, Response: "<state>Accepted</state>",
		Text: Sp("Кохання"), URN: Sp("tel:+380501529999"), Date: Tp(time.Date(2015, 12, 18, 15, 02, 54, 0, time.UTC))},
	{Label: "Receive Valid with empty Text", URL: receiveURL, Data: validReceiveEmptyText, Status: 200, Response: "<state>Accepted</state>",
		Text: Sp(""), URN: Sp("tel:+250788123123")},
	{Label: "Receive Valid missing body", URL: receiveURL, Data: validMissingBody, Status: 200, Response: "<state>Accepted</state>",
		Text: Sp(""), URN: Sp("tel:+250788123123")},
	{Label: "Receive invalidURN", URL: receiveURL, Data: invalidURNReceive, Status: 400, Response: "phone number supplied is not a number"},
	{Label: "Receive missing Request ID", URL: receiveURL, Data: missingRequestID, Status: 400, Response: "Error"},
	{Label: "Receive missing From", URL: receiveURL, Data: missingFrom, Status: 400, Response: "Error"},
	{Label: "Receive missing To", URL: receiveURL, Data: missingTo, Status: 400, Response: "Error"},
	{Label: "Invalid XML", URL: receiveURL, Data: notXML, Status: 400, Response: "Error"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

// setSendURL takes care of setting the sendURL to call
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text:           "Simple Message ☺",
		URN:            "tel:+250788383383",
		Status:         "W",
		ExternalID:     "380502535130309161501",
		ResponseBody:   `<status date='Wed, 25 May 2016 17:29:56 +0300'><id>380502535130309161501</id><state>Accepted</state></status>`,
		ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/xml; charset=utf8",
			"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ=",
		},
		RequestBody: `<message><service id="single" source="2020" validity="+12 hours"></service><to>+250788383383</to><body content-type="plain/text" encoding="plain">Simple Message ☺</body></message>`,
		SendPrep:    setSendURL},
	{Label: "Long Send",
		Text:           "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		URN:            "tel:+250788383383",
		Status:         "W",
		ExternalID:     "380502535130309161501",
		ResponseBody:   `<status date='Wed, 25 May 2016 17:29:56 +0300'><id>380502535130309161501</id><state>Accepted</state></status>`,
		ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/xml; charset=utf8",
			"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ=",
		},
		RequestBody: `<message><service id="single" source="2020" validity="+12 hours"></service><to>+250788383383</to><body content-type="plain/text" encoding="plain">I need to keep adding more things to make it work</body></message>`,
		SendPrep:    setSendURL},
	{Label: "Send Attachment",
		Text:           "My pic!",
		Attachments:    []string{"image/jpeg:https://foo.bar/image.jpg"},
		URN:            "tel:+250788383383",
		Status:         "W",
		ExternalID:     "380502535130309161501",
		ResponseBody:   `<status date='Wed, 25 May 2016 17:29:56 +0300'><id>380502535130309161501</id><state>Accepted</state></status>`,
		ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/xml; charset=utf8",
			"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ=",
		},
		RequestBody: `<message><service id="single" source="2020" validity="+12 hours"></service><to>+250788383383</to><body content-type="plain/text" encoding="plain">My pic!&#xA;https://foo.bar/image.jpg</body></message>`,
		SendPrep:    setSendURL},
	{Label: "Error Response",
		Text:           "Simple Message ☺",
		URN:            "tel:+250788383383",
		Status:         "E",
		ExternalID:     "",
		ResponseBody:   `<error>This is an error</error>`,
		ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/xml; charset=utf8",
			"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ=",
		},
		RequestBody: `<message><service id="single" source="2020" validity="+12 hours"></service><to>+250788383383</to><body content-type="plain/text" encoding="plain">Simple Message ☺</body></message>`,
		SendPrep:    setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `Error`, ResponseStatus: 401,
		Headers: map[string]string{
			"Content-Type":  "application/xml; charset=utf8",
			"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ=",
		},
		RequestBody: `<message><service id="single" source="2020" validity="+12 hours"></service><to>+250788383383</to><body content-type="plain/text" encoding="plain">Error Message</body></message>`,
		SendPrep:    setSendURL},
}

func TestSending(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ST", "2020", "UA", map[string]interface{}{"username": "Username", "password": "Password"})
	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
