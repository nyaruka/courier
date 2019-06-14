package twiml

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"fmt"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/urns"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "T", "2020", "US", map[string]interface{}{"auth_token": "6789"}),
}

var tmsTestChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TMS", "2020", "US", map[string]interface{}{"auth_token": "6789"}),
}

var twTestChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TW", "2020", "US", map[string]interface{}{"auth_token": "6789"}),
}

var swTestChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SW", "2020", "US", map[string]interface{}{"auth_token": "6789"}),
}

var (
	receiveURL         = "/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"
	statusURL          = "/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status"
	statusIDURL        = "/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=12345"
	statusInvalidIDURL = "/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=asdf"

	tmsReceiveURL         = "/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"
	tmsStatusURL          = "/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status"
	tmsStatusIDURL        = "/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=12345"
	tmsStatusInvalidIDURL = "/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=asdf"

	twReceiveURL         = "/c/tw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"
	twStatusURL          = "/c/tw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status"
	twStatusIDURL        = "/c/tw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=12345"
	twStatusInvalidIDURL = "/c/tw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=asdf"

	swReceiveURL         = "/c/sw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"
	swStatusURL          = "/c/sw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status"
	swStatusIDURL        = "/c/sw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=12345"
	swStatusInvalidIDURL = "/c/sw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=asdf"

	receiveValid        = "ToCountry=US&ToState=District+Of+Columbia&SmsMessageSid=SMe287d7109a5a925f182f0e07fe5b223b&NumMedia=0&ToCity=&FromZip=01022&SmsSid=SMe287d7109a5a925f182f0e07fe5b223b&FromState=MA&SmsStatus=received&FromCity=CHICOPEE&Body=Msg&FromCountry=US&To=%2B12028831111&ToZip=&NumSegments=1&MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&AccountSid=acctid&From=%2B14133881111&ApiVersion=2010-04-01"
	receiveMedia        = "ToCountry=US&ToState=District+Of+Columbia&SmsMessageSid=SMe287d7109a5a925f182f0e07fe5b223b&NumMedia=2&ToCity=&FromZip=01022&SmsSid=SMe287d7109a5a925f182f0e07fe5b223b&FromState=MA&SmsStatus=received&FromCity=CHICOPEE&FromCountry=US&To=%2B12028831111&ToZip=&NumSegments=1&MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&AccountSid=acctid&From=%2B14133881111&ApiVersion=2010-04-01&MediaUrl0=cat.jpg&MediaUrl1=dog.jpg"
	receiveMediaWithMsg = "ToCountry=US&ToState=District+Of+Columbia&SmsMessageSid=SMe287d7109a5a925f182f0e07fe5b223b&NumMedia=2&ToCity=&Body=Msg&FromZip=01022&SmsSid=SMe287d7109a5a925f182f0e07fe5b223b&FromState=MA&SmsStatus=received&FromCity=CHICOPEE&FromCountry=US&To=%2B12028831111&ToZip=&NumSegments=1&MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&AccountSid=acctid&From=%2B14133881111&ApiVersion=2010-04-01&MediaUrl0=cat.jpg&MediaUrl1=dog.jpg"
	receiveBase64       = "ToCountry=US&ToState=District+Of+Columbia&SmsMessageSid=SMe287d7109a5a925f182f0e07fe5b223b&NumMedia=0&ToCity=&FromZip=01022&SmsSid=SMe287d7109a5a925f182f0e07fe5b223b&FromState=MA&SmsStatus=received&FromCity=CHICOPEE&Body=QmFubm9uIEV4cGxhaW5zIFRoZSBXb3JsZCAuLi4K4oCcVGhlIENhbXAgb2YgdGhlIFNhaW50c%2BKA&FromCountry=US&To=%2B12028831111&ToZip=&NumSegments=1&MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&AccountSid=acctid&From=%2B14133881111&ApiVersion=2010-04-01"

	statusInvalid = "MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&MessageStatus=huh"
	statusValid   = "MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&MessageStatus=delivered"

	tmsStatusExtra  = "SmsStatus=sent&MessageStatus=sent&To=2021&MessagingServiceSid=MGdb23ec0f89ee2632e46e91d8128f5e2b&MessageSid=SM0b6e2697aae04182a9f5b5c7a8994c7f&AccountSid=acctid&From=%2B14133881111&ApiVersion=2010-04-01"
	tmsReceiveExtra = "ToCountry=US&ToState=&SmsMessageSid=SMbbf29aeb9d380ce2a1c0ae4635ff9dab&NumMedia=0&ToCity=&FromZip=27609&SmsSid=SMbbf29aeb9d380ce2a1c0ae4635ff9dab&FromState=NC&SmsStatus=received&FromCity=RALEIGH&Body=John+Cruz&FromCountry=US&To=384387&ToZip=&NumSegments=1&MessageSid=SMbbf29aeb9d380ce2a1c0ae4635ff9dab&AccountSid=acctid&From=%2B14133881111&ApiVersion=2010-04-01"

	waReceiveValid = "ToCountry=US&ToState=District+Of+Columbia&SmsMessageSid=SMe287d7109a5a925f182f0e07fe5b223b&NumMedia=0&ToCity=&FromZip=01022&SmsSid=SMe287d7109a5a925f182f0e07fe5b223b&FromState=MA&SmsStatus=received&FromCity=CHICOPEE&Body=Msg&FromCountry=US&To=whatsapp:%2B12028831111&ToZip=&NumSegments=1&MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&AccountSid=acctid&From=whatsapp:%2B14133881111&ApiVersion=2010-04-01"
)

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: receiveValid, Status: 200, Response: "<Response/>",
		Text: Sp("Msg"), URN: Sp("tel:+14133881111"), ExternalID: Sp("SMe287d7109a5a925f182f0e07fe5b223b"),
		PrepRequest: addValidSignature},
	{Label: "Receive Invalid Signature", URL: receiveURL, Data: receiveValid, Status: 400, Response: "invalid request signature",
		PrepRequest: addInvalidSignature},
	{Label: "Receive Missing Signature", URL: receiveURL, Data: receiveValid, Status: 400, Response: "missing request signature"},
	{Label: "Receive No Params", URL: receiveURL, Data: " ", Status: 400, Response: "field 'messagesid' required",
		PrepRequest: addValidSignature},
	{Label: "Receive Media", URL: receiveURL, Data: receiveMedia, Status: 200, Response: "<Response/>",
		URN: Sp("tel:+14133881111"), ExternalID: Sp("SMe287d7109a5a925f182f0e07fe5b223b"), Attachments: []string{"cat.jpg", "dog.jpg"},
		PrepRequest: addValidSignature},
	{Label: "Receive Media With Msg", URL: receiveURL, Data: receiveMediaWithMsg, Status: 200, Response: "<Response/>",
		Text: Sp("Msg"), URN: Sp("tel:+14133881111"), ExternalID: Sp("SMe287d7109a5a925f182f0e07fe5b223b"), Attachments: []string{"cat.jpg", "dog.jpg"},
		PrepRequest: addValidSignature},
	{Label: "Receive Base64", URL: receiveURL, Data: receiveBase64, Status: 200, Response: "<Response/>",
		Text: Sp("Bannon Explains The World ...\n“The Camp of the Saints"), URN: Sp("tel:+14133881111"), ExternalID: Sp("SMe287d7109a5a925f182f0e07fe5b223b"),
		PrepRequest: addValidSignature},
	{Label: "Status No Params", URL: statusURL, Data: " ", Status: 200, Response: "no msg status, ignoring",
		PrepRequest: addValidSignature},
	{Label: "Status Invalid Status", URL: statusURL, Data: statusInvalid, Status: 400, Response: "unknown status 'huh'",
		PrepRequest: addValidSignature},
	{Label: "Status Valid", URL: statusURL, Data: statusValid, Status: 200, Response: `"status":"D"`, ExternalID: Sp("SMe287d7109a5a925f182f0e07fe5b223b"),
		PrepRequest: addValidSignature},
	{Label: "Status ID Valid", URL: statusIDURL, Data: statusValid, Status: 200, Response: `"status":"D"`, ID: 12345,
		PrepRequest: addValidSignature},
	{Label: "Status ID Invalid", URL: statusInvalidIDURL, Data: statusValid, Status: 200, Response: `"status":"D"`, ExternalID: Sp("SMe287d7109a5a925f182f0e07fe5b223b"),
		PrepRequest: addValidSignature},
}

var tmsTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: tmsReceiveURL, Data: receiveValid, Status: 200, Response: "<Response/>",
		Text: Sp("Msg"), URN: Sp("tel:+14133881111"), ExternalID: Sp("SMe287d7109a5a925f182f0e07fe5b223b"),
		PrepRequest: addValidSignature},
	{Label: "Receive TMS extra", URL: tmsReceiveURL, Data: tmsReceiveExtra, Status: 200, Response: "<Response/>",
		Text: Sp("John Cruz"), URN: Sp("tel:+14133881111"), ExternalID: Sp("SMbbf29aeb9d380ce2a1c0ae4635ff9dab"),
		PrepRequest: addValidSignature},
	{Label: "Receive Invalid Signature", URL: tmsReceiveURL, Data: receiveValid, Status: 400, Response: "invalid request signature",
		PrepRequest: addInvalidSignature},
	{Label: "Receive Missing Signature", URL: tmsReceiveURL, Data: receiveValid, Status: 400, Response: "missing request signature"},
	{Label: "Receive No Params", URL: tmsReceiveURL, Data: " ", Status: 400, Response: "field 'messagesid' required",
		PrepRequest: addValidSignature},
	{Label: "Receive Media", URL: tmsReceiveURL, Data: receiveMedia, Status: 200, Response: "<Response/>",
		URN: Sp("tel:+14133881111"), ExternalID: Sp("SMe287d7109a5a925f182f0e07fe5b223b"), Attachments: []string{"cat.jpg", "dog.jpg"},
		PrepRequest: addValidSignature},
	{Label: "Receive Media With Msg", URL: tmsReceiveURL, Data: receiveMediaWithMsg, Status: 200, Response: "<Response/>",
		Text: Sp("Msg"), URN: Sp("tel:+14133881111"), ExternalID: Sp("SMe287d7109a5a925f182f0e07fe5b223b"), Attachments: []string{"cat.jpg", "dog.jpg"},
		PrepRequest: addValidSignature},
	{Label: "Receive Base64", URL: tmsReceiveURL, Data: receiveBase64, Status: 200, Response: "<Response/>",
		Text: Sp("Bannon Explains The World ...\n“The Camp of the Saints"), URN: Sp("tel:+14133881111"), ExternalID: Sp("SMe287d7109a5a925f182f0e07fe5b223b"),
		PrepRequest: addValidSignature},
	{Label: "Status TMS extra", URL: tmsStatusURL, Data: tmsStatusExtra, Status: 200, Response: `"status":"S"`,
		ExternalID: Sp("SM0b6e2697aae04182a9f5b5c7a8994c7f"), PrepRequest: addValidSignature},
	{Label: "Status No Params", URL: tmsStatusURL, Data: " ", Status: 200, Response: "no msg status, ignoring",
		PrepRequest: addValidSignature},
	{Label: "Status Invalid Status", URL: tmsStatusURL, Data: statusInvalid, Status: 400, Response: "unknown status 'huh'",
		PrepRequest: addValidSignature},
	{Label: "Status Valid", URL: tmsStatusURL, Data: statusValid, Status: 200, Response: `"status":"D"`, ExternalID: Sp("SMe287d7109a5a925f182f0e07fe5b223b"),
		PrepRequest: addValidSignature},
	{Label: "Status ID Valid", URL: tmsStatusIDURL, Data: statusValid, Status: 200, Response: `"status":"D"`, ID: 12345,
		PrepRequest: addValidSignature},
	{Label: "Status ID Invalid", URL: tmsStatusInvalidIDURL, Data: statusValid, Status: 200, Response: `"status":"D"`, ExternalID: Sp("SMe287d7109a5a925f182f0e07fe5b223b"),
		PrepRequest: addValidSignature},
}

var twTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: twReceiveURL, Data: receiveValid, Status: 200, Response: "<Response/>",
		Text: Sp("Msg"), URN: Sp("tel:+14133881111"), ExternalID: Sp("SMe287d7109a5a925f182f0e07fe5b223b"),
		PrepRequest: addValidSignature},
	{Label: "Receive Forwarded Valid", URL: twReceiveURL, Data: receiveValid,
		Headers: map[string]string{forwardedPathHeader: "/handlers/twilio/receive/8eb23e93-5ecb-45ba-b726-3b064e0c56ab"},
		Status:  200, Response: "<Response/>",
		Text: Sp("Msg"), URN: Sp("tel:+14133881111"), ExternalID: Sp("SMe287d7109a5a925f182f0e07fe5b223b"),
		PrepRequest: addForwardSignature},
	{Label: "Receive Invalid Signature", URL: twReceiveURL, Data: receiveValid, Status: 400, Response: "invalid request signature",
		PrepRequest: addInvalidSignature},
	{Label: "Receive Missing Signature", URL: twReceiveURL, Data: receiveValid, Status: 400, Response: "missing request signature"},
	{Label: "Receive No Params", URL: twReceiveURL, Data: " ", Status: 400, Response: "field 'messagesid' required",
		PrepRequest: addValidSignature},
	{Label: "Receive Media", URL: twReceiveURL, Data: receiveMedia, Status: 200, Response: "<Response/>",
		URN: Sp("tel:+14133881111"), ExternalID: Sp("SMe287d7109a5a925f182f0e07fe5b223b"), Attachments: []string{"cat.jpg", "dog.jpg"},
		PrepRequest: addValidSignature},
	{Label: "Receive Media With Msg", URL: twReceiveURL, Data: receiveMediaWithMsg, Status: 200, Response: "<Response/>",
		Text: Sp("Msg"), URN: Sp("tel:+14133881111"), ExternalID: Sp("SMe287d7109a5a925f182f0e07fe5b223b"), Attachments: []string{"cat.jpg", "dog.jpg"},
		PrepRequest: addValidSignature},
	{Label: "Receive Base64", URL: twReceiveURL, Data: receiveBase64, Status: 200, Response: "<Response/>",
		Text: Sp("Bannon Explains The World ...\n“The Camp of the Saints"), URN: Sp("tel:+14133881111"), ExternalID: Sp("SMe287d7109a5a925f182f0e07fe5b223b"),
		PrepRequest: addValidSignature},
	{Label: "Status No Params", URL: twStatusURL, Data: " ", Status: 200, Response: "no msg status, ignoring",
		PrepRequest: addValidSignature},
	{Label: "Status Invalid Status", URL: twStatusURL, Data: statusInvalid, Status: 400, Response: "unknown status 'huh'",
		PrepRequest: addValidSignature},
	{Label: "Status Valid", URL: twStatusURL, Data: statusValid, Status: 200, Response: `"status":"D"`, ExternalID: Sp("SMe287d7109a5a925f182f0e07fe5b223b"),
		PrepRequest: addValidSignature},
	{Label: "Status ID Valid", URL: twStatusIDURL, Data: statusValid, Status: 200, Response: `"status":"D"`, ID: 12345,
		PrepRequest: addValidSignature},
	{Label: "Status ID Invalid", URL: twStatusInvalidIDURL, Data: statusValid, Status: 200, Response: `"status":"D"`, ExternalID: Sp("SMe287d7109a5a925f182f0e07fe5b223b"),
		PrepRequest: addValidSignature},
}

var swTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: swReceiveURL, Data: receiveValid, Status: 200, Response: "<Response/>",
		Text: Sp("Msg"), URN: Sp("tel:+14133881111"), ExternalID: Sp("SMe287d7109a5a925f182f0e07fe5b223b")},
	{Label: "Receive No Params", URL: swReceiveURL, Data: " ", Status: 400, Response: "field 'messagesid' required"},
	{Label: "Receive Media", URL: swReceiveURL, Data: receiveMedia, Status: 200, Response: "<Response/>",
		URN: Sp("tel:+14133881111"), ExternalID: Sp("SMe287d7109a5a925f182f0e07fe5b223b"), Attachments: []string{"cat.jpg", "dog.jpg"}},
	{Label: "Receive Media With Msg", URL: swReceiveURL, Data: receiveMediaWithMsg, Status: 200, Response: "<Response/>",
		Text: Sp("Msg"), URN: Sp("tel:+14133881111"), ExternalID: Sp("SMe287d7109a5a925f182f0e07fe5b223b"), Attachments: []string{"cat.jpg", "dog.jpg"}},
	{Label: "Receive Base64", URL: swReceiveURL, Data: receiveBase64, Status: 200, Response: "<Response/>",
		Text: Sp("Bannon Explains The World ...\n“The Camp of the Saints"), URN: Sp("tel:+14133881111"), ExternalID: Sp("SMe287d7109a5a925f182f0e07fe5b223b")},
	{Label: "Status No Params", URL: swStatusURL, Data: " ", Status: 200, Response: "no msg status, ignoring"},
	{Label: "Status Invalid Status", URL: swStatusURL, Data: statusInvalid, Status: 400, Response: "unknown status 'huh'"},
	{Label: "Status Valid", URL: swStatusURL, Data: statusValid, Status: 200, Response: `"status":"D"`, ExternalID: Sp("SMe287d7109a5a925f182f0e07fe5b223b")},
	{Label: "Status ID Valid", URL: swStatusIDURL, Data: statusValid, Status: 200, Response: `"status":"D"`, ID: 12345},
	{Label: "Status ID Invalid", URL: swStatusInvalidIDURL, Data: statusValid, Status: 200, Response: `"status":"D"`, ExternalID: Sp("SMe287d7109a5a925f182f0e07fe5b223b")},
}

var waTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: waReceiveValid, Status: 200, Response: "<Response/>",
		Text: Sp("Msg"), URN: Sp("whatsapp:14133881111"), ExternalID: Sp("SMe287d7109a5a925f182f0e07fe5b223b"),
		PrepRequest: addValidSignature},
}

func addValidSignature(r *http.Request) {
	r.ParseForm()
	sig, _ := twCalculateSignature(fmt.Sprintf("%s://%s%s", r.URL.Scheme, r.Host, r.URL.RequestURI()), r.PostForm, "6789")
	r.Header.Set(signatureHeader, string(sig))
}

func addForwardSignature(r *http.Request) {
	r.ParseForm()
	path := r.Header.Get(forwardedPathHeader)
	sig, _ := twCalculateSignature(fmt.Sprintf("%s://%s%s", r.URL.Scheme, r.Host, path), r.PostForm, "6789")
	r.Header.Set(signatureHeader, string(sig))
}

func addInvalidSignature(r *http.Request) {
	r.Header.Set(signatureHeader, "invalidsig")
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newTWIMLHandler("T", "Twilio", true), testCases)
	RunChannelTestCases(t, tmsTestChannels, newTWIMLHandler("TMS", "Twilio Messaging Service", true), tmsTestCases)
	RunChannelTestCases(t, twTestChannels, newTWIMLHandler("TW", "TwiML API", true), twTestCases)
	RunChannelTestCases(t, swTestChannels, newTWIMLHandler("SW", "SignalWire", false), swTestCases)

	waChannel := courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SW", "+12065551212", "US",
		map[string]interface{}{
			configAccountSID:        "accountSID",
			courier.ConfigAuthToken: "6789",
		},
	)
	waChannel.SetScheme(urns.WhatsAppScheme)
	RunChannelTestCases(t, []courier.Channel{waChannel}, newTWIMLHandler("T", "TwilioWhatsApp", true), waTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newTWIMLHandler("T", "Twilio", true), testCases)
	RunChannelBenchmarks(b, tmsTestChannels, newTWIMLHandler("TMS", "Twilio Messaging Service", true), tmsTestCases)
	RunChannelBenchmarks(b, twTestChannels, newTWIMLHandler("TW", "TwiML API", true), twTestCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	if c.ChannelType().String() == "TW" || c.ChannelType().String() == "SW" {
		c.(*courier.MockChannel).SetConfig("send_url", s.URL)
	} else {
		twilioBaseURL = s.URL
	}
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message ☺", URN: "tel:+250788383383",
		Status: "W", ExternalID: "1002",
		ResponseBody: `{ "sid": "1002" }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "Simple Message ☺", "To": "+250788383383", "From": "2020", "StatusCallback": "https://localhost/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"},
		Path:       "/2010-04-01/Accounts/accountSID/Messages.json",
		Headers:    map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
		SendPrep:   setSendURL},
	{Label: "Long Send",
		Text:   "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		URN:    "tel:+250788383383",
		Status: "W", ExternalID: "1002",
		ResponseBody: `{ "sid": "1002" }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "I need to keep adding more things to make it work", "To": "+250788383383", "From": "2020", "StatusCallback": "https://localhost/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"},
		Path:       "/2010-04-01/Accounts/accountSID/Messages.json",
		Headers:    map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
		SendPrep:   setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{ "error": "out of credits" }`, ResponseStatus: 401,
		PostParams: map[string]string{"Body": "Error Message", "To": "+250788383383", "From": "2020", "StatusCallback": "https://localhost/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"},
		SendPrep:   setSendURL},
	{Label: "Error Code",
		Text: "Error Code", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{ "code": 1001 }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "Error Code", "To": "+250788383383", "From": "2020", "StatusCallback": "https://localhost/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"},
		SendPrep:   setSendURL},
	{Label: "Stopped Contact Code",
		Text: "Stopped Contact", URN: "tel:+250788383383",
		Status:       "F",
		ResponseBody: `{ "code": 21610 }`, ResponseStatus: 400,
		PostParams: map[string]string{"Body": "Stopped Contact", "To": "+250788383383", "From": "2020", "StatusCallback": "https://localhost/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"},
		SendPrep:   setSendURL,
		Stopped:    true},
	{Label: "No SID",
		Text: "No SID", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{ }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "No SID", "To": "+250788383383", "From": "2020", "StatusCallback": "https://localhost/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"},
		SendPrep:   setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: `{ "sid": "1002" }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "My pic!", "To": "+250788383383", "MediaUrl": "https://foo.bar/image.jpg", "From": "2020", "StatusCallback": "https://localhost/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"},
		SendPrep:   setSendURL},
}

var tmsDefaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message ☺", URN: "tel:+250788383383",
		Status: "W", ExternalID: "1002",
		ResponseBody: `{ "sid": "1002" }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "Simple Message ☺", "To": "+250788383383", "MessagingServiceSid": "messageServiceSID", "StatusCallback": "https://localhost/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56cd/status?id=10&action=callback"},
		Path:       "/2010-04-01/Accounts/accountSID/Messages.json",
		Headers:    map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
		SendPrep:   setSendURL},
	{Label: "Long Send",
		Text:   "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		URN:    "tel:+250788383383",
		Status: "W", ExternalID: "1002",
		ResponseBody: `{ "sid": "1002" }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "I need to keep adding more things to make it work", "To": "+250788383383", "MessagingServiceSid": "messageServiceSID", "StatusCallback": "https://localhost/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56cd/status?id=10&action=callback"},
		Path:       "/2010-04-01/Accounts/accountSID/Messages.json",
		Headers:    map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
		SendPrep:   setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{ "error": "out of credits" }`, ResponseStatus: 401,
		PostParams: map[string]string{"Body": "Error Message", "To": "+250788383383", "MessagingServiceSid": "messageServiceSID", "StatusCallback": "https://localhost/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56cd/status?id=10&action=callback"},
		SendPrep:   setSendURL},
	{Label: "Error Code",
		Text: "Error Code", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{ "code": 1001 }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "Error Code", "To": "+250788383383", "MessagingServiceSid": "messageServiceSID", "StatusCallback": "https://localhost/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56cd/status?id=10&action=callback"},
		SendPrep:   setSendURL},
	{Label: "Stopped Contact Code",
		Text: "Stopped Contact", URN: "tel:+250788383383",
		Status:       "F",
		ResponseBody: `{ "code": 21610 }`, ResponseStatus: 400,
		PostParams: map[string]string{"Body": "Stopped Contact", "To": "+250788383383", "MessagingServiceSid": "messageServiceSID", "StatusCallback": "https://localhost/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56cd/status?id=10&action=callback"},
		SendPrep:   setSendURL,
		Stopped:    true},
	{Label: "No SID",
		Text: "No SID", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{ }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "No SID", "To": "+250788383383", "MessagingServiceSid": "messageServiceSID", "StatusCallback": "https://localhost/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56cd/status?id=10&action=callback"},
		SendPrep:   setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: `{ "sid": "1002" }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "My pic!", "To": "+250788383383", "MediaUrl": "https://foo.bar/image.jpg", "MessagingServiceSid": "messageServiceSID", "StatusCallback": "https://localhost/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56cd/status?id=10&action=callback"},
		SendPrep:   setSendURL},
}

var twDefaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message ☺", URN: "tel:+250788383383",
		Status: "W", ExternalID: "1002",
		ResponseBody: `{ "sid": "1002" }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "Simple Message ☺", "To": "+250788383383", "From": "2020", "StatusCallback": "https://localhost/c/tw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"},
		Path:       "/2010-04-01/Accounts/accountSID/Messages.json",
		Headers:    map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
		SendPrep:   setSendURL},
	{Label: "Long Send",
		Text:   "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		URN:    "tel:+250788383383",
		Status: "W", ExternalID: "1002",
		ResponseBody: `{ "sid": "1002" }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "I need to keep adding more things to make it work", "To": "+250788383383", "From": "2020", "StatusCallback": "https://localhost/c/tw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"},
		Path:       "/2010-04-01/Accounts/accountSID/Messages.json",
		Headers:    map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
		SendPrep:   setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{ "error": "out of credits" }`, ResponseStatus: 401,
		PostParams: map[string]string{"Body": "Error Message", "To": "+250788383383", "From": "2020", "StatusCallback": "https://localhost/c/tw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"},
		SendPrep:   setSendURL},
	{Label: "Error Code",
		Text: "Error Code", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{ "code": 1001 }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "Error Code", "To": "+250788383383", "From": "2020", "StatusCallback": "https://localhost/c/tw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"},
		SendPrep:   setSendURL},
	{Label: "Stopped Contact Code",
		Text: "Stopped Contact", URN: "tel:+250788383383",
		Status:       "F",
		ResponseBody: `{ "code": 21610 }`, ResponseStatus: 400,
		PostParams: map[string]string{"Body": "Stopped Contact", "To": "+250788383383", "From": "2020", "StatusCallback": "https://localhost/c/tw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"},
		SendPrep:   setSendURL,
		Stopped:    true},
	{Label: "No SID",
		Text: "No SID", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{ }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "No SID", "To": "+250788383383", "From": "2020", "StatusCallback": "https://localhost/c/tw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"},
		SendPrep:   setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: `{ "sid": "1002" }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "My pic!", "To": "+250788383383", "MediaUrl": "https://foo.bar/image.jpg", "From": "2020", "StatusCallback": "https://localhost/c/tw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"},
		SendPrep:   setSendURL},
}

var swSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message ☺", URN: "tel:+250788383383",
		Status: "W", ExternalID: "1002",
		ResponseBody: `{ "sid": "1002" }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "Simple Message ☺", "To": "+250788383383", "From": "2020", "StatusCallback": "https://localhost/c/sw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"},
		Path:       "/2010-04-01/Accounts/accountSID/Messages.json",
		Headers:    map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
		SendPrep:   setSendURL},
	{Label: "Long Send",
		Text:   "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		URN:    "tel:+250788383383",
		Status: "W", ExternalID: "1002",
		ResponseBody: `{ "sid": "1002" }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "I need to keep adding more things to make it work", "To": "+250788383383", "From": "2020", "StatusCallback": "https://localhost/c/sw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"},
		Path:       "/2010-04-01/Accounts/accountSID/Messages.json",
		Headers:    map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
		SendPrep:   setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{ "error": "out of credits" }`, ResponseStatus: 401,
		PostParams: map[string]string{"Body": "Error Message", "To": "+250788383383", "From": "2020", "StatusCallback": "https://localhost/c/sw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"},
		SendPrep:   setSendURL},
	{Label: "Error Code",
		Text: "Error Code", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{ "code": 1001 }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "Error Code", "To": "+250788383383", "From": "2020", "StatusCallback": "https://localhost/c/sw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"},
		SendPrep:   setSendURL},
	{Label: "Stopped Contact Code",
		Text: "Stopped Contact", URN: "tel:+250788383383",
		Status:       "F",
		ResponseBody: `{ "code": 21610 }`, ResponseStatus: 400,
		PostParams: map[string]string{"Body": "Stopped Contact", "To": "+250788383383", "From": "2020", "StatusCallback": "https://localhost/c/sw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"},
		SendPrep:   setSendURL,
		Stopped:    true},
	{Label: "No SID",
		Text: "No SID", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{ }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "No SID", "To": "+250788383383", "From": "2020", "StatusCallback": "https://localhost/c/sw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"},
		SendPrep:   setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: `{ "sid": "1002" }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "My pic!", "To": "+250788383383", "MediaUrl": "https://foo.bar/image.jpg", "From": "2020", "StatusCallback": "https://localhost/c/sw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"},
		SendPrep:   setSendURL},
}

var waSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message ☺", URN: "whatsapp:250788383383",
		Status: "W", ExternalID: "1002",
		ResponseBody: `{ "sid": "1002" }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "Simple Message ☺", "To": "whatsapp:+250788383383", "From": "whatsapp:+12065551212", "StatusCallback": "https://localhost/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"},
		Headers:    map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
		SendPrep:   setSendURL},
}

func TestSending(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "T", "2020", "US",
		map[string]interface{}{
			configAccountSID:        "accountSID",
			courier.ConfigAuthToken: "authToken"})

	var tmsDefaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56cd", "TMS", "2021", "US",
		map[string]interface{}{
			configMessagingServiceSID: "messageServiceSID",
			configAccountSID:          "accountSID",
			courier.ConfigAuthToken:   "authToken"})

	var twDefaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TW", "2020", "US",
		map[string]interface{}{
			configAccountSID:        "accountSID",
			courier.ConfigAuthToken: "authToken",
			configSendURL:           "SEND_URL",
		})

	var swChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SW", "2020", "US",
		map[string]interface{}{
			configAccountSID:        "accountSID",
			courier.ConfigAuthToken: "authToken",
			configSendURL:           "BASE_URL",
		})

	RunChannelSendTestCases(t, defaultChannel, newTWIMLHandler("T", "Twilio", true), defaultSendTestCases, nil)
	RunChannelSendTestCases(t, tmsDefaultChannel, newTWIMLHandler("TMS", "Twilio Messaging Service", true), tmsDefaultSendTestCases, nil)
	RunChannelSendTestCases(t, twDefaultChannel, newTWIMLHandler("TW", "TwiML", true), twDefaultSendTestCases, nil)
	RunChannelSendTestCases(t, swChannel, newTWIMLHandler("SW", "SignalWire", false), swSendTestCases, nil)

	waChannel := courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SW", "+12065551212", "US",
		map[string]interface{}{
			configAccountSID:        "accountSID",
			courier.ConfigAuthToken: "authToken",
		},
	)
	waChannel.SetScheme(urns.WhatsAppScheme)

	RunChannelSendTestCases(t, waChannel, newTWIMLHandler("T", "Twilio Whatsapp", true), waSendTestCases, nil)
}
