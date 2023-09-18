package firebase

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/urns"
)

const (
	receiveURL  = "/c/fcm/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive"
	registerURL = "/c/fcm/8eb23e93-5ecb-45ba-b726-3b064e0c568c/register"
)

var longMsg = `Lorem ipsum dolor sit amet, consectetur adipiscing elit. Maecenas convallis augue vel placerat congue.
Etiam nec tempus enim. Cras placerat at est vel suscipit. Duis quis faucibus metus, non elementum tortor.
Pellentesque posuere ullamcorper metus auctor venenatis. Proin eget hendrerit dui. Sed eget massa nec mauris consequat pretium.
Praesent mattis arcu tortor, ac aliquet turpis tincidunt eu.

Fusce ut lacinia augue. Vestibulum felis nisi, porta ut est condimentum, condimentum volutpat libero.
Suspendisse a elit venenatis, condimentum sem at, ultricies mauris. Morbi interdum sem id tempor tristique.
Ut tincidunt massa eu purus lacinia sodales a volutpat neque. Cras dolor quam, eleifend a rhoncus quis, sodales nec purus.
Vivamus justo dolor, gravida at quam eu, hendrerit rutrum justo. Sed hendrerit nisi vitae nisl ornare tristique.
Proin vulputate id justo non aliquet.

Duis eu arcu pharetra, laoreet nunc at, pharetra sapien. Nulla eu libero diam.
Donec euismod dapibus ligula, sit amet hendrerit neque vulputate ac.`

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "FCM", "1234", "",
		map[string]any{
			configKey:   "FCMKey",
			configTitle: "FCMTitle",
		}),
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "FCM", "1234", "",
		map[string]any{
			configKey:          "FCMKey",
			configNotification: true,
			configTitle:        "FCMTitle",
		}),
}

var testCases = []IncomingTestCase{
	{
		Label:                 "Receive Valid Message",
		URL:                   receiveURL,
		Data:                  "from=12345&date=2017-01-01T08:50:00.000&fcm_token=token&name=fred&msg=hello+world",
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Accepted",
		ExpectedMsgText:       Sp("hello world"),
		ExpectedURN:           "fcm:12345",
		ExpectedDate:          time.Date(2017, 1, 1, 8, 50, 0, 0, time.UTC),
		ExpectedURNAuthTokens: map[urns.URN]map[string]string{"fcm:12345": {"default": "token"}},
		ExpectedContactName:   Sp("fred"),
	},
	{
		Label:                "Receive Invalid Date",
		URL:                  receiveURL,
		Data:                 "from=12345&date=yo&fcm_token=token&name=fred&msg=hello+world",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "unable to parse date",
	},
	{
		Label:                "Receive Missing From",
		URL:                  receiveURL,
		Data:                 "date=2017-01-01T08:50:00.000&fcm_token=token&name=fred&msg=hello+world",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "field 'from' required",
	},
	{
		Label:                "Receive Valid Register",
		URL:                  registerURL,
		Data:                 "urn=12345&fcm_token=token&name=fred",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "contact_uuid",
	},
	{
		Label:                "Receive Missing URN",
		URL:                  registerURL,
		Data:                 "fcm_token=token&name=fred",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "field 'urn' required",
	},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

// setSendURL takes care of setting the base_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	sendURL = s.URL
}

var notificationSendTestCases = []OutgoingTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message",
		MsgURN:              "fcm:250788123123",
		MsgURNAuth:          "auth1",
		MockResponseBody:    `{"success":1, "multicast_id": 123456}`,
		MockResponseStatus:  200,
		ExpectedHeaders:     map[string]string{"Authorization": "key=FCMKey"},
		ExpectedRequestBody: `{"data":{"type":"rapidpro","title":"FCMTitle","message":"Simple Message","message_id":10,"session_status":""},"notification":{"title":"FCMTitle","body":"Simple Message"},"content_available":true,"to":"auth1","priority":"high"}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "123456",
		SendPrep:            setSendURL,
	},
}

var sendTestCases = []OutgoingTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message",
		MsgURN:              "fcm:250788123123",
		MsgURNAuth:          "auth1",
		MockResponseBody:    `{"success":1, "multicast_id": 123456}`,
		MockResponseStatus:  200,
		ExpectedHeaders:     map[string]string{"Authorization": "key=FCMKey"},
		ExpectedRequestBody: `{"data":{"type":"rapidpro","title":"FCMTitle","message":"Simple Message","message_id":10,"session_status":""},"content_available":false,"to":"auth1","priority":"high"}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "123456",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Long Message",
		MsgText:             longMsg,
		MsgURN:              "fcm:250788123123",
		MsgURNAuth:          "auth1",
		MockResponseBody:    `{"success":1, "multicast_id": 123456}`,
		MockResponseStatus:  200,
		ExpectedHeaders:     map[string]string{"Authorization": "key=FCMKey"},
		ExpectedRequestBody: `{"data":{"type":"rapidpro","title":"FCMTitle","message":"ate ac.","message_id":10,"session_status":""},"content_available":false,"to":"auth1","priority":"high"}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "123456",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Quick Reply",
		MsgText:             "Simple Message",
		MsgURN:              "fcm:250788123123",
		MsgURNAuth:          "auth1",
		MsgQuickReplies:     []string{"yes", "no"},
		MsgAttachments:      []string{"image/jpeg:https://foo.bar"},
		MockResponseBody:    `{"success":1, "multicast_id": 123456}`,
		MockResponseStatus:  200,
		ExpectedHeaders:     map[string]string{"Authorization": "key=FCMKey"},
		ExpectedRequestBody: `{"data":{"type":"rapidpro","title":"FCMTitle","message":"Simple Message\nhttps://foo.bar","message_id":10,"session_status":"","quick_replies":["yes","no"]},"content_available":false,"to":"auth1","priority":"high"}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "123456",
		SendPrep:            setSendURL,
	},
	{
		Label:              "Error",
		MsgText:            "Error",
		MsgURN:             "fcm:250788123123",
		MsgURNAuth:         "auth1",
		MockResponseBody:   `{ "success": 0 }`,
		MockResponseStatus: 200,
		ExpectedMsgStatus:  "E",
		ExpectedErrors:     []*courier.ChannelError{courier.ErrorResponseValueUnexpected("success", "1")},
		SendPrep:           setSendURL,
	},
	{
		Label:              "No Multicast ID",
		MsgText:            "Error",
		MsgURN:             "fcm:250788123123",
		MsgURNAuth:         "auth1",
		MockResponseBody:   `{ "success": 1 }`,
		MockResponseStatus: 200,
		ExpectedMsgStatus:  "E",
		ExpectedErrors:     []*courier.ChannelError{courier.ErrorResponseValueMissing("multicast_id")},
		SendPrep:           setSendURL,
	},
	{
		Label:              "Request Error",
		MsgText:            "Error",
		MsgURN:             "fcm:250788123123",
		MsgURNAuth:         "auth1",
		MockResponseBody:   `{ "success": 0 }`,
		MockResponseStatus: 500,
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
}

func TestOutgoing(t *testing.T) {
	RunOutgoingTestCases(t, testChannels[0], newHandler(), sendTestCases, []string{"FCMKey"}, nil)
	RunOutgoingTestCases(t, testChannels[1], newHandler(), notificationSendTestCases, []string{"FCMKey"}, nil)
}
