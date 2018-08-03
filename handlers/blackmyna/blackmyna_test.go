package blackmyna

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "BM", "2020", "US", nil),
}

var (
	receiveURL = "/c/bm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusURL  = "/c/bm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"

	emptyReceive = receiveURL + ""
	validReceive = receiveURL + "?to=3344&smsc=ncell&from=%2B9779814641111&text=Msg"
	invalidURN   = receiveURL + "?to=3344&smsc=ncell&from=MTN&text=Msg"
	missingText  = receiveURL + "?to=3344&smsc=ncell&from=%2B9779814641111"

	missingStatus = statusURL + "?"
	invalidStatus = statusURL + "?id=bmID&status=13"
	validStatus   = statusURL + "?id=bmID&status=2"
)

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: validReceive, Status: 200, Response: "Message Accepted",
		Text: Sp("Msg"), URN: Sp("tel:+9779814641111")},
	{Label: "Invalid URN", URL: invalidURN, Status: 400, Response: "phone number supplied is not a number"},
	{Label: "Receive Empty", URL: emptyReceive, Status: 400, Response: "field 'text' required"},
	{Label: "Receive Missing Text", URL: missingText, Status: 400, Response: "field 'text' required"},

	{Label: "Status Invalid", URL: invalidStatus, Status: 400, Response: "unknown status"},
	{Label: "Status Missing", URL: missingStatus, Status: 400, Response: "field 'status' required"},
	{Label: "Valid Status", URL: validStatus, Status: 200, Response: `"status":"F"`},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

// setSend takes care of setting the sendURL to call
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status: "W", ExternalID: "1002",
		ResponseBody: `[{"id": "1002"}]`, ResponseStatus: 200,
		Headers:    map[string]string{"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ="},
		PostParams: map[string]string{"message": "Simple Message", "address": "+250788383383", "senderaddress": "2020"},
		SendPrep:   setSendURL},
	{Label: "Unicode Send",
		Text: "☺", URN: "tel:+250788383383",
		Status: "W", ExternalID: "1002",
		ResponseBody: `[{"id": "1002"}]`, ResponseStatus: 200,
		PostParams: map[string]string{"message": "☺", "address": "+250788383383", "senderaddress": "2020"},
		SendPrep:   setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status: "W", ExternalID: "1002",
		ResponseBody: `[{ "id": "1002" }]`, ResponseStatus: 200,
		PostParams: map[string]string{"message": "My pic!\nhttps://foo.bar/image.jpg", "address": "+250788383383", "senderaddress": "2020"},
		SendPrep:   setSendURL},
	{Label: "No External Id",
		Text: "No External ID", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{ "error": "failed" }`, ResponseStatus: 200,
		Error:      "no external id returned in body",
		PostParams: map[string]string{"message": `No External ID`, "address": "+250788383383", "senderaddress": "2020"},
		SendPrep:   setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{ "error": "failed" }`, ResponseStatus: 401,
		PostParams: map[string]string{"message": `Error Message`, "address": "+250788383383", "senderaddress": "2020"},
		SendPrep:   setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "BM", "2020", "US",
		map[string]interface{}{
			courier.ConfigPassword: "Password",
			courier.ConfigUsername: "Username",
			courier.ConfigAPIKey:   "KEY",
		})

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
