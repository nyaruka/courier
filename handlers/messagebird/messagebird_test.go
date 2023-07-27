package messagebird

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MBD", "18005551212", "US", map[string]interface{}{
		"secret":     "my_super_secret", // secret key to sign for sig
		"auth_token": "authtoken",       //API bearer token
	}),
}

const (
	receiveURL       = "/c/mbd/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	validSignature   = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJNZXNzYWdlQmlyZCIsIm5iZiI6MTY5MDMwNjMwNSwianRpIjoiZTkyY2YwNzktMzYyZC00ODEzLWFiNDAtYmJkZDkzOGJkYzZkIiwidXJsX2hhc2giOiI4NGNiMWEwOThlZTY2OGRmNTBmNzQ5Y2M2OThlZDBjZmIwN2FmMzllODBiZDgyZjIzNzFiNTY0NzViNTQ5N2EwIiwicGF5bG9hZF9oYXNoIjoiMjhjZTBiYTE5MDg3ZmE3ODgwZWMwOGQyYmFiMWM3ZDVmM2U2NWMzYjZhZTA5M2EwYjI2MTA4NDY3MTc4MDMzOSJ9.hR6TQQRkPLWFxCe0bcCWM0XdnTgNOlxUTcEzLWJuFkI"
	validReceive     = `{"receiver":"18005551515","sender":"188885551515","message":"Test again","date":1690386569,"date_utc":1690418969,"reference":"1","id":"b6aae1b5dfb2427a8f7ea6a717ba31a9","message_id":"3b53c137369242138120d6b0b2122607","recipient":"18005551515","originator":"188885551515","body":"Test 3","createdDatetime":"2023-07-27T00:49:29+00:00","mms":false}`
	invalidSignature = `eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJNZXNzYWdlQmlyZCIsIm5iZiI6MTY5MDMwNjMwNSwianRpIjoiZTkyY2YwNzktMzYyZC00ODEzLWFiNDAtYmJkZDkzOGJkYzZkIiwidXJsX2hhc2giOiI4NGNiMWEwOThlZTY2OGRmNTBmNzQ5Y2M2OThlZDBjZmIwN2FmMzllODBiZDgyZjIzNzFiNTY0NzViNTQ5N2EwIiwicGF5bG9hZF9oYXNoIjoiMDdhZTVjNmE5NjE2MGFlYjJlMGRkOGIwZWEwNTYxZDM2NzRiNjRhNWE3NTFiNmUxNWM0MDQ1MmY1NjFjYjcyZSJ9.jUUzDg2-e8fH7sghmxNC1cuuxRq-qYQgezZ52hPLL1A`
)

var sigtestCases = []ChannelHandleTestCase{
	{
		Label:                "Receive Valid w Signature",
		Headers:              map[string]string{"Content-Type": "application/json", "Messagebird-Signature-Jwt": validSignature},
		URL:                  receiveURL,
		Data:                 validReceive,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Message Accepted",
		ExpectedMsgText:      Sp("Test 3"),
		ExpectedURN:          "tel:188885551515",
		ExpectedDate:         time.Date(2023, time.July, 27, 00, 49, 29, 0, time.UTC),
	},
	{
		Label:                "Bad Signature",
		Headers:              map[string]string{"Content-Type": "application/json", "Messagebird-Signature-Jwt": invalidSignature},
		URL:                  receiveURL,
		Data:                 validReceive,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: `{"message":"Error","data":[{"type":"error","error":"invalid request signature, signature expected: 28ce0ba19087fa7880ec08d2bab1c7d5f3e65c3b6ae093a0b261084671780339 got: 07ae5c6a96160aeb2e0dd8b0ea0561d3674b64a5a751b6e15c40452f561cb72e for body: '{\"receiver\":\"18005551515\",\"sender\":\"188885551515\",\"message\":\"Test again\",\"date\":1690386569,\"date_utc\":1690418969,\"reference\":\"1\",\"id\":\"b6aae1b5dfb2427a8f7ea6a717ba31a9\",\"message_id\":\"3b53c137369242138120d6b0b2122607\",\"recipient\":\"18005551515\",\"originator\":\"188885551515\",\"body\":\"Test 3\",\"createdDatetime\":\"2023-07-27T00:49:29+00:00\",\"mms\":false}'"}]}`,
	},
}

var testCases = []ChannelHandleTestCase{
	{
		Label:                "Receive Valid w Sig",
		Headers:              map[string]string{"Content-Type": "application/json", "Messagebird-Signature-Jwt": validSignature},
		URL:                  receiveURL,
		Data:                 validReceive,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Message Accepted",
		ExpectedMsgText:      Sp("Test 3"),
		ExpectedURN:          "tel:188885551515",
		ExpectedDate:         time.Date(2023, time.July, 27, 00, 49, 29, 0, time.UTC),
	},
	{
		Label:                "Bad JSON",
		Headers:              map[string]string{"Content-Type": "application/json", "Messagebird-Signature-Jwt": invalidSignature},
		URL:                  receiveURL,
		Data:                 "empty",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: `{"message":"Error","data":[{"type":"error","error":"unable to parse request JSON: invalid character 'e' looking for beginning of value"}]}`,
	},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler("MBD", "Messagebird", true), sigtestCases)
	RunChannelTestCases(t, testChannels, newHandler("MBD", "Messagebird", false), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler("MBD", "Messagebird", false), testCases)
}

func setSmsSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	smsURL = s.URL
}

func setMmsSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	mmsURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message ☺",
		MsgURN:              "tel:188885551515",
		MockResponseBody:    "",
		MockResponseStatus:  200,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Authorization": "AccessKey authtoken"},
		ExpectedRequestBody: `{"recipients":["188885551515"],"originator":"18005551212","body":"Simple Message ☺"}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "",
		SendPrep:            setSmsSendURL,
	},
	{
		Label:               "Send with text and image",
		MsgText:             "Simple Message ☺",
		MsgURN:              "tel:188885551515",
		MsgAttachments:      []string{"image:https://foo.bar/image.jpg"},
		MockResponseBody:    "",
		MockResponseStatus:  200,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Authorization": "AccessKey authtoken"},
		ExpectedRequestBody: `{"recipients":["188885551515"],"originator":"18005551212","body":"Simple Message ☺","mediaUrls":["https://foo.bar/image.jpg"]}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "",
		SendPrep:            setMmsSendURL,
	},
	{
		Label:               "Send with image only",
		MsgURN:              "tel:188885551515",
		MsgAttachments:      []string{"image/jpg:https://foo.bar/image.jpg"},
		MockResponseBody:    "",
		MockResponseStatus:  200,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Authorization": "AccessKey authtoken"},
		ExpectedRequestBody: `{"recipients":["188885551515"],"originator":"18005551212","mediaUrls":["https://foo.bar/image.jpg"]}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "",
		SendPrep:            setMmsSendURL,
	},
}

func TestSending(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MBD", "18005551212", "US", map[string]interface{}{
		"secret":     "my_super_secret", // secret key to sign for sig
		"auth_token": "authtoken",
	})
	RunChannelSendTestCases(t, defaultChannel, newHandler("MBD", "Messagebird", false), defaultSendTestCases, []string{"my_super_secret", "authtoken"}, nil)
}
