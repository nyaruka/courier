package webchat

import (
	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"testing"
	"net/http/httptest"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
	"encoding/json"
)

var msgExample = `{
  	"text": "Hello World",
  	"from": "asldkfjpoawije",
	"attachment_url": ""
}`

var msgExampleWithAttachment = `{
  	"text": "Hello World",
  	"from": "asldkfjpoawije",
	"attachment_url": "https://assets.website-files.com/5a18dcffca1ffe0001627dc8/5fa93a867a78a15d5e8bb576_fold-img.png"
}`

var msgExampleWithoutData = `{
  	"text": "",
  	"from": "asldkfjpoawije",
	"attachment_url": ""
}`

var userPayloadExample = `{
 	"urn": "asldkfjpoawije",
 	"language": "en-US"
}`

var userWithoutURNExample = `{
 	"language": "en-US"
}`

var userInvalidPayloadExample = `{
	"test": "no data"
}`

var userWrongLangExample = `{
 	"language": "portuguese"
}`

var testCases = []ChannelHandleTestCase{
	{
		Label:    "Receive Valid Message",
		URL:      "/c/wch/8eb23e93-5ecb-45ba-b726-3b064e0c567b/receive/",
		Data:     msgExample,
		Status:   200,
		Response: "Accepted",
		Text:     Sp("Hello World"),
		URN:      Sp("ext:asldkfjpoawije"),
	},
	{
		Label:      "Receive Valid Message With Attachment",
		URL:        "/c/wch/8eb23e93-5ecb-45ba-b726-3b064e0c567b/receive/",
		Data:       msgExampleWithAttachment,
		Status:     200,
		Response:   "Accepted",
		Text:       Sp("Hello World"),
		URN:        Sp("ext:asldkfjpoawije"),
		Attachment: Sp("https://assets.website-files.com/5a18dcffca1ffe0001627dc8/5fa93a867a78a15d5e8bb576_fold-img.png"),
	},
	{
		Label:    "Receive Invalid Message",
		URL:      "/c/wch/8eb23e93-5ecb-45ba-b726-3b064e0c567b/receive/",
		Data:     msgExampleWithoutData,
		Status:   200,
		Response: "Ignored",
	},
	{
		Label:  "Register Valid Contact",
		URL:    "/c/wch/8eb23e93-5ecb-45ba-b726-3b064e0c567b/register/",
		Data:   userPayloadExample,
		Status: 200,
	},
	{
		Label:  "Register Invalid Contact",
		URL:    "/c/wch/8eb23e93-5ecb-45ba-b726-3b064e0c567b/register/",
		Data:   userWithoutURNExample,
		Status: 200,
	},
	{
		Label:  "Register Wrong Payload",
		URL:    "/c/wch/8eb23e93-5ecb-45ba-b726-3b064e0c567b/register/",
		Data:   userInvalidPayloadExample,
		Status: 200,
	},
	{
		Label:  "Register Wrong Language",
		URL:    "/c/wch/8eb23e93-5ecb-45ba-b726-3b064e0c567b/register/",
		Data:   userWrongLangExample,
		Status: 200,
	},
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	// this is actually a path, which we'll combine with the test server URL
	sendURL := c.ChannelAddress().String()
	sendURL, _ = utils.AddURLPath(s.URL, sendURL)
}

func TestHandler(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c567b", "WCH", "websocket-app.communityconnectlabs.com", "US", nil)
	defaultChannel.SetScheme(urns.ExternalScheme)
	RunChannelTestCases(t, []courier.Channel{defaultChannel}, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c567b", "WCH", "websocket-app.communityconnectlabs.com", "US", nil)
	defaultChannel.SetScheme(urns.ExternalScheme)
	RunChannelBenchmarks(b, []courier.Channel{defaultChannel}, newHandler(), testCases)
}

var defaultSendTestCases = []ChannelSendTestCase{
	{
		Label:          "Plain Send",
		Text:           "Hello World",
		URN:            "ext:asldkfjpoawije",
		Status:         "W",
		ResponseBody:   "Message Sent",
		ResponseStatus: 200,
		SendPrep:       setSendURL,
	},
	{
		Label:          "Quick Replies",
		Text:           "Hello World",
		QuickReplies:   []string{"One", "Two", "Three"},
		URN:            "ext:asldkfjpoawije",
		Status:         "W",
		ResponseBody:   "Message Sent",
		ResponseStatus: 200,
		SendPrep:       setSendURL,
	},
	{
		Label:          "Sending With Attachment",
		Text:           "Hello World",
		Attachments:    []string{"https://assets.website-files.com/5a18dcffca1ffe0001627dc8/5fa93a867a78a15d5e8bb576_fold-img.png"},
		URN:            "ext:asldkfjpoawije",
		Status:         "W",
		ResponseBody:   "Message Sent",
		ResponseStatus: 200,
		SendPrep:       setSendURL,
	},
	{
		Label:             "Receive Attachment",
		Text:              "Hello World",
		URN:               "ext:asldkfjpoawije",
		Status:            "W",
		ResponseBody:      "Message Sent",
		ResponseStatus:    200,
		SendPrep:          setSendURL,
		Metadata:          json.RawMessage(`{"receive_attachment": "image"`),
		ReceiveAttachment: "image",
	},
	{
		Label:          "Sharing Config",
		Text:           "Hello World",
		URN:            "ext:asldkfjpoawije",
		Status:         "W",
		ResponseBody:   "Message Sent",
		ResponseStatus: 200,
		SendPrep:       setSendURL,
		Metadata:       json.RawMessage(`{"sharing_config": {}`),
		SharingConfig:  json.RawMessage(`{"facebook": True}`),
	},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c567b", "WCH", "websocket-app.communityconnectlabs.com", "US", nil)
	defaultChannel.SetScheme(urns.ExternalScheme)

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
