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
			"base_url": "https://foo.bar/",
		}),
}

var helloMsg = `{
  "messages": [{
    "from": "250788123123",
    "id": "41",
    "timestamp": "1454119029",
    "text": {
      "body": "hello world"
    },
    "type": "text"
  }]
}`

var invalidFrom = `{
  "messages": [{
    "from": "notnumber",
    "id": "41",
    "timestamp": "1454119029",
    "text": {
      "body": "hello world"
    },
    "type": "text"
  }]
}`

var invalidTimestamp = `{
  "messages": [{
    "from": "notnumber",
    "id": "41",
    "timestamp": "asdf",
    "text": {
      "body": "hello world"
    },
    "type": "text"
  }]
}`

var invalidMsg = `not json`

var validStatus = `
{
  "statuses": [{
    "id": "9712A34B4A8B6AD50F",
    "recipient_id": "16315555555",
    "status": "sent",
    "timestamp": "1518694700"
  }]
}
`

var invalidStatus = `
{
  "statuses": [{
    "id": "9712A34B4A8B6AD50F",
    "recipient_id": "16315555555",
    "status": "in_orbit",
    "timestamp": "1518694700"
  }]
}
`

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: helloMsg, Status: 200, Response: `"type":"msg"`,
		Text: Sp("hello world"), URN: Sp("whatsapp:250788123123"), ExternalID: Sp("41"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC))},
	{Label: "Receive Invalid JSON", URL: "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: invalidMsg, Status: 400, Response: "unable to parse"},
	{Label: "Receive Invalid From", URL: "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: invalidFrom, Status: 400, Response: "invalid whatsapp id"},
	{Label: "Receive Invalid Timestamp", URL: "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: invalidTimestamp, Status: 400, Response: "invalid timestamp"},

	{Label: "Receive Valid Status", URL: "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: validStatus, Status: 200, Response: `"type":"status"`,
		MsgStatus: Sp("S"), ExternalID: Sp("9712A34B4A8B6AD50F")},
	{Label: "Receive Invalid JSON", URL: "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: "not json", Status: 400, Response: "unable to parse"},
	{Label: "Receive Invalid Status", URL: "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: invalidStatus, Status: 400, Response: `"invalid status: in_orbit"`},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

// setSendURL takes care of setting the base_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	c.(*courier.MockChannel).SetConfig("base_url", s.URL)
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "whatsapp:250788123123",
		Status: "W", ExternalID: "157b5e14568e8",
		ResponseBody: `{ "messages": [{"id": "157b5e14568e8"}] }`, ResponseStatus: 201,
		RequestBody: `{"to":"250788123123","type":"text","text":{"body":"Simple Message"}}`,
		SendPrep:    setSendURL},
	{Label: "Unicode Send",
		Text: "☺", URN: "whatsapp:250788123123",
		Status: "W", ExternalID: "157b5e14568e8",
		ResponseBody: `{ "messages": [{"id": "157b5e14568e8"}] }`, ResponseStatus: 201,
		RequestBody: `{"to":"250788123123","type":"text","text":{"body":"☺"}}`,
		SendPrep:    setSendURL},
	{Label: "Error",
		Text: "Error", URN: "whatsapp:250788123123",
		Status:       "E",
		ResponseBody: `{ "errors": [{"title":"Error Sendind"}] }`, ResponseStatus: 403,
		RequestBody: `{"to":"250788123123","type":"text","text":{"body":"Error"}}`,
		SendPrep:    setSendURL},
	{Label: "No Message ID",
		Text: "Error", URN: "whatsapp:250788123123",
		Status:       "E",
		ResponseBody: `{ "messages": [] }`, ResponseStatus: 200,
		RequestBody: `{"to":"250788123123","type":"text","text":{"body":"Error"}}`,
		SendPrep:    setSendURL},
	{Label: "Error Field",
		Text: "Error", URN: "whatsapp:250788123123",
		Status:       "E",
		ResponseBody: `{ "errors": [{"title":"Error Sendind"}] }`, ResponseStatus: 200,
		RequestBody: `{"to":"250788123123","type":"text","text":{"body":"Error"}}`,
		SendPrep:    setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WA", "250788383383", "US",
		map[string]interface{}{
			"auth_token": "token123",
			"base_url":   "https://foo.bar/",
		})

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
