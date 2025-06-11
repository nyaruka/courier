package clickmobile

import (
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

const (
	receiveURL = "/c/cm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"

	validReceive = `<request>
	<shortCode>2020</shortCode>
	<mobile>265990099333</mobile>
	<referenceID>1232434354</referenceID>
	<text>Join</text>
	</request>`

	invalidURNReceive = `<request>
	<shortCode>2020</shortCode>
	<mobile>MTN</mobile>
	<referenceID>1232434354</referenceID>
	<text>Join</text>
	</request>`

	validReceiveEmptyText = `<request>
	<shortCode>2020</shortCode>
	<mobile>265990099333</mobile>
	<referenceID>1232434354</referenceID>
	<text></text>
	</request>`

	validMissingText = `<request>
	<shortCode>2020</shortCode>
	<mobile>265990099333</mobile>
	<referenceID>1232434354</referenceID>
	</request>`

	validMissingReferenceID = `<request>
	<shortCode>2020</shortCode>
	<mobile>265990099333</mobile>
	<text>Join</text>
	</request>`

	missingShortcode = `<request>
	<mobile>265990099333</mobile>
	<referenceID>1232434354</referenceID>
	<text>Join</text>
	</request>`

	missingMobile = `<request>
	<shortCode>2020</shortCode>
	<referenceID>1232434354</referenceID>
	<text>Join</text>
	</request>`
)

var incomingCases = []IncomingTestCase{
	{
		Label:                "Receive Valid Message",
		URL:                  receiveURL,
		Data:                 validReceive,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Join"),
		ExpectedURN:          "tel:+265990099333",
		ExpectedExternalID:   "1232434354",
	},
	{
		Label:                "Invalid URN",
		URL:                  receiveURL,
		Data:                 invalidURNReceive,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "not a possible number",
	},
	{
		Label:                "Receive valid with empty text",
		URL:                  receiveURL,
		Data:                 validReceiveEmptyText,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp(""),
		ExpectedURN:          "tel:+265990099333",
		ExpectedExternalID:   "1232434354",
	},
	{
		Label:                "Receive valid missing text",
		URL:                  receiveURL,
		Data:                 validMissingText,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp(""),
		ExpectedURN:          "tel:+265990099333",
		ExpectedExternalID:   "1232434354",
	},
	{
		Label:                "Receive valid missing referenceID",
		URL:                  receiveURL,
		Data:                 validMissingReferenceID,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Join"),
		ExpectedURN:          "tel:+265990099333",
	},
	{
		Label:                "Missing Shortcode",
		URL:                  receiveURL,
		Data:                 missingShortcode,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "missing parameters, must have 'mobile' and 'shortcode'",
	},
	{
		Label:                "Missing Mobile",
		URL:                  receiveURL,
		Data:                 missingMobile,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "missing parameters, must have 'mobile' and 'shortcode'",
	},
	{
		Label:                "Receive invalid XML",
		URL:                  receiveURL,
		Data:                 "empty",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "unable to parse request XML",
	},
}

func TestIncoming(t *testing.T) {
	chs := []courier.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "CM", "2020", "MW", []string{urns.Phone.Prefix}, nil),
	}

	RunIncomingTestCases(t, chs, newHandler(), incomingCases)
}

var outgoingCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/send": {
				httpx.NewMockResponse(200, nil, []byte(`{"code":"000","desc":"Operation successful.","data":{"new_record_id":"9"}}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{"Content-Type": "application/json"},
				Body:    `{"app_id":"001-app","org_id":"001-org","user_id":"Username","timestamp":"20180411182430","auth_key":"3e1347ddb444d13aa23d11e097602be0","operation":"send","reference":"10","message_type":"1","src_address":"2020","dst_address":"+250788383383","message":"Simple Message"}`,
			},
		},
	},
	{
		Label:   "Unicode Send",
		MsgText: "☺",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/send": {
				httpx.NewMockResponse(200, nil, []byte(`{"code":"000","desc":"Operation successful.","data":{"new_record_id":"9"}}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{"Content-Type": "application/json"},
				Body:    `{"app_id":"001-app","org_id":"001-org","user_id":"Username","timestamp":"20180411182430","auth_key":"3e1347ddb444d13aa23d11e097602be0","operation":"send","reference":"10","message_type":"1","src_address":"2020","dst_address":"+250788383383","message":"☺"}`,
			},
		},
	},
	{
		Label:   "Error Sending",
		MsgText: "Error Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/send": {
				httpx.NewMockResponse(401, nil, []byte(`{"code":"001","desc":"Database SQL Error"}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{"Content-Type": "application/json"},
				Body:    `{"app_id":"001-app","org_id":"001-org","user_id":"Username","timestamp":"20180411182430","auth_key":"3e1347ddb444d13aa23d11e097602be0","operation":"send","reference":"10","message_type":"1","src_address":"2020","dst_address":"+250788383383","message":"Error Message"}`,
			},
		},
		ExpectedError: courier.ErrResponseStatus,
	},
	{
		Label:          "Send Attachment",
		MsgText:        "My pic!",
		MsgURN:         "tel:+250788383383",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/send": {
				httpx.NewMockResponse(200, nil, []byte(`{"code":"000","desc":"Operation successful.","data":{"new_record_id":"9"}}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{"Content-Type": "application/json"},
				Body:    `{"app_id":"001-app","org_id":"001-org","user_id":"Username","timestamp":"20180411182430","auth_key":"3e1347ddb444d13aa23d11e097602be0","operation":"send","reference":"10","message_type":"1","src_address":"2020","dst_address":"+250788383383","message":"My pic!\nhttps://foo.bar/image.jpg"}`,
			},
		},
	},
	{
		Label:   "Response unexpected",
		MsgText: "Simple Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/send": {
				httpx.NewMockResponse(200, nil, []byte(`{"code":"001","desc":"Database SQL Error"}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{"Content-Type": "application/json"},
				Body:    `{"app_id":"001-app","org_id":"001-org","user_id":"Username","timestamp":"20180411182430","auth_key":"3e1347ddb444d13aa23d11e097602be0","operation":"send","reference":"10","message_type":"1","src_address":"2020","dst_address":"+250788383383","message":"Simple Message"}`,
			},
		},
		ExpectedError: courier.ErrResponseContent,
	},
}

func TestOutgoing(t *testing.T) {
	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "CM", "2020", "MW",
		[]string{urns.Phone.Prefix},
		map[string]any{
			"password": "Password",
			"username": "Username",
			"app_id":   "001-app",
			"org_id":   "001-org",
			"send_url": "http://example.com/send",
		},
	)

	// mock time so we can have predictable MD5 hashes
	dates.SetNowFunc(dates.NewFixedNow(time.Date(2018, 4, 11, 18, 24, 30, 123456000, time.UTC)))

	RunOutgoingTestCases(t, ch, newHandler(), outgoingCases, []string{"Password"}, nil)
}
