package twilio

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"fmt"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "T", "2020", "US", map[string]interface{}{"auth_token": "6789"}),
}

var tmsTestChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TMS", "2020", "US", map[string]interface{}{"auth_token": "6789"}),
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

	receiveValid        = "ToCountry=US&ToState=District+Of+Columbia&SmsMessageSid=SMe287d7109a5a925f182f0e07fe5b223b&NumMedia=0&ToCity=&FromZip=01022&SmsSid=SMe287d7109a5a925f182f0e07fe5b223b&FromState=MA&SmsStatus=received&FromCity=CHICOPEE&Body=Msg&FromCountry=US&To=%2B12028831111&ToZip=&NumSegments=1&MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&AccountSid=acctid&From=%2B14133881111&ApiVersion=2010-04-01"
	receiveMedia        = "ToCountry=US&ToState=District+Of+Columbia&SmsMessageSid=SMe287d7109a5a925f182f0e07fe5b223b&NumMedia=2&ToCity=&FromZip=01022&SmsSid=SMe287d7109a5a925f182f0e07fe5b223b&FromState=MA&SmsStatus=received&FromCity=CHICOPEE&FromCountry=US&To=%2B12028831111&ToZip=&NumSegments=1&MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&AccountSid=acctid&From=%2B14133881111&ApiVersion=2010-04-01&MediaUrl0=cat.jpg&MediaUrl1=dog.jpg"
	receiveMediaWithMsg = "ToCountry=US&ToState=District+Of+Columbia&SmsMessageSid=SMe287d7109a5a925f182f0e07fe5b223b&NumMedia=2&ToCity=&Body=Msg&FromZip=01022&SmsSid=SMe287d7109a5a925f182f0e07fe5b223b&FromState=MA&SmsStatus=received&FromCity=CHICOPEE&FromCountry=US&To=%2B12028831111&ToZip=&NumSegments=1&MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&AccountSid=acctid&From=%2B14133881111&ApiVersion=2010-04-01&MediaUrl0=cat.jpg&MediaUrl1=dog.jpg"
	receiveBase64       = "ToCountry=US&ToState=District+Of+Columbia&SmsMessageSid=SMe287d7109a5a925f182f0e07fe5b223b&NumMedia=0&ToCity=&FromZip=01022&SmsSid=SMe287d7109a5a925f182f0e07fe5b223b&FromState=MA&SmsStatus=received&FromCity=CHICOPEE&Body=QmFubm9uIEV4cGxhaW5zIFRoZSBXb3JsZCAuLi4K4oCcVGhlIENhbXAgb2YgdGhlIFNhaW50c%2BKA&FromCountry=US&To=%2B12028831111&ToZip=&NumSegments=1&MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&AccountSid=acctid&From=%2B14133881111&ApiVersion=2010-04-01"

	statusInvalid = "MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&MessageStatus=huh"
	statusValid   = "MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&MessageStatus=delivered"

	tmsStatusExtra  = "SmsStatus=sent&MessageStatus=sent&To=2021&MessagingServiceSid=MGdb23ec0f89ee2632e46e91d8128f5e2b&MessageSid=SM0b6e2697aae04182a9f5b5c7a8994c7f&AccountSid=acctid&From=%2B14133881111&ApiVersion=2010-04-01"
	tmsReceiveExtra = "ToCountry=US&ToState=&SmsMessageSid=SMbbf29aeb9d380ce2a1c0ae4635ff9dab&NumMedia=0&ToCity=&FromZip=27609&SmsSid=SMbbf29aeb9d380ce2a1c0ae4635ff9dab&FromState=NC&SmsStatus=received&FromCity=RALEIGH&Body=John+Cruz&FromCountry=US&To=384387&ToZip=&NumSegments=1&MessageSid=SMbbf29aeb9d380ce2a1c0ae4635ff9dab&AccountSid=acctid&From=%2B14133881111&ApiVersion=2010-04-01"
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
	{Label: "Status No Params", URL: statusURL, Data: " ", Status: 400, Response: "field 'messagestatus' required",
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
	{Label: "Status No Params", URL: tmsStatusURL, Data: " ", Status: 400, Response: "field 'messagestatus' required",
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

func addValidSignature(r *http.Request) {
	r.ParseForm()
	sig, _ := twCalculateSignature(fmt.Sprintf("%s://%s%s", r.URL.Scheme, r.Host, r.URL.RequestURI()), r.PostForm, "6789")
	r.Header.Set(twSignatureHeader, string(sig))
}

func addInvalidSignature(r *http.Request) {
	r.Header.Set(twSignatureHeader, "invalidsig")
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, NewHandler("T", "Twilio"), testCases)
	RunChannelTestCases(t, tmsTestChannels, NewHandler("TMS", "Twilio Messaging Service"), tmsTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, NewHandler("T", "Twilio"), testCases)
	RunChannelBenchmarks(b, tmsTestChannels, NewHandler("TMS", "Twilio Messaging Service"), tmsTestCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(server *httptest.Server, channel courier.Channel, msg courier.Msg) {
	sendURL = server.URL + "/Account/"
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message ☺", URN: "tel:+250788383383",
		Status: "W", ExternalID: "1002",
		ResponseBody: `{ "sid": "1002" }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "Simple Message ☺", "To": "+250788383383", "From": "2020", "StatusCallback": "https://localhost/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"},
		Path:       "/Account/accountSID/Messages.json",
		Headers:    map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
		SendPrep:   setSendURL},
	{Label: "Long Send",
		Text:   "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		URN:    "tel:+250788383383",
		Status: "W", ExternalID: "1002",
		ResponseBody: `{ "sid": "1002" }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "I need to keep adding more things to make it work", "To": "+250788383383", "From": "2020", "StatusCallback": "https://localhost/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"},
		Path:       "/Account/accountSID/Messages.json",
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
		PostParams: map[string]string{"Body": "Simple Message ☺", "To": "+250788383383", "MessagingServiceSID": "messageServiceSID", "StatusCallback": "https://localhost/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56cd/status?id=10&action=callback"},
		Path:       "/Account/accountSID/Messages.json",
		Headers:    map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
		SendPrep:   setSendURL},
	{Label: "Long Send",
		Text:   "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		URN:    "tel:+250788383383",
		Status: "W", ExternalID: "1002",
		ResponseBody: `{ "sid": "1002" }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "I need to keep adding more things to make it work", "To": "+250788383383", "MessagingServiceSID": "messageServiceSID", "StatusCallback": "https://localhost/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56cd/status?id=10&action=callback"},
		Path:       "/Account/accountSID/Messages.json",
		Headers:    map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
		SendPrep:   setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{ "error": "out of credits" }`, ResponseStatus: 401,
		PostParams: map[string]string{"Body": "Error Message", "To": "+250788383383", "MessagingServiceSID": "messageServiceSID", "StatusCallback": "https://localhost/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56cd/status?id=10&action=callback"},
		SendPrep:   setSendURL},
	{Label: "Error Code",
		Text: "Error Code", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{ "code": 1001 }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "Error Code", "To": "+250788383383", "MessagingServiceSID": "messageServiceSID", "StatusCallback": "https://localhost/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56cd/status?id=10&action=callback"},
		SendPrep:   setSendURL},
	{Label: "Stopped Contact Code",
		Text: "Stopped Contact", URN: "tel:+250788383383",
		Status:       "F",
		ResponseBody: `{ "code": 21610 }`, ResponseStatus: 400,
		PostParams: map[string]string{"Body": "Stopped Contact", "To": "+250788383383", "MessagingServiceSID": "messageServiceSID", "StatusCallback": "https://localhost/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56cd/status?id=10&action=callback"},
		SendPrep:   setSendURL,
		Stopped:    true},
	{Label: "No SID",
		Text: "No SID", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{ }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "No SID", "To": "+250788383383", "MessagingServiceSID": "messageServiceSID", "StatusCallback": "https://localhost/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56cd/status?id=10&action=callback"},
		SendPrep:   setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: `{ "sid": "1002" }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "My pic!", "To": "+250788383383", "MediaUrl": "https://foo.bar/image.jpg", "MessagingServiceSID": "messageServiceSID", "StatusCallback": "https://localhost/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56cd/status?id=10&action=callback"},
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

	RunChannelSendTestCases(t, defaultChannel, NewHandler("T", "Twilio"), defaultSendTestCases)
	RunChannelSendTestCases(t, tmsDefaultChannel, NewHandler("TMS", "Twilio Messaging Service"), tmsDefaultSendTestCases)
}
