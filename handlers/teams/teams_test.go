package teams

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/urns"
	"gopkg.in/go-playground/assert.v1"
)

var access_token = "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6ImFiYzEyMyJ9.eyJpc3MiOiJodHRwczovL2FwaS5ib3RmcmFtZXdvcmsuY29tIiwic2VydmljZXVybCI6Imh0dHBzOi8vc21iYS50cmFmZmljbWFuYWdlci5uZXQvYnIvIiwiYXVkIjoiMTU5NiJ9.hqKdNdlB0NX6jtwkN96jI-kIiWTWPDIA1K7oo56tVsRBmMycyNNHrsGbKrEw7dccLjATmimpk4x0J_umaJZ5mcK5S5F7b4hkGHFIRWc4vaMjxCl6VSJ6E6DTRnQwfrfTF0AerHSO1iABI2YAlbdMV3ahxGzzNkaqnIX496G2IKwiYziOumo4M0gfOt-MqNkOJKvnSRfB7pikSATaSQiaFmrA5A8bH0AbaM9znPIRxHyrKqlFlrpWkPSiUPOS3aHQeD8kVGk7RNEWtOk26sXfUIjHp8ZYExIClBEmc6QPAf2-FAuwsw-S8YDLwsiycJ0gEO8MYPZWn8gXR_sVIwLMMg"

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "TM", "2022", "US", map[string]interface{}{"auth_token": access_token, "tenantID": "cba321", "botID": "0123", "appID": "1596"}),
}

var helloMsg = `{
	"channelId": "msteams",
	"conversation": {
		"converstaionType": "personal",
		"id": "a:2811",
		"tenantId": "cba321"
	},
	"id": "56834",
	"timestamp": "2022-06-06T16:51:00.0000000Z",
	"serviceUrl": "https://smba.trafficmanager.net/br/",
	"text":"Hello World",
	"type":"message"
}`

var attachment = `{
	"channelId": "msteams",
	"conversation": {
		"converstaionType": "personal",
		"id": "a:2811",
		"tenantId": "cba321"
	},
	"id": "56834",
	"timestamp": "2022-06-06T16:51:00.0000000Z",
	"serviceUrl": "https://smba.trafficmanager.net/br/",
	"text":"Hello World",
	"type":"message",
	"attachments":[
		{
			"contentType": "image",
			"contentUrl": "https://image-url/foo.png",
			"name": "foo.png"
		}
	]
}`

var attachmentVideo = `{
	"channelId": "msteams",
	"conversation": {
		"converstaionType": "personal",
		"id": "a:2811",
		"tenantId": "cba321"
	},
	"id": "56834",
	"timestamp": "2022-06-06T16:51:00.0000000Z",
	"serviceUrl": "https://smba.trafficmanager.net/br/",
	"text":"Hello World",
	"type":"message",
	"attachments":[
		{
			"contentType": "video/mp4",
			"contentUrl": "https://video-url/foo.mp4",
			"name": "foo.png"
		}
	]
}`

var attachmentDocument = `{
	"channelId": "msteams",
	"conversation": {
		"converstaionType": "personal",
		"id": "a:2811",
		"tenantId": "cba321"
	},
	"id": "56834",
	"timestamp": "2022-06-06T16:51:00.0000000Z",
	"serviceUrl": "https://smba.trafficmanager.net/br/",
	"text":"Hello World",
	"type":"message",
	"attachments":[
		{
			"contentType": "application/pdf",
			"contentUrl": "https://document-url/foo.pdf",
			"name": "foo.png"
		}
	]
}`

var conversationUpdate = `{
	"channelId": "msteams",
	"id": "56834",
	"timestamp": "2022-06-06T16:51:00.0000000Z",
	"serviceUrl": "https://smba.trafficmanager.net/br/",
	"type":"conversationUpdate",
	"membersAdded": [{
		"id":"4569",
		"name": "Joe",
		"role": "user"
	}]
}`

var messageReaction = `{
	"channelId": "msteams",
	"id": "56834",
	"timestamp": "2022-06-06T16:51:00.0000000Z",
	"serviceUrl": "https://smba.trafficmanager.net/br/",
	"type":"messageReaction"
}`

