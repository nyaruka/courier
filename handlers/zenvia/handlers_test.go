package zenvia

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

var testWhatsappChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ZVW", "2020", "BR", map[string]any{"api_key": "zv-api-token"}),
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ZVS", "2020", "BR", map[string]any{"api_key": "zv-api-token"}),
}

var testSMSChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ZVS", "2020", "BR", map[string]any{"api_key": "zv-api-token"}),
}

var (
	receiveWhatsappURL = "/c/zvw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusWhatsppURL   = "/c/zvw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"

	receiveSMSURL = "/c/zvs/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusSMSURL  = "/c/zvs/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"

	notJSON = "empty"
)

var wrongJSONSchema = `{}`

var validStatus = `{
	"id": "string",
	"type": "MESSAGE_STATUS",
	"channel": "string",
	"messageId": "hs765939216",
	"messageStatus": {
	  "timestamp": "2021-03-12T12:15:31Z",
	  "code": "SENT"
	}
}`

var unknownStatus = `{
	"id": "string",
	"type": "MESSAGE_STATUS",
	"channel": "string",
	"messageId": "hs765939216",
	"messageStatus": {
	  "timestamp": "2021-03-12T12:15:31Z",
	  "code": "FOO"
	}
}`

var validReceive = `{
	"id": "string",
	"timestamp": "2017-05-03T03:04:45Z",
	"type": "MESSAGE",
	"message": {
	  "id": "string",
	  "from": "254791541111",
	  "to": "2020",
	  "direction": "IN",
	  "contents": [
		{
		  "type": "text",
		  "text": "Msg",
		  "payload": "string"
		}
	  ],
	  "visitor": {
		"name": "Bob"
	  }
	}
}`

var fileReceive = `{
	"id": "string",
	"timestamp": "2017-05-03T03:04:45Z",
	"type": "MESSAGE",
	"message": {
	  "id": "string",
	  "from": "254791541111",
	  "to": "2020",
	  "direction": "IN",
	  "contents": [
		{
		  "type": "file",
		  "fileUrl": "https://foo.bar/v1/media/41"
		}
	  ],
	  "visitor": {
		"name": "Bob"
	  }
	}
}`

var locationReceive = `{
	"id": "string",
	"timestamp": "2017-05-03T03:04:45Z",
	"type": "MESSAGE",
	"message": {
	  "id": "string",
	  "from": "254791541111",
	  "to": "2020",
	  "direction": "IN",
	  "contents": [
		{
		  "type": "location",
		  "longitude": 1.00,
		  "latitude": 0.00
		}
	  ],
	  "visitor": {
		"name": "Bob"
	  }
	}
}`

var invalidDateReceive = `{
	"id": "string",
	"timestamp": "2014-08-26T12:55:48.593-03:00",
	"type": "MESSAGE",
	"message": {
	  "id": "string",
	  "from": "254791541111",
	  "to": "2020",
	  "direction": "IN",
	  "contents": [
		{
		  "type": "text",
		  "text": "Msg",
		  "payload": "string"
		}
	  ],
	  "visitor": {
		"name": "Bob"
	  }
	}
  }`

var missingFieldsReceive = `{
	"id": "string",
	"timestamp": "2017-05-03T03:04:45Z",
	"type": "MESSAGE",
	"message": {
	  "id": "",
	  "from": "254791541111",
	  "to": "2020",
	  "direction": "IN",
	  "contents": [
		{
		  "type": "text",
		  "text": "Msg",
		  "payload": "string"
		}
	  ],
	  "visitor": {
		"name": "Bob"
	  }
	}
}`

