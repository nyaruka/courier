package kaleyra

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

const (
	channelUUID      = "8eb23e93-5ecb-45ba-b726-3b064e0c568c"
	receiveMsgURL    = "/c/kwa/" + channelUUID + "/receive"
	receiveStatusURL = "/c/kwa/" + channelUUID + "/status"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "KWA", "250788383383", "",
		map[string]interface{}{
			configAccountSID: "SID",
			configApiKey:     "123456",
		},
	),
}

var testCases = []ChannelHandleTestCase{
	{
		Label:       "Receive Msg",
		URL:         receiveMsgURL + "?created_at=1603914166&type=text&from=14133881111&name=John%20Cruz&body=Hello%20World",
		Name:        Sp("John Cruz"),
		URN:         Sp("whatsapp:14133881111"),
		Text:        Sp("Hello World"),
		Attachments: []string{},
		Status:      200,
		Response:    "Accepted",
	},
	{
		Label:      "Receive Media",
		URL:        receiveMsgURL + "?created_at=1603914166&type=image&from=14133881111&name=John%20Cruz&media_url=https://link.to/image.jpg",
		Name:       Sp("John Cruz"),
		URN:        Sp("whatsapp:14133881111"),
		Text:       Sp(""),
		Attachment: Sp("https://link.to/image.jpg"),
		Status:     200,
		Response:   "Accepted",
	},
	{
		Label:    "Receive Empty",
		URL:      receiveMsgURL + "?created_at=1603914166&type=text&from=14133881111&name=John%20Cruz",
		Status:   400,
		Response: "no text or media",
	},
	{
		Label:    "Receive Invalid CreatedAt",
		URL:      receiveMsgURL + "?created_at=nottimestamp&type=text&from=14133881111&name=John%20Cruz&body=Hi",
		Name:     Sp("John Cruz"),
		Status:   400,
		Response: "invalid created_at",
	},
	{
		Label:    "Receive Invalid Type",
		URL:      receiveMsgURL + "?created_at=1603914166&type=sticker&from=14133881111&name=John%20Cruz",
		Status:   200,
		Response: "unknown message type",
	},
	{
		Label:    "Receive Invalid From",
		URL:      receiveMsgURL + "?created_at=1603914166&type=text&from=notnumber&name=John%20Cruz&body=Hi",
		Name:     Sp("John Cruz"),
		Status:   400,
		Response: "invalid whatsapp id",
	},
	{
		Label:    "Receive Blank From",
		URL:      receiveMsgURL,
		Status:   400,
		Response: "field 'from' required",
	},
	{
		Label:      "Receive Valid Status",
		URL:        receiveStatusURL + "?id=58f86fab-85c5-4f7c-9b68-9c323248afc4%3A0&status=read",
		ExternalID: Sp("58f86fab-85c5-4f7c-9b68-9c323248afc4:0"),
		MsgStatus:  Sp("D"),
		Status:     200,
		Response:   `"type":"status"`,
	},
	{
		Label:      "Receive Invalid Status",
		URL:        receiveStatusURL + "?id=58f86fab-85c5-4f7c-9b68-9c323248afc4%3A0&status=deleted",
		ExternalID: Sp("58f86fab-85c5-4f7c-9b68-9c323248afc4:0"),
		MsgStatus:  Sp("D"),
		Status:     200,
		Response:   "unknown status",
	},
	{
		Label:      "Receive Blank status",
		URL:        receiveStatusURL,
		ExternalID: Sp("58f86fab-85c5-4f7c-9b68-9c323248afc4:0"),
		Status:     400,
		Response:   "field 'status' required",
	},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	baseURL = s.URL
}

