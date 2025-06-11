package justcall

import (
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/courier/utils/clogs"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "JCL", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{courier.ConfigAPIKey: "api_key", courier.ConfigSecret: "api_secret"}),
}

var (
	receiveURL = "/c/jcl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"
	statusURL  = "/c/jcl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status"
)

var helloMsg = `{
	"data": {
	  "type": "sms",
	  "direction": "I",
	  "justcall_number": "2020",
	  "contact_name": "Sushant Tripathi",
	  "contact_number": "+385916242493",
	  "contact_email": "customer@gmail.com",
	  "is_contact": 1,
	  "content": "Hello there",
	  "signature": "35e89fc56b497xxxxxxxxxx8f7b27fe49d",
	  "datetime": "2020-12-03 13:35:13",
	  "delivery_status": "sent",
	  "requestid": "1229153",
	  "messageid": 26523491,
	  "is_mms": "0",
	  "mms": [],
	  "agent_name": "Sales JustCall",
	  "agent_id": 10636
	}
}`

var wrongMsgDirection = `{
	"data": {
	  "type": "sms",
	  "direction": "O",
	  "justcall_number": "2020",
	  "contact_name": "Sushant Tripathi",
	  "contact_number": "+385916242493",
	  "contact_email": "customer@gmail.com",
	  "is_contact": 1,
	  "content": "Hello there",
	  "signature": "35e89fc56b497xxxxxxxxxx8f7b27fe49d",
	  "datetime": "2020-12-03 13:35:13",
	  "delivery_status": "sent",
	  "requestid": "1229153",
	  "messageid": 26523491,
	  "is_mms": "0",
	  "mms": [],
	  "agent_name": "Sales JustCall",
	  "agent_id": 10636
	}
}`

var emptyMsg = `{
	"data": {
	  "type": "sms",
	  "direction": "I",
	  "justcall_number": "2020",
	  "contact_name": "Sushant Tripathi",
	  "contact_number": "+385916242493",
	  "contact_email": "customer@gmail.com",
	  "is_contact": 1,
	  "content": "",
	  "signature": "35e89fc56b497xxxxxxxxxx8f7b27fe49d",
	  "datetime": "2020-12-03 13:35:13",
	  "delivery_status": "sent",
	  "requestid": "1229153",
	  "messageid": 26523491,
	  "is_mms": "0",
	  "mms": [],
	  "agent_name": "Sales JustCall",
	  "agent_id": 10636
	}
}`

var attachmeentMsg = `{
	"data": {
	  "type": "sms",
	  "direction": "I",
	  "justcall_number": "2020",
	  "contact_name": "Sushant Tripathi",
	  "contact_number": "+385916242493",
	  "contact_email": "customer@gmail.com",
	  "is_contact": 1,
	  "content": "Hello there",
	  "signature": "35e89fc56b497xxxxxxxxxx8f7b27fe49d",
	  "datetime": "2020-12-03 13:35:13",
	  "delivery_status": "sent",
	  "requestid": "1229153",
	  "messageid": 26523491,
	  "is_mms": "1",
	  "mms": [
		{
			"media_url": "https://foo.bar/attachmentURL_Image",
			"content_type": "image/jpeg"
		}
	  ],
	  "agent_name": "Sales JustCall",
	  "agent_id": 10636
	}
}`

var validStatus = `{
	"data": {
	  "type": "sms",
	  "direction": "O",
	  "justcall_number": "2020",
	  "contact_name": "Sushant Tripathi",
	  "contact_number": "+385916242493",
	  "contact_email": "customer@gmail.com",
	  "is_contact": 1,
	  "content": "Hello there",
	  "signature": "35e89fc56b497xxxxxxxxxx8f7b27fe49d",
	  "datetime": "2020-12-03 13:35:13",
	  "delivery_status": "sent",
	  "requestid": "1229153",
	  "messageid": 26523491,
	  "is_mms": "1",
	  "mms": [
		{
			"media_url": "https://foo.bar/attachmentURL_Image",
			"content_type": "image/jpeg"
		}
	  ],
	  "agent_name": "Sales JustCall",
	  "agent_id": 10636
	}
}`

var invalidStatusDirection = `{
	"data": {
	  "type": "sms",
	  "direction": "I",
	  "justcall_number": "2020",
	  "contact_name": "Sushant Tripathi",
	  "contact_number": "+385916242493",
	  "contact_email": "customer@gmail.com",
	  "is_contact": 1,
	  "content": "Hello there",
	  "signature": "35e89fc56b497xxxxxxxxxx8f7b27fe49d",
	  "datetime": "2020-12-03 13:35:13",
	  "delivery_status": "sent",
	  "requestid": "1229153",
	  "messageid": 26523491,
	  "is_mms": "1",
	  "mms": [
		{
			"media_url": "https://foo.bar/attachmentURL_Image",
			"content_type": "image/jpeg"
		}
	  ],
	  "agent_name": "Sales JustCall",
	  "agent_id": 10636
	}
}`

