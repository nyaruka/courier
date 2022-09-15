package blackmyna

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "BM", "2020", "US", nil),
}

const (
	receiveURL = "/c/bm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusURL  = "/c/bm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"
)

var testCases = []ChannelHandleTestCase{
	{
		Label:                "Receive Valid",
		URL:                  receiveURL + "?to=3344&smsc=ncell&from=%2B9779814641111&text=Msg",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Message Accepted",
		ExpectedMsgText:      Sp("Msg"),
		ExpectedURN:          "tel:+9779814641111",
	},
	{
		Label:                "Invalid URN",
		URL:                  receiveURL + "?to=3344&smsc=ncell&from=MTN&text=Msg",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "phone number supplied is not a number",
	},
	{
		Label:                "Receive Empty",
		URL:                  receiveURL + "",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "field 'text' required",
	},
	{
		Label:                "Receive Missing Text",
		URL:                  receiveURL + "?to=3344&smsc=ncell&from=%2B9779814641111",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "field 'text' required",
	},
	{
		Label:                "Status Invalid",
		URL:                  statusURL + "?id=bmID&status=13",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "unknown status",
	},
	{
		Label:                "Status Missing",
		URL:                  statusURL + "?",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "field 'status' required",
	},
	{
		Label:                "Valid Status",
		URL:                  statusURL + "?id=bmID&status=2",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"F"`,
		ExpectedMsgStatus:    courier.MsgFailed,
	},
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
	{
		Label:              "Plain Send",
		MsgText:            "Simple Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `[{"id": "1002"}]`,
		MockResponseStatus: 200,
		ExpectedHeaders:    map[string]string{"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ="},
		ExpectedPostParams: map[string]string{"message": "Simple Message", "address": "+250788383383", "senderaddress": "2020"},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "1002",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Unicode Send",
		MsgText:            "☺",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `[{"id": "1002"}]`,
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"message": "☺", "address": "+250788383383", "senderaddress": "2020"},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "1002",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Send Attachment",
		MsgText:            "My pic!",
		MsgURN:             "tel:+250788383383",
		MsgAttachments:     []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody:   `[{ "id": "1002" }]`,
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"message": "My pic!\nhttps://foo.bar/image.jpg", "address": "+250788383383", "senderaddress": "2020"},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "1002",
		SendPrep:           setSendURL,
	},
	{
		Label:              "No External Id",
		MsgText:            "No External ID",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `{ "error": "failed" }`,
		MockResponseStatus: 200,
		ExpectedErrors:     []courier.ChannelError{courier.NewChannelError("no external id returned in body", "")},
		ExpectedPostParams: map[string]string{"message": `No External ID`, "address": "+250788383383", "senderaddress": "2020"},
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Error Sending",
		MsgText:            "Error Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `{ "error": "failed" }`,
		MockResponseStatus: 401,
		ExpectedPostParams: map[string]string{"message": `Error Message`, "address": "+250788383383", "senderaddress": "2020"},
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
}

func TestSending(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "BM", "2020", "US",
		map[string]interface{}{
			courier.ConfigPassword: "Password",
			courier.ConfigUsername: "Username",
			courier.ConfigAPIKey:   "KEY",
		})

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{httpx.BasicAuth("Username", "Password")}, nil)
}
