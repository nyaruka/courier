package viber

import (
	"bytes"
	"crypto/hmac"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

// setSend takes care of setting the sendURL to call
func setSendURL(server *httptest.Server, h courier.ChannelHandler, channel courier.Channel, msg courier.MsgOut) {
	sendURL = server.URL
}

func buildMockAttachmentService(testCases []OutgoingTestCase) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := w.Header()
		if r.Method == http.MethodHead {
			headers["Content-Length"] = []string{"123456"}
		}
		w.Write([]byte(""))
	}))

	// update our tests media urls
	for c := range testCases {
		if len(testCases[c].MsgAttachments) > 0 {
			for i, a := range testCases[c].MsgAttachments {
				mediaType, mediaURL := SplitAttachment(a)
				parts := strings.Split(mediaURL, "/")
				testCases[c].MsgAttachments[i] = fmt.Sprintf("%s:%s/%s", mediaType, server.URL, parts[len(parts)-1])
			}
		}
		testCases[c].ExpectedRequestBody = strings.Replace(testCases[c].ExpectedRequestBody, "{{ SERVER_URL }}", server.URL, -1)
	}

	return server
}

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message",
		MsgURN:              "viber:xy5/5y6O81+/kbWHpLhBoA==",
		MockResponseStatus:  200,
		MockResponseBody:    `{"status":0,"status_message":"ok","message_token":4987381194038857789}`,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Accept": "application/json"},
		ExpectedRequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"Simple Message","type":"text","tracking_data":"10"}`,
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Long Send",
		MsgText:             "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:              "viber:xy5/5y6O81+/kbWHpLhBoA==",
		MockResponseStatus:  200,
		MockResponseBody:    `{"status":0,"status_message":"ok","message_token":4987381194038857789}`,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Accept": "application/json"},
		ExpectedRequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"I need to keep adding more things to make it work","type":"text","tracking_data":"10"}`,
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL},
	{
		Label:               "Unicode Send",
		MsgText:             "☺",
		MsgURN:              "viber:xy5/5y6O81+/kbWHpLhBoA==",
		MockResponseStatus:  200,
		MockResponseBody:    `{"status":0,"status_message":"ok","message_token":4987381194038857789}`,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Accept": "application/json"},
		ExpectedRequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"☺","type":"text","tracking_data":"10"}`,
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Quick Reply",
		MsgText:             "Are you happy?",
		MsgURN:              "viber:xy5/5y6O81+/kbWHpLhBoA==",
		MsgQuickReplies:     []string{"Yes", "No"},
		MockResponseStatus:  200,
		MockResponseBody:    `{"status":0,"status_message":"ok","message_token":4987381194038857789}`,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Accept": "application/json"},
		ExpectedRequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"Are you happy?","type":"text","tracking_data":"10","keyboard":{"Type":"keyboard","DefaultHeight":false,"Buttons":[{"ActionType":"reply","ActionBody":"Yes","Text":"Yes","TextSize":"regular","Columns":"3"},{"ActionType":"reply","ActionBody":"No","Text":"No","TextSize":"regular","Columns":"3"}]}}`,
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Send Attachment",
		MsgText:             "My pic!",
		MsgURN:              "viber:xy5/5y6O81+/kbWHpLhBoA==",
		MsgAttachments:      []string{"image/jpeg:https://localhost/image.jpg"},
		MockResponseStatus:  200,
		MockResponseBody:    `{"status":0,"status_message":"ok","message_token":4987381194038857789}`,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Accept": "application/json"},
		ExpectedRequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"My pic!","type":"picture","tracking_data":"10","media":"{{ SERVER_URL }}/image.jpg"}`,
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Long Description with Attachment",
		MsgText:             "Text description is longer that 10 characters",
		MsgURN:              "viber:xy5/5y6O81+/kbWHpLhBoA==",
		MsgAttachments:      []string{"image/jpeg:https://localhost/image.jpg"},
		MockResponseStatus:  200,
		MockResponseBody:    `{"status":0,"status_message":"ok","message_token":4987381194038857789}`,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Accept": "application/json"},
		ExpectedRequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"Text description is longer that 10 characters","type":"text","tracking_data":"10"}`,
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Send Attachment Video",
		MsgText:             "My video!",
		MsgURN:              "viber:xy5/5y6O81+/kbWHpLhBoA==",
		MsgAttachments:      []string{"video/mp4:https://localhost/video.mp4"},
		MockResponseStatus:  200,
		MockResponseBody:    `{"status":0,"status_message":"ok","message_token":4987381194038857789}`,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Accept": "application/json"},
		ExpectedRequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"My video!","type":"text","tracking_data":"10"}`,
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Send Attachment Audio",
		MsgText:             "My audio!",
		MsgURN:              "viber:xy5/5y6O81+/kbWHpLhBoA==",
		MsgAttachments:      []string{"audio/mp3:https://localhost/audio.mp3"},
		MockResponseStatus:  200,
		MockResponseBody:    `{"status":0,"status_message":"ok","message_token":4987381194038857789}`,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Accept": "application/json"},
		ExpectedRequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"My audio!","type":"text","tracking_data":"10"}`,
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Got non-0 response",
		MsgText:             "Simple Message",
		MsgURN:              "viber:xy5/5y6O81+/kbWHpLhBoA==",
		MockResponseStatus:  200,
		MockResponseBody:    `{"status":3,"status_message":"badData"}`,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Accept": "application/json"},
		ExpectedRequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"Simple Message","type":"text","tracking_data":"10"}`,
		ExpectedMsgStatus:   "E",
		ExpectedErrors:      []*courier.ChannelError{courier.ErrorExternal("3", "There is an error in the request itself (missing comma, brackets, etc.)")},
		SendPrep:            setSendURL,
	},
	{
		Label:               "Got general error response",
		MsgText:             "Simple Message",
		MsgURN:              "viber:xy5/5y6O81+/kbWHpLhBoA==",
		MockResponseStatus:  200,
		MockResponseBody:    `{"status":99,"status_message":"General error"}`,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Accept": "application/json"},
		ExpectedRequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"Simple Message","type":"text","tracking_data":"10"}`,
		ExpectedMsgStatus:   "E",
		ExpectedErrors:      []*courier.ChannelError{courier.ErrorExternal("99", "General error")},
		SendPrep:            setSendURL,
	},
	{
		Label:               "Got Invalid JSON response",
		MsgText:             "Simple Message",
		MsgURN:              "viber:xy5/5y6O81+/kbWHpLhBoA==",
		MockResponseStatus:  200,
		MockResponseBody:    `invalidJSON`,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Accept": "application/json"},
		ExpectedRequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"Simple Message","type":"text","tracking_data":"10"}`,
		ExpectedMsgStatus:   "E",
		ExpectedErrors:      []*courier.ChannelError{courier.ErrorResponseUnparseable("JSON")},
		SendPrep:            setSendURL,
	},
	{
		Label:               "Error Sending",
		MsgText:             "Error Message",
		MsgURN:              "viber:xy5/5y6O81+/kbWHpLhBoA==",
		MockResponseStatus:  401,
		MockResponseBody:    `{"status":"5"}`,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Accept": "application/json"},
		ExpectedRequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"Error Message","type":"text","tracking_data":"10"}`,
		ExpectedMsgStatus:   "E",
		ExpectedErrors:      []*courier.ChannelError{courier.ErrorResponseStatusCode()},
		SendPrep:            setSendURL,
	},
}

