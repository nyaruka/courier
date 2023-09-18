package kaleyra

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
)

const (
	channelUUID      = "8eb23e93-5ecb-45ba-b726-3b064e0c568c"
	receiveMsgURL    = "/c/kwa/" + channelUUID + "/receive"
	receiveStatusURL = "/c/kwa/" + channelUUID + "/status"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "KWA", "250788383383", "",
		map[string]any{
			configAccountSID: "SID",
			configApiKey:     "123456",
		},
	),
}

var testCases = []IncomingTestCase{
	{
		Label:                "Receive Msg",
		URL:                  receiveMsgURL + "?created_at=1603914166&type=text&from=14133881111&name=John%20Cruz&body=Hello%20World",
		ExpectedContactName:  Sp("John Cruz"),
		ExpectedURN:          "whatsapp:14133881111",
		ExpectedMsgText:      Sp("Hello World"),
		ExpectedAttachments:  []string{},
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
	},
	{
		Label:                "Receive Media",
		URL:                  receiveMsgURL + "?created_at=1603914166&type=image&from=14133881111&name=John%20Cruz&media_url=https://link.to/image.jpg",
		ExpectedContactName:  Sp("John Cruz"),
		ExpectedURN:          "whatsapp:14133881111",
		ExpectedMsgText:      Sp(""),
		ExpectedAttachments:  []string{"https://link.to/image.jpg"},
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
	},
	{
		Label:                "Receive Empty",
		URL:                  receiveMsgURL + "?created_at=1603914166&type=text&from=14133881111&name=John%20Cruz",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "no text or media",
	},
	{
		Label:                "Receive Invalid CreatedAt",
		URL:                  receiveMsgURL + "?created_at=nottimestamp&type=text&from=14133881111&name=John%20Cruz&body=Hi",
		ExpectedContactName:  Sp("John Cruz"),
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "invalid created_at",
	},
	{
		Label:                "Receive Invalid Type",
		URL:                  receiveMsgURL + "?created_at=1603914166&type=sticker&from=14133881111&name=John%20Cruz",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "unknown message type",
	},
	{
		Label:                "Receive Invalid From",
		URL:                  receiveMsgURL + "?created_at=1603914166&type=text&from=notnumber&name=John%20Cruz&body=Hi",
		ExpectedContactName:  Sp("John Cruz"),
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "invalid whatsapp id",
	},
	{
		Label:                "Receive Blank From",
		URL:                  receiveMsgURL,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "field 'from' required",
	},
	{
		Label:                "Receive Valid Status",
		URL:                  receiveStatusURL + "?id=58f86fab-85c5-4f7c-9b68-9c323248afc4%3A0&status=read",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"type":"status"`,
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "58f86fab-85c5-4f7c-9b68-9c323248afc4:0", Status: courier.MsgStatusDelivered}},
	},
	{
		Label:                "Receive Invalid Status",
		URL:                  receiveStatusURL + "?id=58f86fab-85c5-4f7c-9b68-9c323248afc4%3A0&status=deleted",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "unknown status",
	},
	{
		Label:                "Receive Blank status",
		URL:                  receiveStatusURL,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "field 'status' required",
	},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	baseURL = s.URL
}

var sendTestCases = []OutgoingTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message",
		MsgURN:              "whatsapp:14133881111",
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "58f86fab-85c5-4f7c-9b68-9c323248afc4:0",
		ExpectedRequestPath: "/v1/SID/messages",
		ExpectedHeaders:     map[string]string{"Content-type": "application/x-www-form-urlencoded"},
		ExpectedRequestBody: "api-key=123456&body=Simple+Message&callback_url=https%3A%2F%2Flocalhost%2Fc%2Fkwa%2F8eb23e93-5ecb-45ba-b726-3b064e0c568c%2Fstatus&channel=WhatsApp&from=250788383383&to=14133881111&type=text",
		MockResponseStatus:  200,
		MockResponseBody:    `{"id":"58f86fab-85c5-4f7c-9b68-9c323248afc4:0"}`,
		SendPrep:            setSendURL,
	},
	{
		Label:               "Unicode Send",
		MsgText:             "☺",
		MsgURN:              "whatsapp:14133881111",
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "58f86fab-85c5-4f7c-9b68-9c323248afc4:0",
		ExpectedRequestPath: "/v1/SID/messages",
		ExpectedHeaders:     map[string]string{"Content-type": "application/x-www-form-urlencoded"},
		ExpectedRequestBody: "api-key=123456&body=%E2%98%BA&callback_url=https%3A%2F%2Flocalhost%2Fc%2Fkwa%2F8eb23e93-5ecb-45ba-b726-3b064e0c568c%2Fstatus&channel=WhatsApp&from=250788383383&to=14133881111&type=text",
		MockResponseStatus:  200,
		MockResponseBody:    `{"id":"58f86fab-85c5-4f7c-9b68-9c323248afc4:0"}`,
		SendPrep:            setSendURL,
	},
	{
		Label:               "URL Send",
		MsgText:             "foo https://foo.bar bar",
		MsgURN:              "whatsapp:14133881111",
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "58f86fab-85c5-4f7c-9b68-9c323248afc4:0",
		ExpectedRequestPath: "/v1/SID/messages",
		ExpectedHeaders:     map[string]string{"Content-type": "application/x-www-form-urlencoded"},
		ExpectedRequestBody: "api-key=123456&body=foo+https%3A%2F%2Ffoo.bar+bar&callback_url=https%3A%2F%2Flocalhost%2Fc%2Fkwa%2F8eb23e93-5ecb-45ba-b726-3b064e0c568c%2Fstatus&channel=WhatsApp&from=250788383383&preview_url=true&to=14133881111&type=text",
		MockResponseStatus:  200,
		MockResponseBody:    `{"id":"58f86fab-85c5-4f7c-9b68-9c323248afc4:0"}`,
		SendPrep:            setSendURL,
	},
	{
		Label:               "Plain Send Error",
		MsgText:             "Error",
		MsgURN:              "whatsapp:14133881112",
		ExpectedMsgStatus:   "F",
		ExpectedRequestPath: "/v1/SID/messages",
		ExpectedHeaders:     map[string]string{"Content-type": "application/x-www-form-urlencoded"},
		ExpectedRequestBody: "api-key=123456&body=Error&callback_url=https%3A%2F%2Flocalhost%2Fc%2Fkwa%2F8eb23e93-5ecb-45ba-b726-3b064e0c568c%2Fstatus&channel=WhatsApp&from=250788383383&to=14133881112&type=text",
		MockResponseStatus:  400,
		MockResponseBody:    `{"error":{"to":"invalid number"}}`,
		SendPrep:            setSendURL,
	},
	{
		Label:              "Medias Send",
		MsgText:            "Medias",
		MsgAttachments:     []string{"image/jpg:https://foo.bar/image.jpg", "image/png:https://foo.bar/video.mp4"},
		MsgURN:             "whatsapp:14133881111",
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "f75fbe1e-a0c0-4923-96e8-5043aa617b2b:0",
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method:       "POST",
				Path:         "/v1/SID/messages",
				BodyContains: "image bytes",
			}: httpx.NewMockResponse(200, nil, []byte(`{"id":"58f86fab-85c5-4f7c-9b68-9c323248afc4:0"}`)),
			{
				Method:       "POST",
				Path:         "/v1/SID/messages",
				BodyContains: "video bytes",
			}: httpx.NewMockResponse(200, nil, []byte(`{"id":"f75fbe1e-a0c0-4923-96e8-5043aa617b2b:0"}`)),
		},
		SendPrep: setSendURL,
	},
	{
		Label:             "Media Send Error",
		MsgText:           "Medias",
		MsgAttachments:    []string{"image/jpg:https://foo.bar/image.jpg", "image/png:https://foo.bar/video.wmv"},
		MsgURN:            "whatsapp:14133881111",
		ExpectedMsgStatus: "F",
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method:       "POST",
				Path:         "/v1/SID/messages",
				BodyContains: "image bytes",
			}: httpx.NewMockResponse(200, nil, []byte(`{"id":"58f86fab-85c5-4f7c-9b68-9c323248afc4:0"}`)),
			{
				Method:       "POST",
				Path:         "/v1/SID/messages",
				BodyContains: "video bytes",
			}: httpx.NewMockResponse(400, nil, []byte(`{"error":{"media":"invalid media type"}}`)),
		},
		SendPrep: setSendURL,
	},
}

func mockAttachmentURLs(mediaServer *httptest.Server, testCases []OutgoingTestCase) []OutgoingTestCase {
	casesWithMockedUrls := make([]OutgoingTestCase, len(testCases))

	for i, testCase := range testCases {
		mockedCase := testCase

		for j, attachment := range testCase.MsgAttachments {
			mockedCase.MsgAttachments[j] = strings.Replace(attachment, "https://foo.bar", mediaServer.URL, 1)
		}
		casesWithMockedUrls[i] = mockedCase
	}
	return casesWithMockedUrls
}

func TestOutgoing(t *testing.T) {
	mediaServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		defer req.Body.Close()
		res.WriteHeader(200)

		path := req.URL.Path
		if strings.Contains(path, "image") {
			res.Write([]byte("image bytes"))
		} else if strings.Contains(path, "video") {
			res.Write([]byte("video bytes"))
		} else {
			res.Write([]byte("media bytes"))
		}
	}))
	mockedSendTestCases := mockAttachmentURLs(mediaServer, sendTestCases)

	RunOutgoingTestCases(t, testChannels[0], newHandler(), mockedSendTestCases, []string{"123456"}, nil)
}
