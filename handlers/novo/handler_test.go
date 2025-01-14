package novo

import (
	"net/url"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "NV", "2020", "TT",
		[]string{urns.Phone.Prefix},
		map[string]any{
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

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message ☺",
		MsgURN:  "tel:+18686846481",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://novosmstools.com/novo_te/my-merchant-id/sendSMS*": {
				httpx.NewMockResponse(200, nil, []byte(`{"blastId": "-437733473338","status": "FINISHED","type": "SMS","statusDescription": "Finished"}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Params: url.Values{"from": {"2020"}, "to": {"18686846481"}, "msg": {"Simple Message ☺"}, "signature": {"29f1fe56b81979aaf9dfb693b91ad16c87a9303951f38abcc2794501da79fff0"}},
		}},
	},
	{
		Label:   "Long Send",
		MsgText: "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:  "tel:+18686846481",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://novosmstools.com/novo_te/my-merchant-id/sendSMS*": {
				httpx.NewMockResponse(200, nil, []byte(`{"blastId": "-437733473338","status": "FINISHED","type": "SMS","statusDescription": "Finished"}`)),
				httpx.NewMockResponse(200, nil, []byte(`{"blastId": "-437733473338","status": "FINISHED","type": "SMS","statusDescription": "Finished"}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Params: url.Values{"from": {"2020"}, "to": {"18686846481"}, "msg": {"This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say,"}, "signature": {"974f711ede732846e4d4da9bc95bf9452ae2337d5452c7417a19ed4034afd197"}},
			},
			{
				Params: url.Values{"from": {"2020"}, "to": {"18686846481"}, "msg": {"I need to keep adding more things to make it work"}, "signature": {"d6251beaa3398cb00c9354fd2fa80cc14ff0d9d42f6d6d488ad0f51b0719d89b"}},
			},
		},
	},
	{
		Label:          "Send Attachment",
		MsgText:        "My pic!",
		MsgURN:         "tel:+18686846481",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"http://novosmstools.com/novo_te/my-merchant-id/sendSMS*": {
				httpx.NewMockResponse(200, nil, []byte(`{"blastId": "-437733473338","status": "FINISHED","type": "SMS","statusDescription": "Finished"}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Params: url.Values{"from": {"2020"}, "to": {"18686846481"}, "msg": {"My pic!\nhttps://foo.bar/image.jpg"}, "signature": {"77a0feaf9a39e593f3e87d8cd3798e8aeabc1646501df7331c8d3bc3a54277fb"}},
		}},
	},
	{
		Label:   "Invalid Parameters",
		MsgText: "Invalid Parameters",
		MsgURN:  "tel:+18686846481",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://novosmstools.com/novo_te/my-merchant-id/sendSMS*": {
				httpx.NewMockResponse(200, nil, []byte(`{"error": "Incorrect Query String Authentication ","expectedQueryString": "8868;18686846480;test;"}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Params: url.Values{"from": {"2020"}, "to": {"18686846481"}, "msg": {"Invalid Parameters"}, "signature": {"4b640a668fd83223e38d429b15ea737ef58e1ab025b756baaca4743f3adb3f77"}},
		}},
		ExpectedError: courier.ErrResponseContent,
	},
	{
		Label:   "Error Response",
		MsgText: "Error Response",
		MsgURN:  "tel:+18686846481",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://novosmstools.com/novo_te/my-merchant-id/sendSMS*": {
				httpx.NewMockResponse(400, nil, []byte(`{"error": "Incorrect Query String Authentication ","expectedQueryString": "8868;18686846480;test;"}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Params: url.Values{"from": {"2020"}, "to": {"18686846481"}, "msg": {"Error Response"}, "signature": {"9fe49f073109de29f8c6d5108fd5719ee0b70c22cedb23fffdbabc8a99b9a0a9"}},
		}},
		ExpectedError: courier.ErrResponseStatus,
	},
}

func TestOutgoing(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "NV", "2020", "TT",
		[]string{urns.Phone.Prefix},
		map[string]any{
			"merchant_id":     "my-merchant-id",
			"merchant_secret": "my-merchant-secret",
			"secret":          "sesame",
		})
	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"my-merchant-secret", "sesame"}, nil)
}