var testCases = []ChannelHandleTestCase{
	{
		Label:             "Receive Message",
		URL:               "/c/tm/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive",
		Data:              helloMsg,
		Status:            200,
		Response:          "Handled",
		Text:              Sp("Hello World"),
		URN:               Sp("teams:2811:smba.trafficmanager.net/br/"),
		ExternalID:        Sp("56834"),
		Date:              Tp(time.Date(2022, 6, 6, 16, 51, 00, 0000000, time.UTC)),
		Headers:           map[string]string{"Authorization": "Bearer " + access_token},
		NoQueueErrorCheck: true,
	},
	{
		Label:             "Receive Attachment Image",
		URL:               "/c/tm/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive",
		Data:              attachment,
		Status:            200,
		Response:          "Handled",
		Text:              Sp("Hello World"),
		Attachments:       []string{"https://image-url/foo.png"},
		URN:               Sp("teams:2811:smba.trafficmanager.net/br/"),
		ExternalID:        Sp("56834"),
		Date:              Tp(time.Date(2022, 6, 6, 16, 51, 00, 0000000, time.UTC)),
		Headers:           map[string]string{"Authorization": "Bearer " + access_token},
		NoQueueErrorCheck: true,
	},
	{
		Label:             "Receive Attachment Video",
		URL:               "/c/tm/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive",
		Data:              attachmentVideo,
		Status:            200,
		Response:          "Handled",
		Text:              Sp("Hello World"),
		Attachments:       []string{"https://video-url/foo.mp4"},
		URN:               Sp("teams:2811:smba.trafficmanager.net/br/"),
		ExternalID:        Sp("56834"),
		Date:              Tp(time.Date(2022, 6, 6, 16, 51, 00, 0000000, time.UTC)),
		Headers:           map[string]string{"Authorization": "Bearer " + access_token},
		NoQueueErrorCheck: true,
	},
	{
		Label:             "Receive Attachment Document",
		URL:               "/c/tm/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive",
		Data:              attachmentDocument,
		Status:            200,
		Response:          "Handled",
		Text:              Sp("Hello World"),
		Attachments:       []string{"https://document-url/foo.pdf"},
		URN:               Sp("teams:2811:smba.trafficmanager.net/br/"),
		ExternalID:        Sp("56834"),
		Date:              Tp(time.Date(2022, 6, 6, 16, 51, 00, 0000000, time.UTC)),
		Headers:           map[string]string{"Authorization": "Bearer " + access_token},
		NoQueueErrorCheck: true,
	},
	{
		Label:             "Receive Message Reaction",
		URL:               "/c/tm/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive",
		Data:              messageReaction,
		Status:            200,
		Text:              nil,
		URN:               Sp(""),
		Response:          "ignoring messageReaction",
		Headers:           map[string]string{"Authorization": "Bearer " + access_token},
		NoQueueErrorCheck: true,
	},
	{
		Label:             "Receive Conversation Update",
		URL:               "/c/tm/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive",
		Data:              "",
		Text:              nil,
		Status:            200,
		Response:          "Handled",
		Headers:           map[string]string{"Authorization": "Bearer " + access_token},
		NoQueueErrorCheck: true,
	},
}

func TestHandler(t *testing.T) {
	tmService := buildMockTeams()
	newTestCases := newConversationUpdateTC(tmService.URL, testCases)
	jwks_url := buildMockJwksURL()
	RunChannelTestCases(t, testChannels, newHandler(), newTestCases)
	jwks_url.Close()
	tmService.Close()

}

func buildMockJwksURL() *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte(`{"keys":[{"kty":"RSA","use":"sig","kid":"abc123","x5t":"abc123","n":"abcd","e":"AQAB","endorsements":["msteams"]}]}`))
	}))

	jwksURI = server.URL

	return server
}

