package kannel

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "KN", "2020", "US", nil),
}

var ignoreChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "KN", "2020", "US", map[string]any{"ignore_sent": true}),
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
		ExpectedBodyContains: "phone number supplied is not a number",
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
		Label:                "Status Valid",
		URL:                  "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/?id=12345&status=4",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"S"`,
		ExpectedStatuses:     []ExpectedStatus{{MsgID: 12345, Status: courier.MsgStatusSent}},
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
		ExpectedStatuses:     []ExpectedStatus{{MsgID: 12345, Status: courier.MsgStatusDelivered}},
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

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), handleTestCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	c.(*test.MockChannel).SetConfig("send_url", s.URL)
}

// setSendURLWithQuery takes care of setting the send_url to our test server host
func setSendURLWithQuery(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	c.(*test.MockChannel).SetConfig("send_url", s.URL+"?auth=foo")
}

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:              "Plain Send",
		MsgText:            "Simple Message",
		MsgURN:             "tel:+250788383383",
		MsgHighPriority:    false,
		ExpectedMsgStatus:  "W",
		MockResponseBody:   "0: Accepted for delivery",
		MockResponseStatus: 200,
		ExpectedURLParams: map[string]string{
			"text":     "Simple Message",
			"to":       "+250788383383",
			"coding":   "",
			"priority": "",
			"dlr-url":  "https://localhost/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&status=%d",
		},
		SendPrep: setSendURL,
	},
	{
		Label:              "Unicode Send",
		MsgText:            "☺",
		MsgURN:             "tel:+250788383383",
		MsgHighPriority:    false,
		ExpectedMsgStatus:  "W",
		MockResponseBody:   "0: Accepted for delivery",
		MockResponseStatus: 200,
		ExpectedURLParams:  map[string]string{"text": "☺", "to": "+250788383383", "coding": "2", "charset": "utf8", "priority": ""},
		SendPrep:           setSendURL,
	},
	{
		Label:              "Smart Encoding",
		MsgText:            "Fancy “Smart” Quotes",
		MsgURN:             "tel:+250788383383",
		MsgHighPriority:    false,
		ExpectedMsgStatus:  "W",
		MockResponseBody:   "0: Accepted for delivery",
		MockResponseStatus: 200,
		ExpectedURLParams:  map[string]string{"text": `Fancy "Smart" Quotes`, "to": "+250788383383", "coding": "", "priority": ""},
		SendPrep:           setSendURL},
	{
		Label:              "Not Routable",
		MsgText:            "Not Routable",
		MsgURN:             "tel:+250788383383",
		MsgHighPriority:    false,
		ExpectedMsgStatus:  "F",
		MockResponseBody:   "Not routable. Do not try again.",
		MockResponseStatus: 403,
		ExpectedURLParams:  map[string]string{"text": `Not Routable`, "to": "+250788383383", "coding": "", "priority": ""},
		SendPrep:           setSendURL,
	},
	{
		Label:              "Error Sending",
		MsgText:            "Error Message",
		MsgURN:             "tel:+250788383383",
		MsgHighPriority:    false,
		ExpectedMsgStatus:  "E",
		MockResponseBody:   "1: Unknown channel",
		MockResponseStatus: 401,
		ExpectedURLParams:  map[string]string{"text": `Error Message`, "to": "+250788383383", "coding": "", "priority": ""},
		SendPrep:           setSendURL,
	},
	{
		Label:              "Custom Params",
		MsgText:            "Custom Params",
		MsgURN:             "tel:+250788383383",
		MsgHighPriority:    true,
		ExpectedMsgStatus:  "W",
		MockResponseBody:   "0: Accepted for delivery",
		MockResponseStatus: 201,
		ExpectedURLParams:  map[string]string{"text": `Custom Params`, "to": "+250788383383", "coding": "", "priority": "1", "auth": "foo"},
		SendPrep:           setSendURLWithQuery,
	},
	{
		Label:              "Send Attachment",
		MsgText:            "My pic!",
		MsgURN:             "tel:+250788383383",
		MsgHighPriority:    true,
		MsgAttachments:     []string{"image/jpeg:https://foo.bar/image.jpg"},
		ExpectedMsgStatus:  "W",
		MockResponseBody:   `0: Accepted for delivery`,
		MockResponseStatus: 200,
		ExpectedURLParams:  map[string]string{"text": "My pic!\nhttps://foo.bar/image.jpg", "to": "+250788383383", "from": "2020", "dlr-mask": "27"},
		SendPrep:           setSendURL,
	},
}

var nationalSendTestCases = []OutgoingTestCase{
	{
		Label:              "National Send",
		MsgText:            "success",
		MsgURN:             "tel:+250788383383",
		MsgHighPriority:    true,
		ExpectedMsgStatus:  "W",
		MockResponseBody:   "0: Accepted for delivery",
		MockResponseStatus: 200,
		ExpectedURLParams:  map[string]string{"text": "success", "to": "788383383", "coding": "", "priority": "1", "dlr-mask": "3"},
		SendPrep:           setSendURL,
	},
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "KN", "2020", "US",
		map[string]any{
			"password": "Password",
			"username": "Username"})

	var nationalChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "KN", "2020", "US",
		map[string]any{
			"password":     "Password",
			"username":     "Username",
			"use_national": true,
			"verify_ssl":   false,
			"dlr_mask":     "3",
		})

	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"Password"}, nil)
	RunOutgoingTestCases(t, nationalChannel, newHandler(), nationalSendTestCases, []string{"Password"}, nil)
}
