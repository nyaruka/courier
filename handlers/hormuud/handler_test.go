package hormuud

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

var (
	receiveNoParams     = "/c/hm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"
	receiveValidMessage = "/c/hm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive?Sender=%2B2349067554729&MessageText=Join&TimeSent=20230418&&ShortCode=2020"
	receiveInvalidURN   = "/c/hm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive?Sender=bad&MessageText=Join&TimeSent=20230418&&ShortCode=2020"
	receiveEmptyMessage = "/c/hm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive?Sender=%2B2349067554729&MessageText=&TimeSent=20230418&&ShortCode=2020"
	//statusNoParams      = "/c/hm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"
	//statusInvalidStatus = "/c/hm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/?id=12345&status=66"
	//statusValid         = "/c/hm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/?id=12345&status=4"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "HM", "2020", "US", []string{urns.Phone.Prefix}, nil),
}

var handleTestCases = []IncomingTestCase{
	{
		Label:                "Receive Valid Message",
		URL:                  receiveValidMessage,
		Data:                 "empty",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Join"),
		ExpectedURN:          "tel:+2349067554729",
	},
	{
		Label:                "Receive Empty Message",
		URL:                  receiveEmptyMessage,
		Data:                 "empty",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp(""),
		ExpectedURN:          "tel:+2349067554729",
	},
	{
		Label:                "Receive No Params",
		URL:                  receiveNoParams,
		Data:                 "empty",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "field 'sender' required",
	},
	{
		Label:                "Invalid URN",
		URL:                  receiveInvalidURN,
		Data:                 "empty",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "not a possible number",
	},
	//	{Label: "Status No Params", URL: statusNoParams, Status: 400, Response: "field 'status' required"},
	//	{Label: "Status Invalid Status", URL: statusInvalidStatus, Status: 400, Response: "unknown status '66', must be one of 1,2,4,8,16"},
	//	{Label: "Status Valid", URL: statusValid, Status: 200, Response: `"status":"S"`},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), handleTestCases)
}

var sendTestCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://smsapi.hormuud.com/token": {
				httpx.NewMockResponse(200, nil, []byte(`{"access_token": "ghK_Wt4lshZhN"}`)),
			},
			"https://smsapi.hormuud.com/api/SendSMS": {
				httpx.NewMockResponse(200, nil, []byte(`{"ResCode": "res", "ResMsg": "msg", "Data": { "MessageID": "msg1", "Description": "accepted" } }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Form: url.Values{
					"Username":   {"foo@bar.com"},
					"Password":   {"sesame"},
					"grant_type": {"password"},
				},
			},
			{
				Headers: map[string]string{
					"Content-Type":  "application/json",
					"Accept":        "application/json",
					"Authorization": "Bearer ghK_Wt4lshZhN",
				},
				Body: `{"mobile":"250788383383","message":"Simple Message","senderid":"2020","mType":-1,"eType":-1,"UDH":""}`,
			},
		},
		ExpectedExtIDs: []string{"msg1"},
	},
	{
		Label:   "Unicode Send",
		MsgText: "☺",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://smsapi.hormuud.com/api/SendSMS": {
				httpx.NewMockResponse(200, nil, []byte(`{"ResCode": "res", "ResMsg": "msg", "Data": { "MessageID": "msg1", "Description": "accepted" } }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Bearer ghK_Wt4lshZhN",
			},
			Body: `{"mobile":"250788383383","message":"☺","senderid":"2020","mType":-1,"eType":-1,"UDH":""}`,
		}},
		ExpectedExtIDs: []string{"msg1"},
	},
	{
		Label:          "Send Attachment",
		MsgText:        "My pic!",
		MsgURN:         "tel:+250788383383",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://smsapi.hormuud.com/api/SendSMS": {
				httpx.NewMockResponse(200, nil, []byte(`{"ResCode": "res", "ResMsg": "msg", "Data": { "MessageID": "msg1", "Description": "accepted" } }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Bearer ghK_Wt4lshZhN",
			},
			Body: `{"mobile":"250788383383","message":"My pic!\nhttps://foo.bar/image.jpg","senderid":"2020","mType":-1,"eType":-1,"UDH":""}`,
		}},
		ExpectedExtIDs: []string{"msg1"},
	},
	{
		Label:   "Error Sending",
		MsgText: "Error Sending",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://smsapi.hormuud.com/api/SendSMS": {
				httpx.NewMockResponse(403, nil, []byte(`[{"Response": "101"}]`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Bearer ghK_Wt4lshZhN",
			},
			Body: `{"mobile":"250788383383","message":"Error Sending","senderid":"2020","mType":-1,"eType":-1,"UDH":""}`,
		}},
		ExpectedError: courier.ErrResponseStatus,
	},
	{
		Label:   "Connection Error",
		MsgText: "Error",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://smsapi.hormuud.com/api/SendSMS": {
				httpx.NewMockResponse(500, nil, []byte(`[{"Response": "101"}]`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Bearer ghK_Wt4lshZhN",
			},
			Body: `{"mobile":"250788383383","message":"Error","senderid":"2020","mType":-1,"eType":-1,"UDH":""}`,
		}},
		ExpectedError: courier.ErrConnectionFailed,
	},
}

var tokenTestCases = []OutgoingTestCase{
	{
		Label:   "Error getting token",
		MsgText: "Simple Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://smsapi.hormuud.com/token": {
				httpx.NewMockResponse(400, nil, []byte(`{"error": "invalid password"}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Form: url.Values{
					"Username":   {"foo@bar.com"},
					"Password":   {"sesame"},
					"grant_type": {"password"},
				},
			},
		},
		ExpectedError: courier.ErrResponseStatus,
	},
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "HM", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			"username": "foo@bar.com",
			"password": "sesame",
		},
	)

	h := newHandler()
	RunOutgoingTestCases(t, defaultChannel, h, sendTestCases, []string{"sesame"}, nil)

	conn := h.(*handler).Backend().RedisPool().Get()
	redis.String(conn.Do("DEL", fmt.Sprintf("hm_token_%s", defaultChannel.UUID())))
	defer conn.Close()

	RunOutgoingTestCases(t, defaultChannel, h, tokenTestCases, []string{"sesame"}, nil)
}
