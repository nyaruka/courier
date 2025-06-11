package telesom

import (
	"net/url"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/courier/utils/clogs"
	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TS", "2020", "SO", []string{urns.Phone.Prefix}, nil),
}

var handleTestCases = []IncomingTestCase{
	{
		Label:                "Receive Valid Message",
		URL:                  "/c/ts/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?mobile=%2B2349067554729&msg=Join",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Join"),
		ExpectedURN:          "tel:+2349067554729",
	},
	{
		Label:                "Invalid URN",
		URL:                  "/c/ts/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?mobile=MTN&msg=Join",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "not a possible number",
	},
	{
		Label:                "Receive No Params",
		URL:                  "/c/ts/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "field 'mobile' required",
	},
	{
		Label:                "Receive No Sender",
		URL:                  "/c/ts/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?msg=Join",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "field 'mobile' required",
	},
	{
		Label:                "Receive Valid Message",
		URL:                  "/c/ts/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/",
		Data:                 "mobile=%2B2349067554729&msg=Join",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Join"),
		ExpectedURN:          "tel:+2349067554729",
	},
	{
		Label:                "Invalid URN",
		URL:                  "/c/ts/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/",
		Data:                 "mobile=MTN&msg=Join",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "not a possible number",
	},
	{
		Label:                "Receive No Params",
		URL:                  "/c/ts/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/",
		Data:                 "empty",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "field 'mobile' required",
	},
	{
		Label:                "Receive No Sender",
		URL:                  "/c/ts/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/",
		Data:                 "msg=Join",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "field 'mobile' required",
	},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), handleTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), handleTestCases)
}

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message",
		MsgURN:  "tel:+252788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://telesom.com/sendsms_other*": {
				httpx.NewMockResponse(200, nil, []byte(`<return>Success</return>`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form:    url.Values{"msg": {"Simple Message"}, "to": {"0788383383"}, "from": {"2020"}, "key": {"D69BB824F88F20482B94ECF3822EBD84"}},
			Headers: map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		}},
	},
	{
		Label:   "Unicode Send",
		MsgText: "☺",
		MsgURN:  "tel:+252788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://telesom.com/sendsms_other*": {
				httpx.NewMockResponse(200, nil, []byte(`<return>Success</return>`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form:    url.Values{"msg": {"☺"}, "to": {"0788383383"}, "from": {"2020"}, "key": {"60421A7D99BD79FE02697D567315AD0E"}},
			Headers: map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		}},
	},
	{
		Label:   "Error Sending",
		MsgText: "Error Message",
		MsgURN:  "tel:+252788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://telesom.com/sendsms_other*": {
				httpx.NewMockResponse(401, nil, []byte(`<return>error</return>`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form:    url.Values{"msg": {`Error Message`}, "to": {"0788383383"}, "from": {"2020"}, "key": {"3F1E492B2186551570F24C2F07D5D7E2"}},
			Headers: map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		}},
		ExpectedError: courier.ErrResponseStatus,
	},
	{
		Label:          "Send Attachment",
		MsgText:        "My pic!",
		MsgURN:         "tel:+252788383383",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"http://telesom.com/sendsms_other*": {
				httpx.NewMockResponse(200, nil, []byte(`<return>Success</return>`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form:    url.Values{"msg": {"My pic!\nhttps://foo.bar/image.jpg"}, "to": {"0788383383"}, "from": {"2020"}, "key": {"DBE569579FD899628C17254ECCE15DB7"}},
			Headers: map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		}},
	},
	{
		Label:   "Connection Error",
		MsgText: "Error Message",
		MsgURN:  "tel:+252788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://telesom.com/sendsms_other*": {
				httpx.NewMockResponse(500, nil, []byte(`<return>error</return>`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form:    url.Values{"msg": {`Error Message`}, "to": {"0788383383"}, "from": {"2020"}, "key": {"3F1E492B2186551570F24C2F07D5D7E2"}},
			Headers: map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		}},
		ExpectedError: courier.ErrConnectionFailed,
	},
	{
		Label:   "Response Unexpected",
		MsgText: "Simple Message",
		MsgURN:  "tel:+252788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://telesom.com/sendsms_other*": {
				httpx.NewMockResponse(200, nil, []byte(`<return>Missing</return>`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form:    url.Values{"msg": {"Simple Message"}, "to": {"0788383383"}, "from": {"2020"}, "key": {"D69BB824F88F20482B94ECF3822EBD84"}},
			Headers: map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		}},
		ExpectedLogErrors: []*clogs.Error{&clogs.Error{Message: "Received invalid response content: <return>Missing</return>"}},
		ExpectedError:     courier.ErrResponseContent,
	},
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TS", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			"password": "Password",
			"username": "Username",
			"secret":   "secret",
			"send_url": "http://telesom.com/sendsms_other",
		},
	)

	// mock time so we can have predictable MD5 hashes
	dates.SetNowFunc(dates.NewFixedNow(time.Date(2018, 4, 11, 18, 24, 30, 123456000, time.UTC)))

	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"Password", "secret"}, nil)
}