var unknownStatus = `{
	"data": {
	  "type": "sms",
	  "direction": "O",
	  "justcall_number": "2020",
	  "contact_name": "Sushant Tripathi",
	  "contact_number": "+385916242493",
	  "contact_email": "customer@gmail.com",
	  "is_contact": 1,
	  "content": "Hello there",
	  "signature": "35e89fc56b497xxxxxxxxxx8f7b27fe49d",
	  "datetime": "2020-12-03 13:35:13",
	  "delivery_status": "foo",
	  "requestid": "1229153",
	  "messageid": 26523491,
	  "is_mms": "1",
	  "mms": [
		{
			"media_url": "https://foo.bar/attachmentURL_Image",
			"content_type": "image/jpeg"
		}
	  ],
	  "agent_name": "Sales JustCall",
	  "agent_id": 10636
	}
}`

var testCases = []IncomingTestCase{
	{
		Label:                "Receive Valid Message",
		URL:                  receiveURL,
		Data:                 helloMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Hello there"),
		ExpectedURN:          "tel:+385916242493",
		ExpectedExternalID:   "26523491",
		ExpectedDate:         time.Date(2020, 12, 03, 13, 35, 13, 000000000, time.FixedZone("", 0)),
	},
	{
		Label:                "Receive Wrong Message Direction",
		URL:                  receiveURL,
		Data:                 wrongMsgDirection,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Ignored",
	},
	{
		Label:                "Receive Empty Message",
		URL:                  receiveURL,
		Data:                 emptyMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp(""),
		ExpectedURN:          "tel:+385916242493",
		ExpectedExternalID:   "26523491",
		ExpectedDate:         time.Date(2020, 12, 03, 13, 35, 13, 000000000, time.FixedZone("", 0)),
	},
	{
		Label:                "Receive Attachment Message",
		URL:                  receiveURL,
		Data:                 attachmeentMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Hello there"),
		ExpectedAttachments:  []string{"https://foo.bar/attachmentURL_Image"},
		ExpectedURN:          "tel:+385916242493",
		ExpectedExternalID:   "26523491",
		ExpectedDate:         time.Date(2020, 12, 03, 13, 35, 13, 000000000, time.FixedZone("", 0)),
	},
	{
		Label:                "Receive valid status ",
		URL:                  statusURL,
		Data:                 validStatus,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"type":"status"`,
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "26523491", Status: courier.MsgStatusSent}},
	},
	{
		Label:                "Receive invalid status direction",
		URL:                  statusURL,
		Data:                 invalidStatusDirection,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `Ignored`,
	},
	{
		Label:                "Receive unknown status direction",
		URL:                  statusURL,
		Data:                 unknownStatus,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: `unknown status 'foo', must be one of send, delivered, undelivered, failed`,
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
		MsgText: "Simple Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.justcall.io/v1/texts/new": {
				httpx.NewMockResponse(200, nil, []byte(`{"status":"success","message":"Text sent","id":12345}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "api_key:api_secret",
			},
			Body: `{"from":"2020","to":"+250788383383","body":"Simple Message"}`,
		}},
		ExpectedExtIDs: []string{"12345"},
	},
	{
		Label:          "Send Document",
		MsgText:        "This is some text.",
		MsgURN:         "tel:+250788383383",
		MsgAttachments: []string{"application/pdf:https://foo.bar/document.pdf"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.justcall.io/v1/texts/new": {
				httpx.NewMockResponse(200, nil, []byte(`{"status":"success","message":"Text sent","id":12345}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "api_key:api_secret",
			},
			Body: `{"from":"2020","to":"+250788383383","body":"This is some text.","media_url":"https://foo.bar/document.pdf"}`,
		}},
		ExpectedExtIDs: []string{"12345"},
	},
	{
		Label:   "ID Error",
		MsgText: "ID Error",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.justcall.io/v1/texts/new": {
				httpx.NewMockResponse(200, nil, []byte(`{ "status": "success" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "api_key:api_secret",
			},
			Body: `{"from":"2020","to":"+250788383383","body":"ID Error"}`,
		}},
		ExpectedLogErrors: []*clogs.Error{courier.ErrorResponseValueMissing("id")},
	},
	{
		Label:   "Error Sending",
		MsgText: "Error",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.justcall.io/v1/texts/new": {
				httpx.NewMockResponse(200, nil, []byte(`{ "status": "fail" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "api_key:api_secret",
			},
			Body: `{"from":"2020","to":"+250788383383","body":"Error"}`,
		}},
		ExpectedError: courier.ErrResponseContent,
	},
	{
		Label:   "Error",
		MsgText: "Error",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.justcall.io/v1/texts/new": {
				httpx.NewMockResponse(403, nil, []byte(`{ "status": "fail" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "api_key:api_secret",
			},
			Body: `{"from":"2020","to":"+250788383383","body":"Error"}`,
		}},
		ExpectedError: courier.ErrResponseStatus,
	},
	{
		Label:   "Error Connection",
		MsgText: "Error",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.justcall.io/v1/texts/new": {
				httpx.NewMockResponse(500, nil, []byte(`Bad Gateway`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "api_key:api_secret",
			},
			Body: `{"from":"2020","to":"+250788383383","body":"Error"}`,
		}},
		ExpectedError: courier.ErrConnectionFailed,
	},
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "JCL", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{courier.ConfigAPIKey: "api_key", courier.ConfigSecret: "api_secret"})

	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"api_key", "api_secret"}, nil)
}
