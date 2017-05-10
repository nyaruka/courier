package twilio

import (
	"net/http"
	"testing"

	"fmt"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TW", "2020", "US", map[string]string{"auth_token": "6789"}),
}

var (
	receiveURL = "/c/tw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusURL  = "/c/tw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"

	receiveValid  = "ToCountry=US&ToState=District+Of+Columbia&SmsMessageSid=SMe287d7109a5a925f182f0e07fe5b223b&NumMedia=0&ToCity=&FromZip=01022&SmsSid=SMe287d7109a5a925f182f0e07fe5b223b&FromState=MA&SmsStatus=received&FromCity=CHICOPEE&Body=Msg&FromCountry=US&To=%2B12028831111&ToZip=&NumSegments=1&MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&AccountSid=acctid&From=%2B14133881111&ApiVersion=2010-04-01"
	receiveMedia  = "ToCountry=US&ToState=District+Of+Columbia&SmsMessageSid=SMe287d7109a5a925f182f0e07fe5b223b&NumMedia=2&ToCity=&FromZip=01022&SmsSid=SMe287d7109a5a925f182f0e07fe5b223b&FromState=MA&SmsStatus=received&FromCity=CHICOPEE&Body=Msg&FromCountry=US&To=%2B12028831111&ToZip=&NumSegments=1&MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&AccountSid=acctid&From=%2B14133881111&ApiVersion=2010-04-01&MediaUrl0=cat.jpg&MediaUrl1=dog.jpg"
	receiveBase64 = "ToCountry=US&ToState=District+Of+Columbia&SmsMessageSid=SMe287d7109a5a925f182f0e07fe5b223b&NumMedia=0&ToCity=&FromZip=01022&SmsSid=SMe287d7109a5a925f182f0e07fe5b223b&FromState=MA&SmsStatus=received&FromCity=CHICOPEE&Body=QmFubm9uIEV4cGxhaW5zIFRoZSBXb3JsZCAuLi4K4oCcVGhlIENhbXAgb2YgdGhlIFNhaW50c%2BKA&FromCountry=US&To=%2B12028831111&ToZip=&NumSegments=1&MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&AccountSid=acctid&From=%2B14133881111&ApiVersion=2010-04-01"

	statusInvalid = "MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&MessageStatus=huh"
)

var testCases = []ChannelTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: receiveValid, Status: 200, Response: "<Response/>",
		Text: Sp("Msg"), URN: Sp("tel:+14133881111"), External: Sp("SMe287d7109a5a925f182f0e07fe5b223b"),
		PrepRequest: addValidSignature},
	{Label: "Receive Invalid Signature", URL: receiveURL, Data: receiveValid, Status: 400, Response: "invalid request signature",
		PrepRequest: addInvalidSignature},
	{Label: "Receive Missing Signature", URL: receiveURL, Data: receiveValid, Status: 400, Response: "missing request signature"},
	{Label: "Receive No Params", URL: receiveURL, Data: " ", Status: 400, Response: "field 'messagesid' required",
		PrepRequest: addValidSignature},
	{Label: "Receive Media", URL: receiveURL, Data: receiveMedia, Status: 200, Response: "<Response/>",
		Text: Sp("Msg"), URN: Sp("tel:+14133881111"), External: Sp("SMe287d7109a5a925f182f0e07fe5b223b"), MediaURLs: []string{"cat.jpg", "dog.jpg"},
		PrepRequest: addValidSignature},
	{Label: "Receive Base64", URL: receiveURL, Data: receiveBase64, Status: 200, Response: "<Response/>",
		Text: Sp("Bannon Explains The World ...\n“The Camp of the Saints"), URN: Sp("tel:+14133881111"), External: Sp("SMe287d7109a5a925f182f0e07fe5b223b"),
		PrepRequest: addValidSignature},
	{Label: "Status No Params", URL: statusURL, Data: " ", Status: 400, Response: "field 'messagestatus' required",
		PrepRequest: addValidSignature},
	{Label: "Status Invalid Status", URL: statusURL, Data: statusInvalid, Status: 400, Response: "unknown status 'huh'",
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
