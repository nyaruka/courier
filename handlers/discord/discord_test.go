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
	{Label: "Invalid ID", URL: "/c/ds/bac782c2-7aeb-4389-92f5-97887744f573/receive", Data: `from=somebody&text=hello`, Status: 400, Response: "Error"},
	{Label: "Garbage Body", URL: "/c/ds/bac782c2-7aeb-4389-92f5-97887744f573/receive", Data: `sdfaskdfajsdkfajsdfaksdf`, Status: 400, Response: "Error"},
	{Label: "Missing Text", URL: "/c/ds/bac782c2-7aeb-4389-92f5-97887744f573/receive", Data: `from=694634743521607802`, Status: 400, Response: "Error"},
}

var sendTestCases = []ChannelSendTestCase{
	{Label: "Simple Send", Text: "Hello World", URN: "discord:694634743521607802", Path: "/discord/rp/send", ResponseBody: "", ResponseStatus: 200, SendPrep: setSendURL},
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