var invalidTokenSendTestCases = []OutgoingTestCase{
	{
		Label:          "Invalid token",
		ExpectedErrors: []*courier.ChannelError{courier.NewChannelError("", "", "missing auth token in config")},
	},
}

var buttonLayoutSendTestCases = []OutgoingTestCase{
	{
		Label:               "Quick Reply With Layout With Column, Row and BgColor definitions",
		MsgText:             "Select a, b, c or d.",
		MsgURN:              "viber:xy5/5y6O81+/kbWHpLhBoA==",
		MsgQuickReplies:     []string{"a", "b", "c", "d"},
		MockResponseStatus:  200,
		MockResponseBody:    `{"status":0,"status_message":"ok","message_token":4987381194038857789}`,
		ExpectedRequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"Select a, b, c or d.","type":"text","tracking_data":"10","keyboard":{"Type":"keyboard","DefaultHeight":false,"Buttons":[{"ActionType":"reply","ActionBody":"a","Text":"\u003cfont color=\"#ffffff\"\u003ea\u003c/font\u003e\u003cbr\u003e\u003cbr\u003e","TextSize":"large","Columns":"2","BgColor":"#f7bb3f"},{"ActionType":"reply","ActionBody":"b","Text":"\u003cfont color=\"#ffffff\"\u003eb\u003c/font\u003e\u003cbr\u003e\u003cbr\u003e","TextSize":"large","Columns":"2","BgColor":"#f7bb3f"},{"ActionType":"reply","ActionBody":"c","Text":"\u003cfont color=\"#ffffff\"\u003ec\u003c/font\u003e\u003cbr\u003e\u003cbr\u003e","TextSize":"large","Columns":"2","BgColor":"#f7bb3f"},{"ActionType":"reply","ActionBody":"d","Text":"\u003cfont color=\"#ffffff\"\u003ed\u003c/font\u003e\u003cbr\u003e\u003cbr\u003e","TextSize":"large","Columns":"6","BgColor":"#f7bb3f"}]}}`,
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
}

func TestOutgoing(t *testing.T) {
	attachmentService := buildMockAttachmentService(defaultSendTestCases)
	defer attachmentService.Close()

	maxMsgLength = 160
	descriptionMaxLength = 10
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "VP", "2020", "",
		map[string]any{
			courier.ConfigAuthToken: "Token",
		})
	var invalidTokenChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "VP", "2020", "",
		map[string]any{},
	)
	var buttonLayoutChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "VP", "2021", "",
		map[string]any{
			courier.ConfigAuthToken: "Token",
			"button_layout":         map[string]any{"bg_color": "#f7bb3f", "text": "<font color=\"#ffffff\">*</font><br><br>", "text_size": "large"},
		})
	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"Token"}, nil)
	RunOutgoingTestCases(t, invalidTokenChannel, newHandler(), invalidTokenSendTestCases, []string{"Token"}, nil)
	RunOutgoingTestCases(t, buttonLayoutChannel, newHandler(), buttonLayoutSendTestCases, []string{"Token"}, nil)
}

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "VP", "2020", "", map[string]any{
		courier.ConfigAuthToken: "Token",
	}),
}

var testChannelsWithWelcomeMessage = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "VP", "2020", "", map[string]any{
		courier.ConfigAuthToken:   "Token",
		configViberWelcomeMessage: "Welcome to VP, Please subscribe here for more.",
	}),
}

var (
	receiveURL = "/c/vp/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"

	invalidJSON = "invalid"

	validMsg = `{
		"event": "message",
		"timestamp": 1481142112807,
		"message_token": 4987381189870374000,
		"sender": {
			"id": "xy5/5y6O81+/kbWHpLhBoA==",
			"name": "ET3"
		},
		"message": {
			"text": "incoming msg",
			"type": "text",
			"tracking_data": "3055"
		}
	}`

	invalidURNMsg = `{
		"event": "message",
		"timestamp": 1481142112807,
		"message_token": 4987381189870374000,
		"sender": {
			"id": "xy5/5$$**y6O81+/kbWHpLhBoA==",
			"name": "ET3"
		},
		"message": {
			"text": "incoming msg",
			"type": "text",
			"tracking_data": "3055"
		}
	}`

	webhookCheck = `{
		"event": "webhook",
		"timestamp": 4987034606158369000,
		"message_token": 1481059480858
	}`

	unexpectedEvent = `{
		"event": "unexpected",
		"timestamp": 4987034606158369000,
		"message_token": 1481059480858
	}`

	validSubscribed = `{
		"event": "subscribed",
		"timestamp": 1457764197627,
		"user": {
			"id": "01234567890A=",
			"name": "yarden",
			"avatar": "http://avatar_url",
			"country": "IL",
			"language": "en",
			"api_version": 1
		},
		"message_token": 4912661846655238145
	}`

	invalidURNSubscribed = `{
		"event": "subscribed",
		"timestamp": 1457764197627,
		"user": {
			"id": "012345678**$$90A=",
			"name": "yarden",
			"avatar": "http://avatar_url",
			"country": "IL",
			"language": "en",
			"api_version": 1
		},
		"message_token": 4912661846655238145
	}`

	validUnsubscribed = `{
		"event": "unsubscribed",
		"timestamp": 1457764197627,
		"user_id": "01234567890A=",
		"message_token": 4912661846655238145
	}`

	invalidURNUnsubscribed = `{
		"event": "unsubscribed",
		"timestamp": 1457764197627,
		"user_id": "012345678$$%**90A=",
		"message_token": 4912661846655238145
	}`

	validConversationStarted = `{
		"event": "conversation_started",
		"timestamp": 1457764197627,
		"message_token": 4912661846655238145,
		"type": "open",
		"context": "context information",
		"user": {
			"id": "xy5/5y6O81+/kbWHpLhBoA==",
			"name": "yarden",
			"avatar": "http://avatar_url",
			"country": "IL",
			"language": "en",
			"api_version": 1
		}
	}`

	rejectedMessage = `{
		"event": "message",
		"timestamp": 1481142112807,
		"message_token": 4987381189870374000,
		"sender": {
			"id": "xy5/5y6O81+/kbWHpLhBoA==",
			"name": "ET3"
		},
		"message": {
			"type": "text",
			"tracking_data": "3055"
		}
	}`

	rejectedPicture = `{
		"event": "message",
		"timestamp": 1481142112807,
		"message_token": 4987381189870374000,
		"sender": {
			"id": "xy5/5y6O81+/kbWHpLhBoA==",
			"name": "ET3"
		},
		"message": {
			"type": "picture",
			"tracking_data": "3055"
		}
	}`

	rejectedVideo = `{
		"event": "message",
		"timestamp": 1481142112807,
		"message_token": 4987381189870374000,
		"sender": {
			"id": "xy5/5y6O81+/kbWHpLhBoA==",
			"name": "ET3"
		},
		"message": {
			"type": "video",
			"tracking_data": "3055"
		}
	}`

	validReceiveContact = `{
		"event": "message",
		"timestamp": 1481142112807,
		"message_token": 4987381189870374000,
		"sender": {
			"id": "xy5/5y6O81+/kbWHpLhBoA==",
			"name": "ET3"
		},
		"message": {
			"text": "incoming msg",
			"type": "contact",
			"contact": {
				"name": "Alex",
				"phone_number": "+12067799191"
			},
			"tracking_data": "3055"
		}
	}`

	validReceiveURL = `{
		"event": "message",
		"timestamp": 1481142112807,
		"message_token": 4987381189870374000,
		"sender": {
			"id": "xy5/5y6O81+/kbWHpLhBoA==",
			"name": "ET3"
		},
		"message": {
			"text": "incoming msg",
			"type": "url",
			"media": "http://foo.com/",
			"tracking_data": "3055"
		}
	}`

	validReceiveLocation = `{
		"event": "message",
		"timestamp": 1481142112807,
		"message_token": 4987381189870374000,
		"sender": {
			"id": "xy5/5y6O81+/kbWHpLhBoA==",
			"name": "ET3"
		},
		"message": {
			"text": "incoming msg",
			"type": "location",
			"location": {
				"lat": 1.2,
				"lon": -1.3
			},
			"tracking_data": "3055"
		}
	}`

	validSticker = `{
		"event": "message",
		"timestamp": 1481142112807,
		"message_token": 4987381189870374000,
		"sender": {
			"id": "xy5/5y6O81+/kbWHpLhBoA==",
			"name": "ET3"
		},
		"message": {
			"text": "incoming msg",
			"type": "sticker",
			"sticker_id": "40133",
			"tracking_data": "3055"
		}
	}`

	receiveInvalidMessageType = `{
		"event": "message",
		"timestamp": 1481142112807,
		"message_token": 4987381189870374000,
		"sender": {
			"id": "xy5/5y6O81+/kbWHpLhBoA==",
			"name": "ET3"
		},
		"message": {
			"text": "incoming msg",
			"type": "unknown",
			"tracking_data": "3055"
		}
	}`

	failedStatusReport = `{
		"event": "failed",
		"timestamp": 1457764197627,
		"message_token": 4912661846655238145,
		"user_id": "01234567890A=",
		"desc": "failure description"
	}`

	deliveredStatusReport = `{
		"event": "delivered",
		"timestamp": 1457764197627,
		"message_token": 4912661846655238145,
		"user_id": "01234567890A=",
		"desc": "failure description"
	}`
)

var testCases = []IncomingTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: validMsg, ExpectedRespStatus: 200, ExpectedBodyContains: "Accepted",
		ExpectedMsgText: Sp("incoming msg"), ExpectedURN: "viber:xy5/5y6O81+/kbWHpLhBoA==", ExpectedExternalID: "4987381189870374000",
		PrepRequest: addValidSignature},
	{Label: "Receive invalid signature", URL: receiveURL, Data: validMsg, ExpectedRespStatus: 400, ExpectedBodyContains: "invalid request signature",
		PrepRequest: addInvalidSignature},
	{Label: "Receive invalid JSON", URL: receiveURL, Data: invalidJSON, ExpectedRespStatus: 400, ExpectedBodyContains: "unable to parse request JSON",
		PrepRequest: addValidSignature},
	{Label: "Receive invalid URN", URL: receiveURL, Data: invalidURNMsg, ExpectedRespStatus: 400, ExpectedBodyContains: "invalid viber id",
		PrepRequest: addValidSignature},
	{Label: "Receive invalid Message Type", URL: receiveURL, Data: receiveInvalidMessageType, ExpectedRespStatus: 400, ExpectedBodyContains: "unknown message type",
		PrepRequest: addValidSignature},
	{Label: "Webhook validation", URL: receiveURL, Data: webhookCheck, ExpectedRespStatus: 200, ExpectedBodyContains: "webhook valid", PrepRequest: addValidSignature},
	{
		Label:                "Failed Status Report",
		URL:                  receiveURL,
		Data:                 failedStatusReport,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"F"`,
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "4912661846655238145", Status: courier.MsgStatusFailed}},
		PrepRequest:          addValidSignature,
	},
	{Label: "Delivered Status Report", URL: receiveURL, Data: deliveredStatusReport, ExpectedRespStatus: 200, ExpectedBodyContains: `Ignored`, PrepRequest: addValidSignature},
	{
		Label:                "Subcribe",
		URL:                  receiveURL,
		Data:                 validSubscribed,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedEvents: []ExpectedEvent{
			{Type: courier.EventTypeNewConversation, URN: "viber:01234567890A="},
		},
		PrepRequest: addValidSignature,
	},
	{
		Label:                "Subcribe Invalid URN",
		URL:                  receiveURL,
		Data:                 invalidURNSubscribed,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "invalid viber id",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Unsubcribe",
		URL:                  receiveURL,
		Data:                 validUnsubscribed,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedEvents: []ExpectedEvent{
			{Type: courier.EventTypeStopContact, URN: "viber:01234567890A="},
		},
		PrepRequest: addValidSignature,
	},
	{Label: "Unsubcribe Invalid URN", URL: receiveURL, Data: invalidURNUnsubscribed, ExpectedRespStatus: 400, ExpectedBodyContains: "invalid viber id", PrepRequest: addValidSignature},
	{Label: "Conversation Started", URL: receiveURL, Data: validConversationStarted, ExpectedRespStatus: 200, ExpectedBodyContains: "ignored conversation start", PrepRequest: addValidSignature},
	{Label: "Unexpected event", URL: receiveURL, Data: unexpectedEvent, ExpectedRespStatus: 400,
		ExpectedBodyContains: "not handled, unknown event: unexpected", PrepRequest: addValidSignature},
	{Label: "Message missing text", URL: receiveURL, Data: rejectedMessage, ExpectedRespStatus: 400, ExpectedBodyContains: "missing text or media in message in request body", PrepRequest: addValidSignature},
	{Label: "Picture missing media", URL: receiveURL, Data: rejectedPicture, ExpectedRespStatus: 400, ExpectedBodyContains: "missing text or media in message in request body", PrepRequest: addValidSignature},
	{Label: "Video missing media", URL: receiveURL, Data: rejectedVideo, ExpectedRespStatus: 400, ExpectedBodyContains: "missing text or media in message in request body", PrepRequest: addValidSignature},

	{Label: "Valid Contact receive", URL: receiveURL, Data: validReceiveContact, ExpectedRespStatus: 200, ExpectedBodyContains: "Accepted",
		ExpectedMsgText: Sp("Alex: +12067799191"), ExpectedURN: "viber:xy5/5y6O81+/kbWHpLhBoA==", ExpectedExternalID: "4987381189870374000",
		PrepRequest: addValidSignature},
	{Label: "Valid URL receive", URL: receiveURL, Data: validReceiveURL, ExpectedRespStatus: 200, ExpectedBodyContains: "Accepted",
		ExpectedMsgText: Sp("http://foo.com/"), ExpectedURN: "viber:xy5/5y6O81+/kbWHpLhBoA==", ExpectedExternalID: "4987381189870374000",
		PrepRequest: addValidSignature},

	{Label: "Valid Location receive", URL: receiveURL, Data: validReceiveLocation, ExpectedRespStatus: 200, ExpectedBodyContains: "Accepted",
		ExpectedMsgText: Sp("incoming msg"), ExpectedURN: "viber:xy5/5y6O81+/kbWHpLhBoA==", ExpectedExternalID: "4987381189870374000",
		ExpectedAttachments: []string{"geo:1.200000,-1.300000"}, PrepRequest: addValidSignature},
	{Label: "Valid Sticker", URL: receiveURL, Data: validSticker, ExpectedRespStatus: 200, ExpectedBodyContains: "Accepted",
		ExpectedMsgText: Sp("incoming msg"), ExpectedURN: "viber:xy5/5y6O81+/kbWHpLhBoA==", ExpectedExternalID: "4987381189870374000",
		ExpectedAttachments: []string{"https://viber.github.io/docs/img/stickers/40133.png"}, PrepRequest: addValidSignature},
}