func buildMockTeams() *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accessToken := r.Header.Get("Authorization")
		tokenH := strings.Replace(accessToken, "Bearer ", "", 1)
		// payload := r.GetBody
		defer r.Body.Close()

		// invalid auth token
		if tokenH != access_token {
			http.Error(w, "invalid auth token", 400)
		}

		if r.URL.Path == "/v3/conversations" {
			w.Header().Add("Content-Type", "application/json")
			w.Write([]byte(`{"id":"a:2811"}`))
		}

		if r.URL.Path == "/v3/conversations/a:2022/activities" {
			byteBody, err := io.ReadAll(r.Body)
			if err != nil {
				log.Fatal(err)
			}
			text, err := jsonparser.GetString(byteBody, "text")
			if err != nil {
				log.Fatal(err)
			}
			if text == "Error" {
				w.Header().Add("Content-Type", "application/json")
				w.Write([]byte(`{"is_error": true}`))
			}
			w.Header().Add("Content-Type", "application/json")
			w.Write([]byte(`{"id":"1234567890"}`))
		}

		if r.URL.Path == "/v3/conversations/a:2022/members" {
			w.Write([]byte(`[{"givenName": "John","surname": "Doe"}]`))
		}
	}))

	return server
}

func newConversationUpdateTC(newUrl string, testCase []ChannelHandleTestCase) []ChannelHandleTestCase {
	casesWithMockedUrls := make([]ChannelHandleTestCase, len(testCases))
	for i, tc := range testCases {
		mockedCase := tc
		if mockedCase.Label == "Receive Conversation Update" {
			mockedCase.Data = strings.Replace(conversationUpdate, "https://smba.trafficmanager.net/br/", newUrl, 1)
		}
		casesWithMockedUrls[i] = mockedCase
	}
	return casesWithMockedUrls
}

var defaultSendTestCases = []ChannelSendTestCase{
	{
		Label:  "Plain Send",
		Text:   "Simple Message",
		URN:    "teams:2022:smba.trafficmanager.net/br/",
		Status: "W", ExternalID: "1234567890",
		ResponseBody: `{id:"1234567890"}`, ResponseStatus: 200,
	},
	{Label: "Send Photo",
		URN: "teams:2022:smba.trafficmanager.net/br/", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status: "W", ExternalID: "1234567890",
		ResponseBody: `{"id": "1234567890"}`, ResponseStatus: 200,
	},
	{Label: "Send Video",
		URN: "teams:2022:smba.trafficmanager.net/br/", Attachments: []string{"video/mp4:https://foo.bar/video.mp4"},
		Status: "W", ExternalID: "1234567890",
		ResponseBody: `{"id": "1234567890"}`, ResponseStatus: 200,
	},
	{Label: "Send Document",
		URN: "teams:2022:smba.trafficmanager.net/br/", Attachments: []string{"application/pdf:https://foo.bar/document.pdf"},
		Status: "W", ExternalID: "1234567890",
		ResponseBody: `{"id": "1234567890"}`, ResponseStatus: 200,
	},
	{Label: "Error",
		Text: "Error", URN: "teams:2022:smba.trafficmanager.net/br/",
		Status:       "E",
		ResponseBody: `{"is_error": true}`, ResponseStatus: 400,
	},
}

func newSendTestCases(testSendCases []ChannelSendTestCase, url string) []ChannelSendTestCase {
	var newtestSendCases []ChannelSendTestCase
	for _, tc := range testSendCases {
		spTC := strings.Split(tc.URN, ":")
		newURN := spTC[0] + ":" + spTC[1] + ":" + url + "/"
		tc.URN = newURN
		newtestSendCases = append(newtestSendCases, tc)
	}
	return newtestSendCases
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TM", "2022", "US",
		map[string]interface{}{courier.ConfigAuthToken: access_token, "tenantID": "cba321", "botID": "0123", "appID": "1596"})

	serviceTM := buildMockTeams()
	url := strings.Split(serviceTM.URL, "http://")
	newSendTestCases := newSendTestCases(defaultSendTestCases, url[1])
	RunChannelSendTestCases(t, defaultChannel, newHandler(), newSendTestCases, nil)
	serviceTM.Close()
}

func TestDescribe(t *testing.T) {
	server := buildMockTeams()
	url := strings.Split(server.URL, "http://")

	handler := newHandler().(courier.URNDescriber)
	tcs := []struct {
		urn      urns.URN
		metadata map[string]string
	}{{urns.URN("teams:2022:" + string(url[1]) + "/"), map[string]string{"name": "John Doe"}}}

	for _, tc := range tcs {
		metadata, _ := handler.DescribeURN(context.Background(), testChannels[0], tc.urn)
		assert.Equal(t, metadata, tc.metadata)
	}
	server.Close()
}
