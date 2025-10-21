package kannel

import (
	"net/url"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/core/models"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "KN", "2020", "US", []string{urns.Phone.Prefix}, nil),
}

var ignoreChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "KN", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{"ignore_sent": true}),
}

var handleTestCases = []IncomingTestCase{
	{
		Label:                "Receive Valid Message",
		URL:                  "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?backend=NIG_MTN&sender=%2B2349067554729&message=Join&ts=1493735509&id=asdf-asdf&to=24453",
		Data:                 "empty",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Join"),
		ExpectedURN:          "tel:+2349067554729",
		ExpectedExternalID:   "asdf-asdf",
		ExpectedDate:         time.Date(2017, 5, 2, 14, 31, 49, 0, time.UTC),
	},
	{
		Label:                "Receive KI Message",
		URL:                  "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?backend=NIG_MTN&sender=%2B68673076228&message=Join&ts=1493735509&id=asdf-asdf&to=24453",
		Data:                 "empty",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Join"),
		ExpectedURN:          "tel:+68673076228",
		ExpectedExternalID:   "asdf-asdf",
		ExpectedDate:         time.Date(2017, 5, 2, 14, 31, 49, 0, time.UTC),
	},
	{
		Label:                "Receive Empty Message",
		URL:                  "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?backend=NIG_MTN&sender=%2B2349067554729&message=&ts=1493735509&id=asdf-asdf&to=24453",
		Data:                 "empty",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp(""),
		ExpectedURN:          "tel:+2349067554729",
		ExpectedExternalID:   "asdf-asdf",
		ExpectedDate:         time.Date(2017, 5, 2, 14, 31, 49, 0, time.UTC),
	},
	{
		Label:                "Receive No Params",
		URL:                  "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/",
		Data:                 "empty",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "field 'sender' required",
	},
	{
		Label:                "Invalid URN",
		URL:                  "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?backend=NIG_MTN&sender=MTN&message=Join&ts=1493735509&id=asdf-asdf&to=24453",
		Data:                 "empty",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "not a possible number",
	},
	{
		Label:                "Status No Params",
		URL:                  "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "field 'status' required"},
	{
		Label:                "Status Invalid Status",
		URL:                  "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/?id=12345&status=66",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "unknown status '66', must be one of 1,2,4,8,16",
	},
	{
		Label:                "Status Valid by ID",
		URL:                  "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/?id=12345&status=4",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"S"`,
		ExpectedStatuses:     []ExpectedStatus{{MsgID: 12345, Status: models.MsgStatusSent}},
	},
	{
		Label:                "Status Valid by UUID",
		URL:                  "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/?uuid=384833a8-a817-401e-b37b-b2452298e21c&status=4",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"S"`,
		ExpectedStatuses:     []ExpectedStatus{{MsgUUID: "384833a8-a817-401e-b37b-b2452298e21c", Status: models.MsgStatusSent}},
	},
}

var ignoreTestCases = []IncomingTestCase{
	{
		Label:                "Receive Valid Message",
		URL:                  "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?backend=NIG_MTN&sender=%2B2349067554729&message=Join&ts=1493735509&id=asdf-asdf&to=24453",
		Data:                 "empty",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Join"),
		ExpectedURN:          "tel:+2349067554729",
		ExpectedExternalID:   "asdf-asdf",
		ExpectedDate:         time.Date(2017, 5, 2, 14, 31, 49, 0, time.UTC),
	},
	{
		Label:                "Write Status Delivered",
		URL:                  "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/?id=12345&status=1",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses:     []ExpectedStatus{{MsgID: 12345, Status: models.MsgStatusDelivered}},
	},
	{
		Label:                "Write Status Delivered by UUID",
		URL:                  "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/?uuid=384833a8-a817-401e-b37b-b2452298e21c&status=1",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses:     []ExpectedStatus{{MsgUUID: "384833a8-a817-401e-b37b-b2452298e21c", Status: models.MsgStatusDelivered}},
	},
	{
		Label:                "Ignore Status Wired",
		URL:                  "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/?uuid=384833a8-a817-401e-b37b-b2452298e21c&status=4",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `ignoring sent report`,
	},
	{
		Label:                "Ignore Status Sent",
		URL:                  "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/?uuid=384833a8-a817-401e-b37b-b2452298e21c&status=8",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `ignoring sent report`,
	},
	{
		Label:                "Ignore Status Wired",
		URL:                  "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/?id=12345&status=4",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `ignoring sent report`,
	},
	{
		Label:                "Ignore Status Sent",
		URL:                  "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/?id=12345&status=8",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `ignoring sent report`,
	},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), handleTestCases)
	RunIncomingTestCases(t, ignoreChannels, newHandler(), ignoreTestCases)
}

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:           "Plain Send",
		MsgText:         "Simple Message",
		MsgURN:          "tel:+250788383383",
		MsgHighPriority: false,
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/send*": {
				httpx.NewMockResponse(200, nil, []byte(`0: Accepted for delivery`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Params: url.Values{
				"text":     {"Simple Message"},
				"to":       {"+250788383383"},
				"from":     {"2020"},
				"dlr-mask": {"27"},
				"dlr-url":  {"https://localhost/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?uuid=0191e180-7d60-7000-aded-7d8b151cbd5b&status=%d"},
				"username": {"Username"},
				"password": {"Password"},
			},
		}},
	},
	{
		Label:           "Unicode Send",
		MsgText:         "☺",
		MsgURN:          "tel:+250788383383",
		MsgHighPriority: false,
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/send*": {
				httpx.NewMockResponse(200, nil, []byte(`0: Accepted for delivery`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Params: url.Values{
				"text":     {"☺"},
				"to":       {"+250788383383"},
				"from":     {"2020"},
				"coding":   {"2"},
				"charset":  {"utf8"},
				"dlr-mask": {"27"},
				"dlr-url":  {"https://localhost/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?uuid=0191e180-7d60-7000-aded-7d8b151cbd5b&status=%d"},
				"username": {"Username"},
				"password": {"Password"},
			},
		}},
	},
	{
		Label:           "Smart Encoding",
		MsgText:         "Fancy “Smart” Quotes",
		MsgURN:          "tel:+250788383383",
		MsgHighPriority: false,
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/send*": {
				httpx.NewMockResponse(200, nil, []byte(`0: Accepted for delivery`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Params: url.Values{
				"text":     {`Fancy "Smart" Quotes`},
				"to":       {"+250788383383"},
				"from":     {"2020"},
				"dlr-mask": {"27"},
				"dlr-url":  {"https://localhost/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?uuid=0191e180-7d60-7000-aded-7d8b151cbd5b&status=%d"},
				"username": {"Username"},
				"password": {"Password"},
			},
		}},
	},
	{
		Label:           "Not Routable",
		MsgText:         "Not Routable",
		MsgURN:          "tel:+250788383383",
		MsgHighPriority: false,
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/send*": {
				httpx.NewMockResponse(403, nil, []byte(`Not routable. Do not try again.`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Params: url.Values{
				"text":     {"Not Routable"},
				"to":       {"+250788383383"},
				"from":     {"2020"},
				"dlr-mask": {"27"},
				"dlr-url":  {"https://localhost/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?uuid=0191e180-7d60-7000-aded-7d8b151cbd5b&status=%d"},
				"username": {"Username"},
				"password": {"Password"},
			},
		}},
		ExpectedError: courier.ErrResponseStatus,
	},
	{
		Label:           "Error Sending",
		MsgText:         "Error Message",
		MsgURN:          "tel:+250788383383",
		MsgHighPriority: false,
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/send*": {
				httpx.NewMockResponse(401, nil, []byte(`1: Unknown channel`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Params: url.Values{
				"text":     {"Error Message"},
				"to":       {"+250788383383"},
				"from":     {"2020"},
				"dlr-mask": {"27"},
				"dlr-url":  {"https://localhost/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?uuid=0191e180-7d60-7000-aded-7d8b151cbd5b&status=%d"},
				"username": {"Username"},
				"password": {"Password"},
			},
		}},
		ExpectedError: courier.ErrResponseStatus,
	},

	{
		Label:           "Send Attachment",
		MsgText:         "My pic!",
		MsgURN:          "tel:+250788383383",
		MsgHighPriority: true,
		MsgAttachments:  []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/send*": {
				httpx.NewMockResponse(200, nil, []byte(`0: Accepted for delivery`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Params: url.Values{
				"text":     {"My pic!\nhttps://foo.bar/image.jpg"},
				"to":       {"+250788383383"},
				"from":     {"2020"},
				"priority": {"1"},
				"dlr-mask": {"27"},
				"dlr-url":  {"https://localhost/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?uuid=0191e180-7d60-7000-aded-7d8b151cbd5b&status=%d"},
				"username": {"Username"},
				"password": {"Password"},
			},
		}},
	},
}

