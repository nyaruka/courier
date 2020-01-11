package hormuud

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var (
	receiveNoParams     = "/c/hm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"
	receiveValidMessage = "/c/hm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive?Sender=%2B2349067554729&MessageText=Join&TimeSent=1493735509&&ShortCode=2020"
	receiveInvalidURN   = "/c/hm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive?Sender=bad&MessageText=Join&TimeSent=1493735509&&ShortCode=2020"
	receiveEmptyMessage = "/c/hm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive?Sender=%2B2349067554729&MessageText=&TimeSent=1493735509&&ShortCode=2020"
	statusNoParams      = "/c/hm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"
	statusInvalidStatus = "/c/hm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/?id=12345&status=66"
	statusValid         = "/c/hm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/?id=12345&status=4"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "HM", "2020", "US", nil),
}

var handleTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveValidMessage, Data: "empty", Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+2349067554729"), Date: Tp(time.Date(2017, 5, 2, 14, 31, 49, 0, time.UTC))},
	{Label: "Receive Empty Message", URL: receiveEmptyMessage, Data: "empty", Status: 200, Response: "Accepted",
		Text: Sp(""), URN: Sp("tel:+2349067554729"), Date: Tp(time.Date(2017, 5, 2, 14, 31, 49, 0, time.UTC))},
	{Label: "Receive No Params", URL: receiveNoParams, Data: "empty", Status: 400, Response: "field 'sender' required"},
	{Label: "Invalid URN", URL: receiveInvalidURN, Data: "empty", Status: 400, Response: "phone number supplied is not a number"},
	//	{Label: "Status No Params", URL: statusNoParams, Status: 400, Response: "field 'status' required"},
	//	{Label: "Status Invalid Status", URL: statusInvalidStatus, Status: 400, Response: "unknown status '66', must be one of 1,2,4,8,16"},
	//	{Label: "Status Valid", URL: statusValid, Status: 200, Response: `"status":"S"`},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), handleTestCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL
}

var sendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status: "W", ExternalID: "msg1",
		ResponseBody: `{"ResCode": "res", "ResMsg": "msg", "Data": { "MessageID": "msg1", "Description": "accepted" } }`, ResponseStatus: 200,
		RequestBody: `{"mobile":"250788383383","message":"Simple Message","senderid":"2020","mType":-1,"eType":-1,"UDH":""}`,
		SendPrep:    setSendURL},
	{Label: "Unicode Send",
		Text: "☺", URN: "tel:+250788383383",
		Status: "W", ExternalID: "msg1",
		ResponseBody: `{"ResCode": "res", "ResMsg": "msg", "Data": { "MessageID": "msg1", "Description": "accepted" } }`, ResponseStatus: 200,
		RequestBody: `{"mobile":"250788383383","message":"☺","senderid":"2020","mType":-1,"eType":-1,"UDH":""}`,
		SendPrep:    setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status: "W", ExternalID: "msg1",
		ResponseBody: `{"ResCode": "res", "ResMsg": "msg", "Data": { "MessageID": "msg1", "Description": "accepted" } }`, ResponseStatus: 200,
		RequestBody: `{"mobile":"250788383383","message":"My pic!\nhttps://foo.bar/image.jpg","senderid":"2020","mType":-1,"eType":-1,"UDH":""}`,
		SendPrep:    setSendURL},
	{Label: "Error Sending",
		Text: "Error Sending", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `[{"Response": "101"}]`, ResponseStatus: 403,
		SendPrep: setSendURL},
}

var tokenTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status:   "E",
		SendPrep: setSendURL},
}

func TestSending(t *testing.T) {
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

	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "HM", "2020", "US",
		map[string]interface{}{
			"username": "foo@bar.com",
			"password": "sesame",
		},
	)

	RunChannelSendTestCases(t, defaultChannel, newHandler(), sendTestCases, nil)

	tokenURL = server.URL + "?invalid=true"

	RunChannelSendTestCases(t, defaultChannel, newHandler(), tokenTestCases, nil)
}
