package hormuud

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
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
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "HM", "2020", "US", nil),
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
		ExpectedBodyContains: "phone number supplied is not a number",
	},
	//	{Label: "Status No Params", URL: statusNoParams, Status: 400, Response: "field 'status' required"},
	//	{Label: "Status Invalid Status", URL: statusInvalidStatus, Status: 400, Response: "unknown status '66', must be one of 1,2,4,8,16"},
	//	{Label: "Status Valid", URL: statusValid, Status: 200, Response: `"status":"S"`},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), handleTestCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	sendURL = s.URL
}

var sendTestCases = []OutgoingTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message",
		MsgURN:              "tel:+250788383383",
		MockResponseBody:    `{"ResCode": "res", "ResMsg": "msg", "Data": { "MessageID": "msg1", "Description": "accepted" } }`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"mobile":"250788383383","message":"Simple Message","senderid":"2020","mType":-1,"eType":-1,"UDH":""}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "msg1",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Unicode Send",
		MsgText:             "☺",
		MsgURN:              "tel:+250788383383",
		MockResponseBody:    `{"ResCode": "res", "ResMsg": "msg", "Data": { "MessageID": "msg1", "Description": "accepted" } }`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"mobile":"250788383383","message":"☺","senderid":"2020","mType":-1,"eType":-1,"UDH":""}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "msg1",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Send Attachment",
		MsgText:             "My pic!",
		MsgURN:              "tel:+250788383383",
		MsgAttachments:      []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody:    `{"ResCode": "res", "ResMsg": "msg", "Data": { "MessageID": "msg1", "Description": "accepted" } }`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"mobile":"250788383383","message":"My pic!\nhttps://foo.bar/image.jpg","senderid":"2020","mType":-1,"eType":-1,"UDH":""}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "msg1",
		SendPrep:            setSendURL,
	},
	{
		Label:              "Error Sending",
		MsgText:            "Error Sending",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `[{"Response": "101"}]`,
		MockResponseStatus: 403,
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
}

var tokenTestCases = []OutgoingTestCase{
	{
		Label:             "Plain Send",
		MsgText:           "Simple Message",
		MsgURN:            "tel:+250788383383",
		ExpectedMsgStatus: "E",
		SendPrep:          setSendURL,
	},
}

func TestOutgoing(t *testing.T) {
	// set up a token server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("valid") == "true" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"access_token": "ghK_Wt4lshZhN"}`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "invalid password"}`))
	}))
	defer server.Close()

	tokenURL = server.URL + "?valid=true"

	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "HM", "2020", "US",
		map[string]any{
			"username": "foo@bar.com",
			"password": "sesame",
		},
	)

	RunOutgoingTestCases(t, defaultChannel, newHandler(), sendTestCases, []string{"sesame"}, nil)

	tokenURL = server.URL + "?invalid=true"

	RunOutgoingTestCases(t, defaultChannel, newHandler(), tokenTestCases, []string{"sesame"}, nil)
}
