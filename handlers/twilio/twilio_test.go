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

var (
	receiveURL         = "/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"
	statusURL          = "/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status"
	statusIDURL        = "/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=12345"
	statusInvalidIDURL = "/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=asdf"

	receiveValid        = "ToCountry=US&ToState=District+Of+Columbia&SmsMessageSid=SMe287d7109a5a925f182f0e07fe5b223b&NumMedia=0&ToCity=&FromZip=01022&SmsSid=SMe287d7109a5a925f182f0e07fe5b223b&FromState=MA&SmsStatus=received&FromCity=CHICOPEE&Body=Msg&FromCountry=US&To=%2B12028831111&ToZip=&NumSegments=1&MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&AccountSid=acctid&From=%2B14133881111&ApiVersion=2010-04-01"
	receiveMedia        = "ToCountry=US&ToState=District+Of+Columbia&SmsMessageSid=SMe287d7109a5a925f182f0e07fe5b223b&NumMedia=2&ToCity=&FromZip=01022&SmsSid=SMe287d7109a5a925f182f0e07fe5b223b&FromState=MA&SmsStatus=received&FromCity=CHICOPEE&FromCountry=US&To=%2B12028831111&ToZip=&NumSegments=1&MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&AccountSid=acctid&From=%2B14133881111&ApiVersion=2010-04-01&MediaUrl0=cat.jpg&MediaUrl1=dog.jpg"
	receiveMediaWithMsg = "ToCountry=US&ToState=District+Of+Columbia&SmsMessageSid=SMe287d7109a5a925f182f0e07fe5b223b&NumMedia=2&ToCity=&Body=Msg&FromZip=01022&SmsSid=SMe287d7109a5a925f182f0e07fe5b223b&FromState=MA&SmsStatus=received&FromCity=CHICOPEE&FromCountry=US&To=%2B12028831111&ToZip=&NumSegments=1&MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&AccountSid=acctid&From=%2B14133881111&ApiVersion=2010-04-01&MediaUrl0=cat.jpg&MediaUrl1=dog.jpg"
	receiveBase64       = "ToCountry=US&ToState=District+Of+Columbia&SmsMessageSid=SMe287d7109a5a925f182f0e07fe5b223b&NumMedia=0&ToCity=&FromZip=01022&SmsSid=SMe287d7109a5a925f182f0e07fe5b223b&FromState=MA&SmsStatus=received&FromCity=CHICOPEE&Body=QmFubm9uIEV4cGxhaW5zIFRoZSBXb3JsZCAuLi4K4oCcVGhlIENhbXAgb2YgdGhlIFNhaW50c%2BKA&FromCountry=US&To=%2B12028831111&ToZip=&NumSegments=1&MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&AccountSid=acctid&From=%2B14133881111&ApiVersion=2010-04-01"

	statusInvalid = "MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&MessageStatus=huh"
	statusValid   = "MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&MessageStatus=delivered"
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

func addValidSignature(r *http.Request) {
	r.ParseForm()
	sig, _ := twCalculateSignature(fmt.Sprintf("%s%s", "http://courier.test", r.URL.RequestURI()), r.PostForm, "6789")
	r.Header.Set(twSignatureHeader, string(sig))
}

func addInvalidSignature(r *http.Request) {
	r.Header.Set(twSignatureHeader, "invalidsig")
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, NewHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, NewHandler(), testCases)
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
		PostParams: map[string]string{"Body": "Simple Message ☺", "To": "+250788383383"},
		Path:       "/Account/accountSID/Messages.json",
		Headers:    map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
		SendPrep:   setSendURL},
	{Label: "Long Send",
		Text:   "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		URN:    "tel:+250788383383",
		Status: "W", ExternalID: "1002",
		ResponseBody: `{ "sid": "1002" }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "I need to keep adding more things to make it work", "To": "+250788383383"},
		Path:       "/Account/accountSID/Messages.json",
		Headers:    map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
		SendPrep:   setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{ "error": "out of credits" }`, ResponseStatus: 401,
		PostParams: map[string]string{"Body": "Error Message", "To": "+250788383383"},
		SendPrep:   setSendURL},
	{Label: "Error Code",
		Text: "Error Code", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{ "error_code": 1001 }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "Error Code", "To": "+250788383383"},
		SendPrep:   setSendURL},
	{Label: "Stopped Contact Code",
		Text: "Stopped Contact", URN: "tel:+250788383383",
		Status:       "F",
		ResponseBody: `{ "error_code": 21610 }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "Stopped Contact", "To": "+250788383383"},
		SendPrep:   setSendURL,
		Stopped:    true},
	{Label: "No SID",
		Text: "No SID", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{ }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "No SID", "To": "+250788383383"},
		SendPrep:   setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: `{ "sid": "1002" }`, ResponseStatus: 200,
		PostParams: map[string]string{"Body": "My pic!", "To": "+250788383383", "MediaUrl": "https://foo.bar/image.jpg"},
		SendPrep:   setSendURL},
}

func TestSending(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "T", "2020", "US",
		map[string]interface{}{
			configAccountSID:        "accountSID",
			courier.ConfigAuthToken: "authToken"})

	RunChannelSendTestCases(t, defaultChannel, NewHandler(), defaultSendTestCases)
}