var sendTestCases = []ChannelSendTestCase{
	{
		Label:          "Plain Send",
		Text:           "Simple Message",
		URN:            "whatsapp:14133881111",
		Status:         "W",
		ExternalID:     "58f86fab-85c5-4f7c-9b68-9c323248afc4:0",
		Path:           "/v1/SID/messages",
		Headers:        map[string]string{"Content-type": "application/x-www-form-urlencoded"},
		RequestBody:    "api-key=123456&body=Simple+Message&callback_url=https%3A%2F%2Flocalhost%2Fc%2Fkwa%2F8eb23e93-5ecb-45ba-b726-3b064e0c568c%2Fstatus&channel=WhatsApp&from=250788383383&to=14133881111&type=text",
		ResponseStatus: 200,
		ResponseBody:   `{"id":"58f86fab-85c5-4f7c-9b68-9c323248afc4:0"}`,
		SendPrep:       setSendURL,
	},
	{
		Label:          "Unicode Send",
		Text:           "☺",
		URN:            "whatsapp:14133881111",
		Status:         "W",
		ExternalID:     "58f86fab-85c5-4f7c-9b68-9c323248afc4:0",
		Path:           "/v1/SID/messages",
		Headers:        map[string]string{"Content-type": "application/x-www-form-urlencoded"},
		RequestBody:    "api-key=123456&body=%E2%98%BA&callback_url=https%3A%2F%2Flocalhost%2Fc%2Fkwa%2F8eb23e93-5ecb-45ba-b726-3b064e0c568c%2Fstatus&channel=WhatsApp&from=250788383383&to=14133881111&type=text",
		ResponseStatus: 200,
		ResponseBody:   `{"id":"58f86fab-85c5-4f7c-9b68-9c323248afc4:0"}`,
		SendPrep:       setSendURL,
	},
	{
		Label:          "URL Send",
		Text:           "foo https://foo.bar bar",
		URN:            "whatsapp:14133881111",
		Status:         "W",
		ExternalID:     "58f86fab-85c5-4f7c-9b68-9c323248afc4:0",
		Path:           "/v1/SID/messages",
		Headers:        map[string]string{"Content-type": "application/x-www-form-urlencoded"},
		RequestBody:    "api-key=123456&body=foo+https%3A%2F%2Ffoo.bar+bar&callback_url=https%3A%2F%2Flocalhost%2Fc%2Fkwa%2F8eb23e93-5ecb-45ba-b726-3b064e0c568c%2Fstatus&channel=WhatsApp&from=250788383383&preview_url=true&to=14133881111&type=text",
		ResponseStatus: 200,
		ResponseBody:   `{"id":"58f86fab-85c5-4f7c-9b68-9c323248afc4:0"}`,
		SendPrep:       setSendURL,
	},
	{
		Label:          "Plain Send Error",
		Text:           "Error",
		URN:            "whatsapp:14133881112",
		Status:         "F",
		Path:           "/v1/SID/messages",
		Headers:        map[string]string{"Content-type": "application/x-www-form-urlencoded"},
		RequestBody:    "api-key=123456&body=Error&callback_url=https%3A%2F%2Flocalhost%2Fc%2Fkwa%2F8eb23e93-5ecb-45ba-b726-3b064e0c568c%2Fstatus&channel=WhatsApp&from=250788383383&to=14133881112&type=text",
		ResponseStatus: 400,
		ResponseBody:   `{"error":{"to":"invalid number"}}`,
		SendPrep:       setSendURL,
	},
	{
		Label:       "Medias Send",
		Text:        "Medias",
		Attachments: []string{"image/jpg:https://foo.bar/image.jpg", "image/png:https://foo.bar/video.mp4"},
		URN:         "whatsapp:14133881111",
		Status:      "W",
		ExternalID:  "f75fbe1e-a0c0-4923-96e8-5043aa617b2b:0",
		Responses: map[MockedRequest]MockedResponse{
			MockedRequest{
				Method:       "POST",
				Path:         "/v1/SID/messages",
				BodyContains: "image bytes",
			}: {
				Status: 200,
				Body:   `{"id":"58f86fab-85c5-4f7c-9b68-9c323248afc4:0"}`,
			},
			MockedRequest{
				Method:       "POST",
				Path:         "/v1/SID/messages",
				BodyContains: "video bytes",
			}: {
				Status: 200,
				Body:   `{"id":"f75fbe1e-a0c0-4923-96e8-5043aa617b2b:0"}`,
			},
		},
		SendPrep: setSendURL,
	},
	{
		Label:       "Media Send Error",
		Text:        "Medias",
		Attachments: []string{"image/jpg:https://foo.bar/image.jpg", "image/png:https://foo.bar/video.wmv"},
		URN:         "whatsapp:14133881111",
		Status:      "F",
		Responses: map[MockedRequest]MockedResponse{
			MockedRequest{
				Method:       "POST",
				Path:         "/v1/SID/messages",
				BodyContains: "image bytes",
			}: {
				Status: 200,
				Body:   `{"id":"58f86fab-85c5-4f7c-9b68-9c323248afc4:0"}`,
			},
			MockedRequest{
				Method:       "POST",
				Path:         "/v1/SID/messages",
				BodyContains: "video bytes",
			}: {
				Status: 400,
				Body:   `{"error":{"media":"invalid media type"}}`,
			},
		},
		SendPrep: setSendURL,
	},
}