var testWelcomeMessageCases = []IncomingTestCase{
	{
		Label:                "Receive Valid",
		URL:                  receiveURL,
		Data:                 validMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("incoming msg"),
		ExpectedURN:          "viber:xy5/5y6O81+/kbWHpLhBoA==",
		ExpectedExternalID:   "4987381189870374000",
		PrepRequest:          addValidSignature},
	{
		Label:                "Conversation Started",
		URL:                  receiveURL,
		Data:                 validConversationStarted,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `{"auth_token":"Token","text":"Welcome to VP, Please subscribe here for more.","type":"text","tracking_data":"0"}`,
		ExpectedEvents: []ExpectedEvent{
			{Type: courier.EventTypeWelcomeMessage, URN: "viber:xy5/5y6O81+/kbWHpLhBoA=="},
		},
		PrepRequest: addValidSignature,
	},
}

func addValidSignature(r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewBuffer(body))
	sig := calculateSignature("Token", body)
	r.Header.Set(viberSignatureHeader, string(sig))
}

func TestSignature(t *testing.T) {
	sig := calculateSignature(
		"44b935abea139fd6-53fa53b32559c4a6-12dbd3d883b06835",
		[]byte(`{"event":"unsubscribed","timestamp":1516678387902,"user_id":"KMMqtlNTDxIm/5bZhdQ5uA==","message_token":5136431130449316903}`),
	)

	if !hmac.Equal([]byte(sig), []byte("d84d8648b402a2737838fea4da41d903d1af1aed92466b1758828ad27e31a9f9")) {
		t.Errorf("hex digest not equal: %s", sig)
	}
}

func addInvalidSignature(r *http.Request) {
	r.Header.Set(viberSignatureHeader, "invalidsig")
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), testCases)
	RunIncomingTestCases(t, testChannelsWithWelcomeMessage, newHandler(), testWelcomeMessageCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
	RunChannelBenchmarks(b, testChannelsWithWelcomeMessage, newHandler(), testWelcomeMessageCases)
}
