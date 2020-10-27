package discord

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
)

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

var testChannels = []courier.Channel{
	courier.NewMockChannel("bac782c2-7aeb-4389-92f5-97887744f573", "DS", "discord", "US", map[string]interface{}{}),
}

var testCases = []ChannelHandleTestCase{
	{Label: "Recieve Message", URL: "/c/ds/bac782c2-7aeb-4389-92f5-97887744f573/receive", Data: `from=694634743521607802&text=hello`, Status: 200, Text: Sp("hello"), URN: Sp("discord:694634743521607802")},
	{Label: "Recieve Message with attachment", URL: "/c/ds/bac782c2-7aeb-4389-92f5-97887744f573/receive", Data: `from=694634743521607802&text=hello&attachments=https://test.test/foo.png`, Status: 200, Text: Sp("hello"), URN: Sp("discord:694634743521607802"), Attachments: []string{"https://test.test/foo.png"}},
	{Label: "Invalid ID", URL: "/c/ds/bac782c2-7aeb-4389-92f5-97887744f573/receive", Data: `from=somebody&text=hello`, Status: 400, Response: "Error"},
	{Label: "Garbage Body", URL: "/c/ds/bac782c2-7aeb-4389-92f5-97887744f573/receive", Data: `sdfaskdfajsdkfajsdfaksdf`, Status: 400, Response: "Error"},
	{Label: "Missing Text", URL: "/c/ds/bac782c2-7aeb-4389-92f5-97887744f573/receive", Data: `from=694634743521607802`, Status: 400, Response: "Error"},
	{Label: "Message Sent Handler", URL: "/c/ds/bac782c2-7aeb-4389-92f5-97887744f573/sent/", Data: `id=12345`, Status: 200, Response: `"status":"S"`},
	{Label: "Message Sent Handler Garbage", URL: "/c/ds/bac782c2-7aeb-4389-92f5-97887744f573/sent/", Data: `nothing`, Status: 400},
}

var sendTestCases = []ChannelSendTestCase{
	{Label: "Simple Send", Text: "Hello World", URN: "discord:694634743521607802", Path: "/discord/rp/send", ResponseStatus: 200, RequestBody: `{"id":"10","text":"Hello World","to":"694634743521607802","channel":"bac782c2-7aeb-4389-92f5-97887744f573","attachments":[],"quick_replies":null}`, SendPrep: setSendURL},
	{Label: "Simple Send", Text: "Hello World", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"}, URN: "discord:694634743521607802", Path: "/discord/rp/send", RequestBody: `{"id":"10","text":"Hello World","to":"694634743521607802","channel":"bac782c2-7aeb-4389-92f5-97887744f573","attachments":["https://foo.bar/image.jpg"],"quick_replies":null}`, ResponseStatus: 200, SendPrep: setSendURL},
	{Label: "Simple Send with attachements and Quick Replies", Text: "Hello World", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"}, QuickReplies: []string{"hello", "world"}, URN: "discord:694634743521607802", Path: "/discord/rp/send", RequestBody: `{"id":"10","text":"Hello World","to":"694634743521607802","channel":"bac782c2-7aeb-4389-92f5-97887744f573","attachments":["https://foo.bar/image.jpg"],"quick_replies":["hello","world"]}`, ResponseStatus: 200, SendPrep: setSendURL},
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	// this is actually a path, which we'll combine with the test server URL
	sendURL := c.StringConfigForKey("send_path", "/discord/rp/send")
	sendURL, _ = utils.AddURLPath(s.URL, sendURL)
	c.(*courier.MockChannel).SetConfig(courier.ConfigSendURL, sendURL)
}
func TestSending(t *testing.T) {

	RunChannelSendTestCases(t, testChannels[0], newHandler(), sendTestCases, nil)
}
