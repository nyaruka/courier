package novo

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "NV", "2020", "TT", map[string]any{
		"merchant_id":     "my-merchant-id",
		"merchant_secret": "my-merchant-secret",
		"secret":          "sesame",
	}),
}

const (
	receiveURL = "/c/nv/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
)

var testCases = []IncomingTestCase{
	{
		Label:                "Receive Valid",
		URL:                  receiveURL,
		Headers:              map[string]string{"Authorization": "sesame"},
		Data:                 "text=Msg&from=18686846481",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Message Accepted",
		ExpectedMsgText:      Sp("Msg"),
		ExpectedURN:          "tel:+18686846481",
	},
	{
		Label:                "Receive Missing Number",
		URL:                  receiveURL,
		Headers:              map[string]string{"Authorization": "sesame"},
		Data:                 "text=Msg",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "required field 'from'",
	},
	{
		Label:                "Receive Missing Authorization",
		URL:                  receiveURL,
		Data:                 "text=Msg&from=18686846481",
		ExpectedRespStatus:   401,
		ExpectedBodyContains: "invalid Authorization header",
	},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	sendURL = s.URL + "?%s"
}

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:              "Plain Send",
		MsgText:            "Simple Message ☺",
		MsgURN:             "tel:+18686846481",
		MockResponseBody:   `{"blastId": "-437733473338","status": "FINISHED","type": "SMS","statusDescription": "Finished"}`,
		MockResponseStatus: 200,
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Long Send",
		MsgText:            "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:             "tel:+18686846481",
		MockResponseBody:   `{"blastId": "-437733473338","status": "FINISHED","type": "SMS","statusDescription": "Finished"}`,
		MockResponseStatus: 200,
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Send Attachment",
		MsgText:            "My pic!",
		MsgURN:             "tel:+18686846481",
		MsgAttachments:     []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody:   `{"blastId": "-437733473338","status": "FINISHED","type": "SMS","statusDescription": "Finished"}`,
		MockResponseStatus: 200,
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Invalid Parameters",
		MsgText:            "Invalid Parameters",
		MsgURN:             "tel:+18686846481",
		MockResponseBody:   `{"error": "Incorrect Query String Authentication ","expectedQueryString": "8868;18686846480;test;"}`,
		MockResponseStatus: 200,
		ExpectedMsgStatus:  "F",
		ExpectedErrors:     []*courier.ChannelError{courier.NewChannelError("", "", "received invalid response")},
		SendPrep:           setSendURL,
	},
	{
		Label:              "Error Response",
		MsgText:            "Error Response",
		MsgURN:             "tel:+18686846481",
		MockResponseBody:   `{"error": "Incorrect Query String Authentication ","expectedQueryString": "8868;18686846480;test;"}`,
		MockResponseStatus: 200,
		ExpectedMsgStatus:  "F",
		ExpectedErrors:     []*courier.ChannelError{courier.NewChannelError("", "", "received invalid response")},
		SendPrep:           setSendURL,
	},
}

func TestOutgoing(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "NV", "2020", "TT",
		map[string]any{
			"merchant_id":     "my-merchant-id",
			"merchant_secret": "my-merchant-secret",
			"secret":          "sesame",
		})
	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"my-merchant-secret", "sesame"}, nil)
}
