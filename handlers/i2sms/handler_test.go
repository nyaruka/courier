package i2sms

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
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "I2", "2020", "US", []string{urns.Phone.Prefix}, nil),
}

const (
	receiveURL = "/c/i2/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
)

var testCases = []IncomingTestCase{
	{
		Label:                "Receive Valid",
		URL:                  receiveURL,
		Data:                 "message=Msg&mobile=254791541111",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "",
		ExpectedMsgText:      Sp("Msg"),
		ExpectedURN:          "tel:+254791541111",
	},
	{
		Label:                "Receive Missing Number",
		URL:                  receiveURL,
		Data:                 "message=Msg",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "required field 'mobile'",
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
		Label:          "Plain Send",
		MsgText:        "Simple Message ☺",
		MsgURN:         "tel:+250788383383",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://mx2.i2sms.net/mxapi.php": {
				httpx.NewMockResponse(200, nil, []byte(`{"result":{"session_id":"5b8fc97d58795484819426"}, "error_code": "00", "error_desc": "Success"}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Form: url.Values{
					"action":  {"send_single"},
					"mobile":  {"250788383383"},
					"message": {"Simple Message ☺\nhttps://foo.bar/image.jpg"},
					"channel": {"hash123"},
				},
			},
		},
		ExpectedExtIDs: []string{"5b8fc97d58795484819426"},
	},
	{
		Label:   "Invalid JSON",
		MsgText: "Invalid XML",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://mx2.i2sms.net/mxapi.php": {
				httpx.NewMockResponse(200, nil, []byte(`not json`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Form: url.Values{
					"action":  {"send_single"},
					"mobile":  {"250788383383"},
					"message": {"Invalid XML"},
					"channel": {"hash123"},
				},
			},
		},
		ExpectedError: courier.ErrResponseUnparseable,
	},
	{
		Label:   "Error Response",
		MsgText: "Error Response",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://mx2.i2sms.net/mxapi.php": {
				httpx.NewMockResponse(200, nil, []byte(`{"result":{}, "error_code": "10", "error_desc": "Failed"}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Form: url.Values{
					"action":  {"send_single"},
					"mobile":  {"250788383383"},
					"message": {"Error Response"},
					"channel": {"hash123"},
				},
			},
		},
		ExpectedError: courier.ErrFailedWithReason("10", "Failed"),
	},
	{
		Label:   "Error Sending",
		MsgText: "Error Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://mx2.i2sms.net/mxapi.php": {
				httpx.NewMockResponse(501, nil, []byte(`Bad Gateway`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Form: url.Values{
					"action":  {"send_single"},
					"mobile":  {"250788383383"},
					"message": {"Error Message"},
					"channel": {"hash123"},
				},
			},
		},
		ExpectedError: courier.ErrConnectionFailed,
	},
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "I2", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			courier.ConfigUsername: "user1",
			courier.ConfigPassword: "pass1",
			configChannelHash:      "hash123",
		})
	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{httpx.BasicAuth("user1", "pass1"), "hash123"}, nil)
}