var testWhatappCases = []IncomingTestCase{
	{Label: "Receive Valid", URL: receiveWhatsappURL, Data: validReceive, ExpectedRespStatus: 200, ExpectedBodyContains: "Message Accepted",
		ExpectedMsgText: Sp("Msg"), ExpectedURN: "whatsapp:254791541111", ExpectedDate: time.Date(2017, 5, 3, 03, 04, 45, 0, time.UTC)},

	{Label: "Receive file Valid", URL: receiveWhatsappURL, Data: fileReceive, ExpectedRespStatus: 200, ExpectedBodyContains: "Message Accepted",
		ExpectedMsgText: Sp(""), ExpectedAttachments: []string{"https://foo.bar/v1/media/41"}, ExpectedURN: "whatsapp:254791541111", ExpectedDate: time.Date(2017, 5, 3, 03, 04, 45, 0, time.UTC)},

	{Label: "Receive location Valid", URL: receiveWhatsappURL, Data: locationReceive, ExpectedRespStatus: 200, ExpectedBodyContains: "Message Accepted",
		ExpectedMsgText: Sp(""), ExpectedAttachments: []string{"geo:0.000000,1.000000"}, ExpectedURN: "whatsapp:254791541111", ExpectedDate: time.Date(2017, 5, 3, 03, 04, 45, 0, time.UTC)},

	{Label: "Not JSON body", URL: receiveWhatsappURL, Data: notJSON, ExpectedRespStatus: 400, ExpectedBodyContains: "unable to parse request JSON"},
	{Label: "Wrong JSON schema", URL: receiveWhatsappURL, Data: wrongJSONSchema, ExpectedRespStatus: 400, ExpectedBodyContains: "request JSON doesn't match required schema"},
	{Label: "Missing field", URL: receiveWhatsappURL, Data: missingFieldsReceive, ExpectedRespStatus: 400, ExpectedBodyContains: "validation for 'ID' failed on the 'required'"},
	{Label: "Bad Date", URL: receiveWhatsappURL, Data: invalidDateReceive, ExpectedRespStatus: 400, ExpectedBodyContains: "invalid date format"},

	{
		Label:                "Valid Status",
		URL:                  statusWhatsppURL,
		Data:                 validStatus,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `Accepted`,
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "hs765939216", Status: courier.MsgStatusSent}},
	},
	{
		Label:                "Unkown Status",
		URL:                  statusWhatsppURL,
		Data:                 unknownStatus,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "hs765939216", Status: courier.MsgStatusErrored}},
	},
	{Label: "Not JSON body", URL: statusWhatsppURL, Data: notJSON, ExpectedRespStatus: 400, ExpectedBodyContains: "unable to parse request JSON"},
	{Label: "Wrong JSON schema", URL: statusWhatsppURL, Data: wrongJSONSchema, ExpectedRespStatus: 400, ExpectedBodyContains: "request JSON doesn't match required schema"},
}

