package messangi

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)


var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MG", "2020", "JM", nil),
}

var (
	receiveURL = "/c/mg/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"

	validReceive  = "mo=Msg&mobile=18765422035"
	missingNumber = "mo=Msg"
)

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: validReceive, Status: 200, Response: "Message Accepted",
		Text: Sp("Msg"), URN: Sp("tel:+18765422035")},
	{Label: "Receive Missing Number", URL: receiveURL, Data: missingNumber, Status: 400, Response: "required field 'mobile'"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase {
	{Label: "Plain Send",
		Text: "Simple Message ☺", URN: "tel:+18765422035",
		Status: "W", ExternalID: "",
		ResponseBody: `<response><input>sendMT</input><status>OK</status><description>Completed</description></response>`,
		ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Long Send",
		Text: "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		URN: "tel:+18765422035",
		Status: "W",
		ExternalID: "",
		ResponseBody: `<response><input>sendMT</input><status>OK</status><description>Completed</description></response>`,
		ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+18765422035", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status: "W", ExternalID: "",
		ResponseBody: `<response><input>sendMT</input><status>OK</status><description>Completed</description></response>`,
		ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Invalid Parameters",
		Text: "Invalid Parameters", URN: "tel:+18765422035",
		Status: "E",
		ResponseBody: "", ResponseStatus: 404,
		SendPrep: setSendURL},
	{Label: "Error Response",
		Text: "Error Response", URN: "tel:+18765422035",
		Status: "F",
		ResponseBody: `<response><input>sendMT</input><status>ERROR</status><description>Completed</description></response>`,
		ResponseStatus: 200,
		SendPrep: setSendURL},
}

func TestSending(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MG", "2020", "JM",
		map[string]interface{}{
			"public_key": "my-public-key",
			"private_key": "my-private-key",
			"instance_id": 7,
			"carrier_id": 2,
		})
	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
