package whatsapp

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel(
		"8eb23e93-5ecb-45ba-b726-3b064e0c568c",
		"WA",
		"250788383383",
		"RW",
		map[string]interface{}{
			"username": "wa123",
			"password": "pword123",
			"base_url": "https://foo.bar/",
		}),
}

var helloMsg = `{
  "meta": null,
  "payload": {
    "from": "250788123123",
    "message_id": "41",
    "timestamp": "1454119029",
    "message": {
        "has_media": false,
        "text": "hello world",
        "type": "text"
    }
  },
  "error": false
}`

var invalidMsg = `not json`

var errorMsg = `{
    "meta": null,
    "payload": {
      "from": "250788123123",
      "message_id": "41",
      "timestamp": "1454119029",
      "message": {
          "has_media": false,
          "text": "hello world",
          "type": "text"
      }
    },
    "error": true
  }`

var validStatus = `
{
    "meta": null,
    "payload": {
      "message_id": "157b5e14568e8",
      "to": "16315555555",
      "timestamp": "1476225796",
      "message_status": "sent"
    },
    "error": false
  } 
`

var invalidStatus = `
{
    "meta": null,
    "payload": {
      "message_id": "157b5e14568e8",
      "to": "16315555555",
      "timestamp": "1476225796",
      "message_status": "in_orbit"
    },
    "error": false
  } 
`

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: helloMsg, Status: 200, Response: "Accepted",
		Text: Sp("hello world"), URN: Sp("whatsapp:250788123123"), ExternalID: Sp("41"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC))},
	{Label: "Receive Invalid JSON", URL: "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: invalidMsg, Status: 400, Response: "unable to parse"},
	{Label: "Receive Error Msg", URL: "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: errorMsg, Status: 400, Response: "received errored message"},

	{Label: "Receive Valid Status", URL: "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/status", Data: validStatus, Status: 200, Response: "Status Update Accepted"},
	{Label: "Receive Invalid JSON", URL: "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/status", Data: "not json", Status: 400, Response: "unable to parse"},
	{Label: "Receive Invalid Status", URL: "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/status", Data: invalidStatus, Status: 400, Response: "unknown status"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, NewHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, NewHandler(), testCases)
}

// setSendURL takes care of setting the base_url to our test server host
func setSendURL(server *httptest.Server, channel courier.Channel, msg courier.Msg) {
	channel.(*courier.MockChannel).SetConfig("base_url", server.URL)
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "whatsapp:250788123123",
		Status: "W", ExternalID: "157b5e14568e8",
		ResponseBody: `{ "payload": { "message_id": "157b5e14568e8" }, "error": false }`, ResponseStatus: 200,
		RequestBody: `{"payload":{"to":"250788123123","body":"Simple Message"}}`,
		SendPrep:    setSendURL},
	{Label: "Unicode Send",
		Text: "☺", URN: "whatsapp:250788123123",
		Status: "W", ExternalID: "157b5e14568e8",
		ResponseBody: `{ "payload": { "message_id": "157b5e14568e8" }, "error": false }`, ResponseStatus: 200,
		RequestBody: `{"payload":{"to":"250788123123","body":"☺"}}`,
		SendPrep:    setSendURL},
	{Label: "Error",
		Text: "Error", URN: "whatsapp:250788123123",
		Status:       "E",
		ResponseBody: `{ "error": true }`, ResponseStatus: 403,
		RequestBody: `{"payload":{"to":"250788123123","body":"Error"}}`,
		SendPrep:    setSendURL},
	{Label: "Error Field",
		Text: "Error", URN: "whatsapp:250788123123",
		Status:       "E",
		ResponseBody: `{ "payload": { "message_id": "157b5e14568e8" }, "error": true }`, ResponseStatus: 200,
		RequestBody: `{"payload":{"to":"250788123123","body":"Error"}}`,
		SendPrep:    setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WA", "250788383383", "US",
		map[string]interface{}{
			"username": "wa123",
			"password": "pword123",
			"base_url": "https://foo.bar/",
		})

	RunChannelSendTestCases(t, defaultChannel, NewHandler(), defaultSendTestCases)
}