var testSMSCases = []IncomingTestCase{
	{Label: "Receive Valid", URL: receiveSMSURL, Data: validReceive, ExpectedRespStatus: 200, ExpectedBodyContains: "Message Accepted",
		ExpectedMsgText: Sp("Msg"), ExpectedURN: "whatsapp:254791541111", ExpectedDate: time.Date(2017, 5, 3, 03, 04, 45, 0, time.UTC)},

	{Label: "Receive file Valid", URL: receiveSMSURL, Data: fileReceive, ExpectedRespStatus: 200, ExpectedBodyContains: "Message Accepted",
		ExpectedMsgText: Sp(""), ExpectedAttachments: []string{"https://foo.bar/v1/media/41"}, ExpectedURN: "whatsapp:254791541111", ExpectedDate: time.Date(2017, 5, 3, 03, 04, 45, 0, time.UTC)},

	{Label: "Receive location Valid", URL: receiveSMSURL, Data: locationReceive, ExpectedRespStatus: 200, ExpectedBodyContains: "Message Accepted",
		ExpectedMsgText: Sp(""), ExpectedAttachments: []string{"geo:0.000000,1.000000"}, ExpectedURN: "whatsapp:254791541111", ExpectedDate: time.Date(2017, 5, 3, 03, 04, 45, 0, time.UTC)},

	{Label: "Not JSON body", URL: receiveSMSURL, Data: notJSON, ExpectedRespStatus: 400, ExpectedBodyContains: "unable to parse request JSON"},
	{Label: "Wrong JSON schema", URL: receiveSMSURL, Data: wrongJSONSchema, ExpectedRespStatus: 400, ExpectedBodyContains: "request JSON doesn't match required schema"},
	{Label: "Missing field", URL: receiveSMSURL, Data: missingFieldsReceive, ExpectedRespStatus: 400, ExpectedBodyContains: "validation for 'ID' failed on the 'required'"},
	{Label: "Bad Date", URL: receiveSMSURL, Data: invalidDateReceive, ExpectedRespStatus: 400, ExpectedBodyContains: "invalid date format"},

	{
		Label:                "Valid Status",
		URL:                  statusSMSURL,
		Data:                 validStatus,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `Accepted`,
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "hs765939216", Status: courier.MsgStatusSent}},
	},
	{
		Label:                "Unknown Status",
		URL:                  statusSMSURL,
		Data:                 unknownStatus,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "hs765939216", Status: courier.MsgStatusErrored}},
	},
	{Label: "Not JSON body", URL: statusSMSURL, Data: notJSON, ExpectedRespStatus: 400, ExpectedBodyContains: "unable to parse request JSON"},
	{Label: "Wrong JSON schema", URL: statusSMSURL, Data: wrongJSONSchema, ExpectedRespStatus: 400, ExpectedBodyContains: "request JSON doesn't match required schema"},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testWhatsappChannels, newHandler("ZVW", "Zenvia WhatsApp"), testWhatappCases)
	RunIncomingTestCases(t, testSMSChannels, newHandler("ZVS", "Zenvia SMS"), testSMSCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testWhatsappChannels, newHandler("ZVW", "Zenvia WhatsApp"), testWhatappCases)
	RunChannelBenchmarks(b, testSMSChannels, newHandler("ZVS", "Zenvia SMS"), testSMSCases)
}

// setSendURL takes care of setting the sendURL to call
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	whatsappSendURL = s.URL
	smsSendURL = s.URL
}