var customParamsTestCases = []OutgoingTestCase{
	{
		Label:           "Custom Params",
		MsgText:         "Custom Params",
		MsgURN:          "tel:+250788383383",
		MsgHighPriority: true,
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/send*": {
				httpx.NewMockResponse(201, nil, []byte(`0: Accepted for delivery`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Params: url.Values{
				"text":     {"Custom Params"},
				"to":       {"+250788383383"},
				"from":     {"2020"},
				"priority": {"1"},
				"dlr-mask": {"27"},
				"dlr-url":  {"https://localhost/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?uuid=0191e180-7d60-7000-aded-7d8b151cbd5b&status=%d"},
				"username": {"Username"},
				"password": {"Password"},
				"auth":     {"foo"},
			},
		}},
	},
}

var nationalSendTestCases = []OutgoingTestCase{
	{
		Label:           "National Send",
		MsgText:         "success",
		MsgURN:          "tel:+250788383383",
		MsgHighPriority: true,
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/send*": {
				httpx.NewMockResponse(200, nil, []byte(`0: Accepted for delivery`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Params: url.Values{
				"text":     {"success"},
				"to":       {"788383383"},
				"from":     {"2020"},
				"priority": {"1"},
				"dlr-mask": {"3"},
				"dlr-url":  {"https://localhost/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?uuid=0191e180-7d60-7000-aded-7d8b151cbd5b&status=%d"},
				"username": {"Username"},
				"password": {"Password"},
			},
		}},
	},
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "KN", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			"password":           "Password",
			"username":           "Username",
			models.ConfigSendURL: "http://example.com/send",
		})

	var customParamsChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "KN", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			"password":           "Password",
			"username":           "Username",
			models.ConfigSendURL: "http://example.com/send?auth=foo",
		})

	var nationalChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "KN", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			"password":           "Password",
			"username":           "Username",
			"use_national":       true,
			"verify_ssl":         false,
			"dlr_mask":           "3",
			models.ConfigSendURL: "http://example.com/send",
		})

	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"Password"}, nil)
	RunOutgoingTestCases(t, customParamsChannel, newHandler(), customParamsTestCases, []string{"Password"}, nil)
	RunOutgoingTestCases(t, nationalChannel, newHandler(), nationalSendTestCases, []string{"Password"}, nil)
}
