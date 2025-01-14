package africastalking

import (
	"net/url"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

const (
	receiveURL = "/c/at/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusURL  = "/c/at/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"
)

var incomingCases = []IncomingTestCase{
	{
		Label:                "Receive Valid",
		URL:                  receiveURL,
		Data:                 "linkId=03090445075804249226&text=Msg&to=21512&id=ec9adc86-51d5-4bc8-8eb0-d8ab0bb53dc3&date=2017-05-03T06%3A04%3A45Z&from=%2B254791541111",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Message Accepted",
		ExpectedMsgText:      Sp("Msg"),
		ExpectedURN:          "tel:+254791541111",
		ExpectedExternalID:   "ec9adc86-51d5-4bc8-8eb0-d8ab0bb53dc3",
		ExpectedDate:         time.Date(2017, 5, 3, 06, 04, 45, 0, time.UTC),
	},
	{
		Label:                "Receive Valid",
		URL:                  receiveURL,
		Data:                 "linkId=03090445075804249226&text=Msg&to=21512&id=ec9adc86-51d5-4bc8-8eb0-d8ab0bb53dc3&date=2017-05-03+06%3A04%3A45&from=%2B254791541111",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Message Accepted",
		ExpectedMsgText:      Sp("Msg"),
		ExpectedURN:          "tel:+254791541111",
		ExpectedExternalID:   "ec9adc86-51d5-4bc8-8eb0-d8ab0bb53dc3",
		ExpectedDate:         time.Date(2017, 5, 3, 06, 04, 45, 0, time.UTC),
	},
	{
		Label:                "Receive Empty",
		URL:                  receiveURL,
		Data:                 "empty",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "field 'id' required",
	},
	{
		Label:                "Receive Missing Text",
		URL:                  receiveURL,
		Data:                 "linkId=03090445075804249226&to=21512&id=ec9adc86-51d5-4bc8-8eb0-d8ab0bb53dc3&date=2017-05-03T06%3A04%3A45Z&from=%2B254791541111",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "field 'text' required",
	},
	{
		Label:                "Invalid URN",
		URL:                  receiveURL,
		Data:                 "linkId=03090445075804249226&text=Msg&to=21512&id=ec9adc86-51d5-4bc8-8eb0-d8ab0bb53dc3&date=2017-05-03T06%3A04%3A45Z&from=MTN",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "not a possible number",
	},
	{
		Label:                "Invalid Date",
		URL:                  receiveURL,
		Data:                 "linkId=03090445075804249226&text=Msg&to=21512&id=ec9adc86-51d5-4bc8-8eb0-d8ab0bb53dc3&date=2017-05-03T06%3A04&from=%2B254791541111",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "invalid date format",
	},
	{
		Label:                "Status Invalid",
		URL:                  statusURL,
		Data:                 "id=ATXid_dda018a640edfcc5d2ce455de3e4a6e7&status=Borked",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "unknown status",
	},
	{
		Label:                "Status Missing",
		URL:                  statusURL,
		Data:                 "id=ATXid_dda018a640edfcc5d2ce455de3e4a6e7",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "field 'status' required",
	},
	{
		Label:                "Status Success",
		URL:                  statusURL,
		Data:                 "id=ATXid_dda018a640edfcc5d2ce455de3e4a6e7&status=Success",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "ATXid_dda018a640edfcc5d2ce455de3e4a6e7", Status: courier.MsgStatusDelivered}},
	},
	{
		Label:                "Status Expired",
		URL:                  statusURL,
		Data:                 "id=ATXid_dda018a640edfcc5d2ce455de3e4a6e7&status=Expired",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"F"`,
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "ATXid_dda018a640edfcc5d2ce455de3e4a6e7", Status: courier.MsgStatusFailed}},
	},
}

func TestIncoming(t *testing.T) {
	chs := []courier.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "AT", "2020", "US", []string{urns.Phone.Prefix}, nil),
	}

	RunIncomingTestCases(t, chs, newHandler(), incomingCases)
}

var outgoingCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message ☺",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.africastalking.com/version1/messaging": {
				httpx.NewMockResponse(200, nil, []byte(`{ "SMSMessageData": {"Recipients": [{"status": "Success", "messageId": "1002"}] } }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{"apikey": "KEY"},
				Form:    url.Values{"message": {"Simple Message ☺"}, "username": {"Username"}, "to": {"+250788383383"}, "from": {"2020"}},
			},
		},
		ExpectedExtIDs: []string{"1002"},
	},
	{
		Label:          "Send Attachment",
		MsgText:        "My pic!",
		MsgURN:         "tel:+250788383383",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.africastalking.com/version1/messaging": {
				httpx.NewMockResponse(200, nil, []byte(`{ "SMSMessageData": {"Recipients": [{"status": "Success", "messageId": "1002"}] } }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Form: url.Values{"message": {"My pic!\nhttps://foo.bar/image.jpg"}, "username": {"Username"}, "to": {"+250788383383"}, "from": {"2020"}}},
		},
		ExpectedExtIDs: []string{"1002"},
	},
	{
		Label:   "Explicit failed status",
		MsgText: "Hi",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.africastalking.com/version1/messaging": {
				httpx.NewMockResponse(200, nil, []byte(`{ "SMSMessageData": {"Recipients": [{"status": "Failed" }] } }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Form: url.Values{"message": {`Hi`}, "username": {"Username"}, "to": {"+250788383383"}, "from": {"2020"}}},
		},
		ExpectedError: courier.ErrResponseContent,
	},
	{
		Label:   "Missing status value",
		MsgText: "Error Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.africastalking.com/version1/messaging": {
				httpx.NewMockResponse(401, nil, []byte(`{ "error": "failed" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Form: url.Values{"message": {`Error Message`}, "username": {"Username"}, "to": {"+250788383383"}, "from": {"2020"}}},
		},
		ExpectedError: courier.ErrResponseStatus,
	},
}

var sharedOutgoingCases = []OutgoingTestCase{
	{
		Label:   "Shared Send",
		MsgText: "Simple Message ☺",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.africastalking.com/version1/messaging": {
				httpx.NewMockResponse(200, nil, []byte(`{ "SMSMessageData": {"Recipients": [{"status": "Success", "messageId": "1002"}] } }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{"apikey": "KEY"},
				Form:    url.Values{"message": {"Simple Message ☺"}, "username": {"Username"}, "to": {"+250788383383"}}},
		},
		ExpectedExtIDs: []string{"1002"},
	},
}

func TestOutgoing(t *testing.T) {
	defaultChannel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "AT", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			courier.ConfigUsername: "Username",
			courier.ConfigAPIKey:   "KEY",
		})
	sharedChannel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "AT", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			courier.ConfigUsername: "Username",
			courier.ConfigAPIKey:   "KEY",
			configIsShared:         true,
		})

	RunOutgoingTestCases(t, defaultChannel, newHandler(), outgoingCases, []string{"KEY"}, nil)
	RunOutgoingTestCases(t, sharedChannel, newHandler(), sharedOutgoingCases, []string{"KEY"}, nil)
}