func mockAttachmentURLs(mediaServer *httptest.Server, testCases []ChannelSendTestCase) []ChannelSendTestCase {
	casesWithMockedUrls := make([]ChannelSendTestCase, len(testCases))

	for i, testCase := range testCases {
		mockedCase := testCase

		for j, attachment := range testCase.Attachments {
			mockedCase.Attachments[j] = strings.Replace(attachment, "https://foo.bar", mediaServer.URL, 1)
		}
		casesWithMockedUrls[i] = mockedCase
	}
	return casesWithMockedUrls
}

func TestSending(t *testing.T) {
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

	RunChannelSendTestCases(t, testChannels[0], newHandler(), mockedSendTestCases, nil)
}

var urlTestCases = []struct {
	text  string
	valid bool
}{
	// supported by whatsapp
	{"http://foo.com/blah_blah", true},
	{"http://foo.com/blah_blah/", true},
	{"http://foo.com/blah_blah_(wikipedia)", true},
	{"http://foo.com/blah_blah_(wikipedia)_(again)", true},
	{"http://www.example.com/wpstyle/?p=364", true},
	{"https://www.example.com/foo/?bar=baz&inga=42&quux", true},
	{"http://userid:password@example.com:8080", true},
	{"http://userid:password@example.com:8080/", true},
	{"http://userid@example.com", true},
	{"http://userid@example.com/", true},
	{"http://userid@example.com:8080", true},
	{"http://userid@example.com:8080/", true},
	{"http://userid:password@example.com", true},
	{"http://userid:password@example.com/", true},
	{"http://foo.com/blah_(wikipedia)#cite-1", true},
	{"http://foo.com/blah_(wikipedia)_blah#cite-1", true},
	{"http://foo.com/unicode_(✪)_in_parens", true},
	{"http://foo.com/(something)?after=parens", true},
	{"http://code.google.com/events/#&product=browser", true},
	{"http://foo.bar/?q=Test%20URL-encoded%20stuff", true},
	{"http://1337.net", true},
	{"http://a.b-c.de", true},
	{"http://foo.bar?q=Spaces foo bar", true},
	{"http://foo.bar/foo(bar)baz quux", true},
	{"http://a.b--c.de/", true},
	{"http://www.foo.bar./", true},
	// not supported by whatsapp
	{"http://✪df.ws/123", false},
	{"http://142.42.1.1/", false},
	{"http://142.42.1.1:8080/", false},
	{"http://➡.ws/䨹", false},
	{"http://⌘.ws", false},
	{"http://⌘.ws/", false},
	{"http://☺.damowmow.com/", false},
	{"ftp://foo.bar/baz", false},
	{"http://مثال.إختبار", false},
	{"http://例子.测试", false},
	{"http://उदाहरण.परीक्षा", false},
	{"http://-.~_!$&'()*+,;=:%40:80%2f::::::@example.com", false},
	{"http://223.255.255.254", false},
	{"https://foo_bar.example.com/", false},
	{"http://", false},
	{"http://.", false},
	{"http://..", false},
	{"http://../", false},
	{"http://?", false},
	{"http://??", false},
	{"http://??/", false},
	{"http://#", false},
	{"http://##", false},
	{"http://##/", false},
	{"//", false},
	{"//a", false},
	{"///a", false},
	{"///", false},
	{"http:///a", false},
	{"foo.com", false},
	{"rdar://1234", false},
	{"h://test", false},
	{"http:// shouldfail.com", false},
	{":// should fail", false},
	{"ftps://foo.bar/", false},
	{"http://-error-.invalid/", false},
	{"http://-a.b.co", false},
	{"http://a.b-.co", false},
	{"http://0.0.0.0", false},
	{"http://10.1.1.0", false},
	{"http://10.1.1.255", false},
	{"http://224.1.1.1", false},
	{"http://1.1.1.1.1", false},
	{"http://123.123.123", false},
	{"http://3628126748", false},
	{"http://.www.foo.bar/", false},
	{"http://.www.foo.bar./", false},
	{"http://10.1.1.1", false},
	{"http://10.1.1.254", false},
}

func TestUrlRegex(t *testing.T) {
	for _, c := range urlTestCases {
		if valid := urlRegex.MatchString(c.text); valid != c.valid {
			t.Errorf(`
				ERROR:	Not equal:
						text = %#v
						expected: %t
						actual	: %t`, c.text, c.valid, valid)
		}
	}
}