var defaultWhatsappSendTestCases = []OutgoingTestCase{
	{
		Label:              "Plain Send",
		MsgText:            "Simple Message ☺",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `{"id": "55555"}`,
		MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
			"X-API-TOKEN":  "zv-api-token",
		},
		ExpectedRequestBody: `{"from":"2020","to":"250788383383","contents":[{"type":"text","text":"Simple Message ☺"}]}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "55555",
		SendPrep:            setSendURL,
	},
	{
		Label:              "Long Send",
		MsgText:            "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `{"id": "55555"}`,
		MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
			"X-API-TOKEN":  "zv-api-token",
		},
		ExpectedRequestBody: `{"from":"2020","to":"250788383383","contents":[{"type":"text","text":"This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say,"},{"type":"text","text":"I need to keep adding more things to make it work"}]}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "55555",
		SendPrep:            setSendURL,
	},
	{
		Label:              "Send Attachment",
		MsgText:            "My pic!",
		MsgURN:             "tel:+250788383383",
		MsgAttachments:     []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody:   `{"id": "55555"}`,
		MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
			"X-API-TOKEN":  "zv-api-token",
		},
		ExpectedRequestBody: `{"from":"2020","to":"250788383383","contents":[{"type":"file","fileUrl":"https://foo.bar/image.jpg","fileMimeType":"image/jpeg"},{"type":"text","text":"My pic!"}]}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "55555",
		SendPrep:            setSendURL,
	},
	{
		Label:              "No External ID",
		MsgText:            "No External ID",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `{"code": "400","message": "Validation error","details": [{"code": "400","path": "Error","message": "Error description"}]}`,
		MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
			"X-API-TOKEN":  "zv-api-token",
		},
		ExpectedRequestBody: `{"from":"2020","to":"250788383383","contents":[{"type":"text","text":"No External ID"}]}`,
		ExpectedMsgStatus:   "E",
		ExpectedErrors:      []*courier.ChannelError{courier.ErrorResponseValueMissing("id")},
		SendPrep:            setSendURL,
	},
	{
		Label:               "Error Sending",
		MsgText:             "Error Message",
		MsgURN:              "tel:+250788383383",
		MockResponseBody:    `{ "error": "failed" }`,
		MockResponseStatus:  401,
		ExpectedRequestBody: `{"from":"2020","to":"250788383383","contents":[{"type":"text","text":"Error Message"}]}`,
		ExpectedMsgStatus:   "E",
		SendPrep:            setSendURL,
	},
}

var defaultSMSSendTestCases = []OutgoingTestCase{
	{
		Label:              "Plain Send",
		MsgText:            "Simple Message ☺",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `{"id": "55555"}`,
		MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
			"X-API-TOKEN":  "zv-api-token",
		},
		ExpectedRequestBody: `{"from":"2020","to":"250788383383","contents":[{"type":"text","text":"Simple Message ☺"}]}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "55555",
		SendPrep:            setSendURL},
	{
		Label:              "Long Send",
		MsgText:            "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `{"id": "55555"}`,
		MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
			"X-API-TOKEN":  "zv-api-token",
		},
		ExpectedRequestBody: `{"from":"2020","to":"250788383383","contents":[{"type":"text","text":"This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say,"},{"type":"text","text":"I need to keep adding more things to make it work"}]}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "55555",
		SendPrep:            setSendURL},
	{
		Label:              "Send Attachment",
		MsgText:            "My pic!",
		MsgURN:             "tel:+250788383383",
		MsgAttachments:     []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody:   `{"id": "55555"}`,
		MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
			"X-API-TOKEN":  "zv-api-token",
		},
		ExpectedRequestBody: `{"from":"2020","to":"250788383383","contents":[{"type":"text","text":"My pic!\nhttps://foo.bar/image.jpg"}]}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "55555",
		SendPrep:            setSendURL,
	},
	{
		Label:              "No External ID",
		MsgText:            "No External ID",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `{"code": "400","message": "Validation error","details": [{"code": "400","path": "Error","message": "Error description"}]}`,
		MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
			"X-API-TOKEN":  "zv-api-token",
		},
		ExpectedRequestBody: `{"from":"2020","to":"250788383383","contents":[{"type":"text","text":"No External ID"}]}`,
		ExpectedMsgStatus:   "E",
		ExpectedErrors:      []*courier.ChannelError{courier.ErrorResponseValueMissing("id")},
		SendPrep:            setSendURL,
	},
	{
		Label:               "Error Sending",
		MsgText:             "Error Message",
		MsgURN:              "tel:+250788383383",
		MockResponseBody:    `{ "error": "failed" }`,
		MockResponseStatus:  401,
		ExpectedRequestBody: `{"from":"2020","to":"250788383383","contents":[{"type":"text","text":"Error Message"}]}`,
		ExpectedMsgStatus:   "E",
		SendPrep:            setSendURL,
	},
}

func TestOutgoing(t *testing.T) {
	maxMsgLength = 160
	var defaultWhatsappChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ZVW", "2020", "BR", map[string]any{"api_key": "zv-api-token"})
	RunOutgoingTestCases(t, defaultWhatsappChannel, newHandler("ZVW", "Zenvia WhatsApp"), defaultWhatsappSendTestCases, []string{"zv-api-token"}, nil)

	var defaultSMSChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ZVS", "2020", "BR", map[string]any{"api_key": "zv-api-token"})
	RunOutgoingTestCases(t, defaultSMSChannel, newHandler("ZVS", "Zenvia SMS"), defaultSMSSendTestCases, []string{"zv-api-token"}, nil)
}
