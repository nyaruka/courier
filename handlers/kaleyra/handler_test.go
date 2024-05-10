package kaleyra

import (
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

const (
	channelUUID      = "8eb23e93-5ecb-45ba-b726-3b064e0c568c"
	receiveMsgURL    = "/c/kwa/" + channelUUID + "/receive"
	receiveStatusURL = "/c/kwa/" + channelUUID + "/status"
)

var incomingCases = []IncomingTestCase{
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
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "58f86fab-85c5-4f7c-9b68-9c323248afc4:0", Status: courier.MsgStatusRead}},
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
	chs := []courier.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "KWA", "250788383383", "",
			[]string{urns.WhatsApp.Prefix},
			map[string]any{configAccountSID: "SID", configApiKey: "123456"},
		),
	}

	RunIncomingTestCases(t, chs, newHandler(), incomingCases)
}

var sendTestCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message",
		MsgURN:  "whatsapp:14133881111",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.kaleyra.io/v1/SID/messages": {httpx.NewMockResponse(200, nil, []byte(`{"id":"58f86fab-85c5-4f7c-9b68-9c323248afc4:0"}`))},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{"Content-type": "application/x-www-form-urlencoded"},
				Body:    "api-key=123456&body=Simple+Message&callback_url=https%3A%2F%2Flocalhost%2Fc%2Fkwa%2F8eb23e93-5ecb-45ba-b726-3b064e0c568c%2Fstatus&channel=WhatsApp&from=250788383383&to=14133881111&type=text",
			},
		},
		ExpectedExtIDs: []string{"58f86fab-85c5-4f7c-9b68-9c323248afc4:0"},
	},
	{
		Label:   "URL Send",
		MsgText: "foo https://foo.bar bar",
		MsgURN:  "whatsapp:14133881111",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.kaleyra.io/v1/SID/messages": {httpx.NewMockResponse(200, nil, []byte(`{"id":"58f86fab-85c5-4f7c-9b68-9c323248afc4:0"}`))},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{"Content-type": "application/x-www-form-urlencoded"},
				Body:    "api-key=123456&body=foo+https%3A%2F%2Ffoo.bar+bar&callback_url=https%3A%2F%2Flocalhost%2Fc%2Fkwa%2F8eb23e93-5ecb-45ba-b726-3b064e0c568c%2Fstatus&channel=WhatsApp&from=250788383383&preview_url=true&to=14133881111&type=text",
			},
		},
		ExpectedExtIDs: []string{"58f86fab-85c5-4f7c-9b68-9c323248afc4:0"},
	},
	{
		Label:   "Plain Send Error",
		MsgText: "Error",
		MsgURN:  "whatsapp:14133881112",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.kaleyra.io/v1/SID/messages": {httpx.NewMockResponse(400, nil, []byte(`{"error":{"to":"invalid number"}}`))},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{"Content-type": "application/x-www-form-urlencoded"},
				Body:    "api-key=123456&body=Error&callback_url=https%3A%2F%2Flocalhost%2Fc%2Fkwa%2F8eb23e93-5ecb-45ba-b726-3b064e0c568c%2Fstatus&channel=WhatsApp&from=250788383383&to=14133881112&type=text",
			},
		},
		ExpectedError: courier.ErrResponseStatus,
	},
	{
		Label:          "Medias Send",
		MsgText:        "Medias",
		MsgAttachments: []string{"image/jpg:https://foo.bar/image.jpg", "image/png:https://foo.bar/video.mp4"},
		MsgURN:         "whatsapp:14133881111",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.kaleyra.io/v1/SID/messages": {
				httpx.NewMockResponse(200, nil, []byte(`{"id":"f75fbe1e-a0c0-4923-96e8-5043aa617b2b:0"}`)),
				httpx.NewMockResponse(200, nil, []byte(`{"id":"f75fbe1e-a0c0-4923-96e8-5043aa617b2b:0"}`)),
			},
			"https://foo.bar/image.jpg": {
				httpx.NewMockResponse(200, nil, []byte(`image bytes`)),
			},
			"https://foo.bar/video.mp4": {
				httpx.NewMockResponse(200, nil, []byte(`video bytes`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{},
			{BodyContains: "image bytes"},
			{},
			{BodyContains: "video bytes"},
		},
		ExpectedExtIDs: []string{"f75fbe1e-a0c0-4923-96e8-5043aa617b2b:0"},
	},
	{
		Label:          "Media Send Error",
		MsgText:        "Medias",
		MsgAttachments: []string{"image/jpg:https://foo.bar/image.jpg", "image/png:https://foo.bar/video.wmv"},
		MsgURN:         "whatsapp:14133881111",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.kaleyra.io/v1/SID/messages": {
				httpx.NewMockResponse(200, nil, []byte(`{"id":"f75fbe1e-a0c0-4923-96e8-5043aa617b2b:0"}`)),
				httpx.NewMockResponse(400, nil, []byte(`{"error":{"media":"invalid media type"}}`)),
			},
			"https://foo.bar/image.jpg": {
				httpx.NewMockResponse(200, nil, []byte(`image bytes`)),
			},
			"https://foo.bar/video.wmv": {
				httpx.NewMockResponse(200, nil, []byte(`video bytes`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{},
			{BodyContains: "image bytes"},
			{},
			{BodyContains: "video bytes"},
		},
		ExpectedError: courier.ErrResponseStatus,
	},
}

func TestOutgoing(t *testing.T) {
	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "KWA", "250788383383", "",
		[]string{urns.WhatsApp.Prefix},
		map[string]any{configAccountSID: "SID", configApiKey: "123456"},
	)

	RunOutgoingTestCases(t, ch, newHandler(), sendTestCases, []string{"123456"}, nil)
}
