package messangi

import (
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MG", "2020", "JM", []string{urns.Phone.Prefix}, nil),
}

const (
	receiveURL = "/c/mg/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
)

var testCases = []IncomingTestCase{
	{
		Label:                "Receive Valid",
		URL:                  receiveURL,
		Data:                 "mo=Msg&mobile=18765422035",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Message Accepted",
		ExpectedMsgText:      Sp("Msg"),
		ExpectedURN:          "tel:+18765422035"},
	{
		Label:                "Receive Missing Number",
		URL:                  receiveURL,
		Data:                 "mo=Msg",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "required field 'mobile'"},
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
		MsgURN:  "tel:+18765422035",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://flow.messangi.me/mmc/rest/api/sendMT/*": {
				httpx.NewMockResponse(200, nil, []byte(`<response><input>sendMT</input><status>OK</status><description>Completed</description></response>`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Path: "/mmc/rest/api/sendMT/7/2020/2/18765422035/U2ltcGxlIE1lc3NhZ2Ug4pi6/my-public-key/f69bc6a924480d3ed82970d9679c4be90589bd3064add51c47e8bf50a211d55f",
		}},
	},
	{
		Label:   "Long Send",
		MsgText: "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:  "tel:+18765422035",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://flow.messangi.me/mmc/rest/api/sendMT/*": {
				httpx.NewMockResponse(200, nil, []byte(`<response><input>sendMT</input><status>OK</status><description>Completed</description></response>`)),
				httpx.NewMockResponse(200, nil, []byte(`<response><input>sendMT</input><status>OK</status><description>Completed</description></response>`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Path: "/mmc/rest/api/sendMT/7/2020/2/18765422035/VGhpcyBpcyBhIGxvbmdlciBtZXNzYWdlIHRoYW4gMTYwIGNoYXJhY3RlcnMgYW5kIHdpbGwgY2F1c2UgdXMgdG8gc3BsaXQgaXQgaW50byB0d28gc2VwYXJhdGUgcGFydHMsIGlzbid0IHRoYXQgcmlnaHQgYnV0IGl0IGlzIGV2ZW4gbG9uZ2VyIHRoYW4gYmVmb3JlIEkgc2F5LA/my-public-key/48c658e8db8635843ac3d3e497a81cf79cc0d75b8630dae03c6e7d93a749ab90",
			},
			{
				Path: "/mmc/rest/api/sendMT/7/2020/2/18765422035/SSBuZWVkIHRvIGtlZXAgYWRkaW5nIG1vcmUgdGhpbmdzIHRvIG1ha2UgaXQgd29yaw/my-public-key/ba305915a6cf56c1255071655de42b4408071460317bb5bf3419bb9f865c5078",
			},
		},
	},
	{
		Label:          "Send Attachment",
		MsgText:        "My pic!",
		MsgURN:         "tel:+18765422035",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://flow.messangi.me/mmc/rest/api/sendMT/*": {
				httpx.NewMockResponse(200, nil, []byte(`<response><input>sendMT</input><status>OK</status><description>Completed</description></response>`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Path: "/mmc/rest/api/sendMT/7/2020/2/18765422035/TXkgcGljIQpodHRwczovL2Zvby5iYXIvaW1hZ2UuanBn/my-public-key/4babdf316c0b5c7b6b40855329b421b1da1b8e63690d59eb5c231049dc4067fd",
		}},
	},
	{
		Label:   "Invalid Parameters",
		MsgText: "Invalid Parameters",
		MsgURN:  "tel:+18765422035",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://flow.messangi.me/mmc/rest/api/sendMT/*": {
				httpx.NewMockResponse(404, nil, []byte(``)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Path: "/mmc/rest/api/sendMT/7/2020/2/18765422035/SW52YWxpZCBQYXJhbWV0ZXJz/my-public-key/f3d2ea825cf61226925dee2db3c14b7fc00f3183f11809d2183d1e2dbd230df6",
		}},
		ExpectedError: courier.ErrResponseStatus,
	},
	{
		Label:   "Error Response",
		MsgText: "Error Response",
		MsgURN:  "tel:+18765422035",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://flow.messangi.me/mmc/rest/api/sendMT/*": {
				httpx.NewMockResponse(200, nil, []byte(`<response><input>sendMT</input><status>ERROR</status><description>Completed</description></response>`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Path: "/mmc/rest/api/sendMT/7/2020/2/18765422035/RXJyb3IgUmVzcG9uc2U/my-public-key/27f4c67fa00848ea6029cc0b1797aae6d05e2970ecb6e44ca486b463b933e61a",
		}},
		ExpectedError: courier.ErrResponseStatus,
	},
}

func TestOutgoing(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MG", "2020", "JM",
		[]string{urns.Phone.Prefix},
		map[string]any{
			"public_key":  "my-public-key",
			"private_key": "my-private-key",
			"instance_id": 7,
			"carrier_id":  2,
		})
	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"my-private-key"}, nil)
}
