package highconnection

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "HX", "2020", "US", nil),
}

const (
	receiveURL = "/c/hx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusURL  = "/c/hx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"
)

var testCases = []IncomingTestCase{
	{
		Label:                "Receive Valid Message",
		URL:                  receiveURL,
		Data:                 "FROM=+33610346460&TO=5151&MESSAGE=Hello+World&RECEPTION_DATE=2015-04-02T14%3A26%3A06&ID=123456",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Hello World"),
		ExpectedURN:          "tel:+33610346460",
		ExpectedDate:         time.Date(2015, 04, 02, 14, 26, 06, 0, time.UTC),
		ExpectedExternalID:   "123456",
	},
	{
		Label:                "Receive Valid Message with accents",
		URL:                  receiveURL,
		Data:                 "FROM=+33610346460&TO=5151&MESSAGE=je+suis+tr%E8s+satisfait+&RECEPTION_DATE=2015-04-02T14%3A26%3A06&ID=123123",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("je suis très satisfait "),
		ExpectedURN:          "tel:+33610346460",
		ExpectedDate:         time.Date(2015, 04, 02, 14, 26, 06, 0, time.UTC),
		ExpectedExternalID:   "123123",
	},
	{
		Label:                "Invalid URN",
		URL:                  receiveURL,
		Data:                 "FROM=MTN&TO=5151&MESSAGE=Hello+World&RECEPTION_DATE=2015-04-02T14%3A26%3A06",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "phone number supplied is not a number",
	},
	{
		Label:                "Receive Missing Params",
		URL:                  receiveURL,
		Data:                 " ",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "validation for 'From' failed",
	},
	{
		Label:                "Receive Invalid Date",
		URL:                  receiveURL,
		Data:                 "FROM=+33610346460&TO=5151&MESSAGE=Hello+World&RECEPTION_DATE=2015-04-02T14:26",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "cannot parse",
	},
	{
		Label:                "Status Missing Params",
		URL:                  statusURL,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "validation for 'Status' failed",
	},
	{
		Label:                "Status Delivered",
		URL:                  statusURL + "?ret_id=12345&status=6",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses:     []ExpectedStatus{{MsgID: 12345, Status: courier.MsgStatusDelivered}},
	},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

// setSend takes care of setting the sendURL to call
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	sendURL = s.URL
}

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:              "Plain Send",
		MsgText:            "Simple Message",
		MsgURN:             "tel:+250788383383",
		MsgFlow:            &courier.FlowReference{UUID: "9de3663f-c5c5-4c92-9f45-ecbc09abcc85", Name: "Favorites"},
		MockResponseStatus: 200,
		ExpectedURLParams: map[string]string{
			"accountid":  "Username",
			"password":   "Password",
			"text":       "Simple Message",
			"to":         "+250788383383",
			"ret_id":     "10",
			"datacoding": "8",
			"user_data":  "Favorites",
			"ret_url":    "https://localhost/c/hx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status",
			"ret_mo_url": "https://localhost/c/hx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive",
		},
		ExpectedMsgStatus: "W",
		SendPrep:          setSendURL,
	},
	{
		Label:              "Plain Send without flow",
		MsgText:            "Simple Message",
		MsgURN:             "tel:+250788383383",
		MockResponseStatus: 200,
		ExpectedURLParams: map[string]string{
			"accountid":  "Username",
			"password":   "Password",
			"text":       "Simple Message",
			"to":         "+250788383383",
			"ret_id":     "10",
			"datacoding": "8",
			"user_data":  "",
			"ret_url":    "https://localhost/c/hx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status",
			"ret_mo_url": "https://localhost/c/hx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive",
		},
		ExpectedMsgStatus: "W",
		SendPrep:          setSendURL,
	},
	{
		Label:              "Unicode Send",
		MsgText:            "☺",
		MsgURN:             "tel:+250788383383",
		MsgFlow:            &courier.FlowReference{UUID: "9de3663f-c5c5-4c92-9f45-ecbc09abcc85", Name: "Favorites"},
		MockResponseStatus: 200,
		ExpectedURLParams: map[string]string{
			"accountid":  "Username",
			"password":   "Password",
			"text":       "☺",
			"to":         "+250788383383",
			"ret_id":     "10",
			"datacoding": "8",
			"user_data":  "Favorites",
			"ret_url":    "https://localhost/c/hx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status",
			"ret_mo_url": "https://localhost/c/hx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive",
		},
		ExpectedMsgStatus: "W",
		SendPrep:          setSendURL,
	},
	{
		Label:              "Long Send",
		MsgText:            "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:             "tel:+250788383383",
		MsgFlow:            &courier.FlowReference{UUID: "9de3663f-c5c5-4c92-9f45-ecbc09abcc85", Name: "Favorites"},
		MockResponseStatus: 200,
		ExpectedURLParams: map[string]string{
			"accountid":  "Username",
			"password":   "Password",
			"text":       "I need to keep adding more things to make it work",
			"to":         "+250788383383",
			"ret_id":     "10",
			"datacoding": "8",
			"user_data":  "Favorites",
			"ret_url":    "https://localhost/c/hx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status",
			"ret_mo_url": "https://localhost/c/hx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive",
		},
		ExpectedMsgStatus: "W",
		SendPrep:          setSendURL,
	},
	{
		Label:              "Send Attachement",
		MsgText:            "My pic!",
		MsgAttachments:     []string{"image/jpeg:https://foo.bar/image.jpg"},
		MsgURN:             "tel:+250788383383",
		MsgFlow:            &courier.FlowReference{UUID: "9de3663f-c5c5-4c92-9f45-ecbc09abcc85", Name: "Favorites"},
		MockResponseStatus: 200,
		ExpectedURLParams: map[string]string{
			"accountid":  "Username",
			"password":   "Password",
			"text":       "My pic!\nhttps://foo.bar/image.jpg",
			"to":         "+250788383383",
			"ret_id":     "10",
			"datacoding": "8",
			"user_data":  "Favorites",
			"ret_url":    "https://localhost/c/hx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status",
			"ret_mo_url": "https://localhost/c/hx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive",
		},
		ExpectedMsgStatus: "W",
		SendPrep:          setSendURL,
	},
	{
		Label:              "Error Sending",
		MsgText:            "Error Sending",
		MsgURN:             "tel:+250788383383",
		MockResponseStatus: 403,
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
}

func TestOutgoing(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "HX", "2020", "US",
		map[string]any{
			courier.ConfigPassword: "Password",
			courier.ConfigUsername: "Username",
		})

	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"Password"}, nil)
}
