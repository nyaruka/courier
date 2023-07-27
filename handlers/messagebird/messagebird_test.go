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
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MBD", "18665551212", "US", map[string]interface{}{
		"username":   "18665551212", 							//sending number
		"secret":     "my_super_secret",                            // secret key to sign for sig
		"auth_token": "authtoken",                            //API bearer token
	}),
}

const (

	receiveURL       = "/c/mbd/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	validSignature   = `eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJNZXNzYWdlQmlyZCIsIm5iZiI6MTY5MDMwNTEzOCwianRpIjoiZjRlZDgzN2UtYWM0Ni00ZWY3LThhYjItYWExY2YzMTE4MGJkIiwicGF5bG9hZF9oYXNoIjoiYWM3OTA3NTk3Mjc4ZDdjY2VkOTU0NmYyY2E3ZmFmYWFjNmY1MjU4YzQxN2VjYTkyNDkwNjVkZDM4NDU3M2RmYyJ9.FrhoATZOt5G7teacfeP-r-PNaGuwE1GZcxZHO8w1No0`
	validReceive     = `{"body":"Test 3","createdDatetime":"2023-07-25T17:31:42+00:00","date":"1690273902","date_utc":"1690306302","flowId":"21303270-85f5-4661-997d-8f406dec1932","flowRevisionId":"ffacb840-3381-4737-8017-9c0819a01c53","id":"22e6af2f764143e0b3e86b34084cc925","incomingMessage":"Test 3","invocationId":"d9ea9694-22d5-48aa-9b90-7363a2849ec2","message":"Test 3","messageBirdRequestId":"201412c7-2b11-11ee-b1e4-4a1a51e3ad83","message_id":"587c623091d84f228a0eda05b50bc0d3","originator":"188885551515","payload":"Test 3","receivedSMSDateTime":"2023-07-25T17:31:42+00:00","receiver":"18005551515","recipient":"18005551515","reference":"1","sender":"188885551515"}`
	invalidSignature = `eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJNZXNzYWdlQmlyZCIsIm5iZiI6MTY5MDMwNTEzOCwianRpIjoiZjRlZDgzN2UtYWM0Ni00ZWY3LThhYjItYWExY2YzMTE4MGJkIiwicGF5bG9hZF9oYXNoIjoiYWM3OTA3NTk3Mjc4ZDdjY2VkOTU0NmYyY2E3ZmFmYWFjNmY1MjU4YzQxN2VjYTkyNDkwNjVkZDM4NDU3M2RmYyJ9.V4_HH1ExRl625Vpl2bNDRGXK-OC8J70dRfNIVejBJDU`
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
		ExpectedDate:         time.Date(2019, 6, 21, 17, 43, 20, 866000000, time.UTC),
	},
	{
		Label:                "Bad Signature",
		Headers:              map[string]string{"Content-Type": "application/json", "Messagebird-Signature-Jwt": invalidSignature},
		URL:                  receiveURL,
		Data:                 validReceive,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: `{"message":"Error","data":[{"type":"error","error":"unable to verify signature, crypto/rsa: verification error"}]}`,
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
		ExpectedDate:         time.Date(2019, 6, 21, 17, 43, 20, 866000000, time.UTC),
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

func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	smsURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message ☺",
		MsgURN:              "tel:188885551515",
		MockResponseBody:    "",
		MockResponseStatus:  200,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Authorization": "Bearer enYtdXNlcm5hbWU6enYtcGFzc3dvcmQ="},
		ExpectedRequestBody: `{"messages":[{"message_parts":[{"text":{"content":"Simple Message ☺"}}],"actor_id":"c8fddfaf-622a-4a0e-b060-4f3ccbeab606","actor_type":"agent"}],"channel_id":"0534f78-b6e9-4f79-8853-11cedfc1f35b","users":[{"id":"c8fddfaf-622a-4a0e-b060-4f3ccbeab606"}]}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Send with text and image",
		MsgText:             "Simple Message ☺",
		MsgURN:              "tel:188885551515",
		MsgAttachments:      []string{"image:https://foo.bar/image.jpg"},
		MockResponseBody:    "",
		MockResponseStatus:  200,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Authorization": "Bearer enYtdXNlcm5hbWU6enYtcGFzc3dvcmQ="},
		ExpectedRequestBody: `{"messages":[{"message_parts":[{"text":{"content":"Simple Message ☺"}},{"image":{"url":"https://foo.bar/image.jpg"}}],"actor_id":"c8fddfaf-622a-4a0e-b060-4f3ccbeab606","actor_type":"agent"}],"channel_id":"0534f78-b6e9-4f79-8853-11cedfc1f35b","users":[{"id":"c8fddfaf-622a-4a0e-b060-4f3ccbeab606"}]}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Send with image only",
		MsgURN:              "tel:188885551515",
		MsgAttachments:      []string{"image/jpg:https://foo.bar/image.jpg"},
		MockResponseBody:    "",
		MockResponseStatus:  200,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Authorization": "Bearer enYtdXNlcm5hbWU6enYtcGFzc3dvcmQ="},
		ExpectedRequestBody: `{"messages":[{"message_parts":[{"image":{"url":"https://foo.bar/image.jpg"}}],"actor_id":"c8fddfaf-622a-4a0e-b060-4f3ccbeab606","actor_type":"agent"}],"channel_id":"0534f78-b6e9-4f79-8853-11cedfc1f35b","users":[{"id":"c8fddfaf-622a-4a0e-b060-4f3ccbeab606"}]}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "",
		SendPrep:            setSendURL,
	},
}

func TestSending(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MBD", "2020", "US", map[string]interface{}{
		"username":   "18665551212", 							//sending number
		"secret":     "my_super_secret",                            // secret key to sign for sig
		"auth_token": "authtoken",  
	})
	RunChannelSendTestCases(t, defaultChannel, newHandler("MBD", "Messagebird", false), defaultSendTestCases, []string{"my_super_secret", "authtoken"}, nil)
}
