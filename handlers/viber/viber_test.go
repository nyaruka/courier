package viber

import (
	"bytes"
	"crypto/hmac"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

// setSend takes care of setting the sendURL to call
func setSendURL(server *httptest.Server, h courier.ChannelHandler, channel courier.Channel, msg courier.Msg) {
	sendURL = server.URL
}

func buildMockAttachmentService(testCases []ChannelSendTestCase) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := w.Header()
		if r.Method == http.MethodHead {
			headers["Content-Length"] = []string{"123456"}
		}
		w.Write([]byte(""))
	}))

	// update our tests media urls
	for c := range testCases {
		if len(testCases[c].Attachments) > 0 {
			for i, a := range testCases[c].Attachments {
				mediaType, mediaURL := SplitAttachment(a)
				parts := strings.Split(mediaURL, "/")
				testCases[c].Attachments[i] = fmt.Sprintf("%s:%s/%s", mediaType, server.URL, parts[len(parts)-1])
			}
		}
		testCases[c].RequestBody = strings.Replace(testCases[c].RequestBody, "{{ SERVER_URL }}", server.URL, -1)
	}

	return server
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "viber:xy5/5y6O81+/kbWHpLhBoA==",
		Status: "W", ResponseStatus: 200,
		ResponseBody: `{"status":0,"status_message":"ok","message_token":4987381194038857789}`,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		RequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"Simple Message","type":"text","tracking_data":"10"}`,
		SendPrep:    setSendURL},
	{Label: "Long Send",
		Text:   "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		URN:    "viber:xy5/5y6O81+/kbWHpLhBoA==",
		Status: "W", ResponseStatus: 200,
		ResponseBody: `{"status":0,"status_message":"ok","message_token":4987381194038857789}`,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		RequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"I need to keep adding more things to make it work","type":"text","tracking_data":"10"}`,
		SendPrep:    setSendURL},
	{Label: "Unicode Send",
		Text: "☺", URN: "viber:xy5/5y6O81+/kbWHpLhBoA==",
		Status: "W", ResponseStatus: 200,
		ResponseBody: `{"status":0,"status_message":"ok","message_token":4987381194038857789}`,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		RequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"☺","type":"text","tracking_data":"10"}`,
		SendPrep:    setSendURL},
	{Label: "Quick Reply",
		Text: "Are you happy?", URN: "viber:xy5/5y6O81+/kbWHpLhBoA==", QuickReplies: []string{"Yes", "No"},
		Status: "W", ResponseStatus: 200,
		ResponseBody: `{"status":0,"status_message":"ok","message_token":4987381194038857789}`,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		RequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"Are you happy?","type":"text","tracking_data":"10","keyboard":{"Type":"keyboard","DefaultHeight":true,"Buttons":[{"ActionType":"reply","ActionBody":"Yes","Text":"Yes","TextSize":"regular"},{"ActionType":"reply","ActionBody":"No","Text":"No","TextSize":"regular"}]}}`,
		SendPrep:    setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "viber:xy5/5y6O81+/kbWHpLhBoA==", Attachments: []string{"image/jpeg:https://localhost/image.jpg"},
		Status: "W", ResponseStatus: 200,
		ResponseBody: `{"status":0,"status_message":"ok","message_token":4987381194038857789}`,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		RequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"My pic!","type":"picture","tracking_data":"10","media":"{{ SERVER_URL }}/image.jpg"}`,
		SendPrep:    setSendURL},
	{Label: "Long Description with Attachment",
		Text: "Text description is longer that 10 characters",
		URN:  "viber:xy5/5y6O81+/kbWHpLhBoA==", Attachments: []string{"image/jpeg:https://localhost/image.jpg"},
		Status: "W", ResponseStatus: 200,
		ResponseBody: `{"status":0,"status_message":"ok","message_token":4987381194038857789}`,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		RequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"description is longer that 10 characters","type":"text","tracking_data":"10"}`,
		SendPrep:    setSendURL},
	{Label: "Send Attachment Video",
		Text: "My video!", URN: "viber:xy5/5y6O81+/kbWHpLhBoA==", Attachments: []string{"video/mp4:https://localhost/video.mp4"},
		Status: "W", ResponseStatus: 200,
		ResponseBody: `{"status":0,"status_message":"ok","message_token":4987381194038857789}`,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		RequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"My video!","type":"video","tracking_data":"10","media":"{{ SERVER_URL }}/video.mp4","size":123456}`,
		SendPrep:    setSendURL},
	{Label: "Send Attachment Audio",
		Text: "My audio!", URN: "viber:xy5/5y6O81+/kbWHpLhBoA==", Attachments: []string{"audio/mp3:https://localhost/audio.mp3"},
		Status: "W", ResponseStatus: 200,
		ResponseBody: `{"status":0,"status_message":"ok","message_token":4987381194038857789}`,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		RequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"My audio!","type":"file","tracking_data":"10","media":"{{ SERVER_URL }}/audio.mp3","size":123456,"file_name":"Audio"}`,
		SendPrep:    setSendURL},
	{Label: "Got non-0 response",
		Text: "Simple Message", URN: "viber:xy5/5y6O81+/kbWHpLhBoA==",
		Status: "E", ResponseStatus: 200,
		ResponseBody: `{"status":3,"status_message":"InvalidToken"}`,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		RequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"Simple Message","type":"text","tracking_data":"10"}`,
		SendPrep:    setSendURL},
	{Label: "Got Invalid JSON response",
		Text: "Simple Message", URN: "viber:xy5/5y6O81+/kbWHpLhBoA==",
		Status: "E", ResponseStatus: 200,
		ResponseBody: `invalidJSON`,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		RequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"Simple Message","type":"text","tracking_data":"10"}`,
		SendPrep:    setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "viber:xy5/5y6O81+/kbWHpLhBoA==",
		Status: "E", ResponseStatus: 401,
		ResponseBody: `{"status":"5"}`,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		RequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"Error Message","type":"text","tracking_data":"10"}`,
		SendPrep:    setSendURL},
}

var invalidTokenSendTestCases = []ChannelSendTestCase{
	{Label: "Invalid token", Error: "missing auth token in config"},
}

func TestSending(t *testing.T) {
	attachmentService := buildMockAttachmentService(defaultSendTestCases)
	defer attachmentService.Close()

	maxMsgLength = 160
	descriptionMaxLength = 10
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "VP", "2020", "",
		map[string]interface{}{
			courier.ConfigAuthToken: "Token",
		})
	var invalidTokenChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "VP", "2020", "",
		map[string]interface{}{},
	)
	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
	RunChannelSendTestCases(t, invalidTokenChannel, newHandler(), invalidTokenSendTestCases, nil)
}

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "VP", "2020", "", map[string]interface{}{
		courier.ConfigAuthToken: "Token",
	}),
}

var testChannelsWithWelcomeMessage = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "VP", "2020", "", map[string]interface{}{
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

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: validMsg, Status: 200, Response: "Accepted",
		Text: Sp("incoming msg"), URN: Sp("viber:xy5/5y6O81+/kbWHpLhBoA=="), ExternalID: Sp("4987381189870374000"),
		PrepRequest: addValidSignature},
	{Label: "Receive invalid signature", URL: receiveURL, Data: validMsg, Status: 400, Response: "invalid request signature",
		PrepRequest: addInvalidSignature},
	{Label: "Receive invalid JSON", URL: receiveURL, Data: invalidJSON, Status: 400, Response: "unable to parse request JSON",
		PrepRequest: addValidSignature},
	{Label: "Receive invalid URN", URL: receiveURL, Data: invalidURNMsg, Status: 400, Response: "invalid viber id",
		PrepRequest: addValidSignature},
	{Label: "Receive invalid Message Type", URL: receiveURL, Data: receiveInvalidMessageType, Status: 400, Response: "unknown message type",
		PrepRequest: addValidSignature},
	{Label: "Webhook validation", URL: receiveURL, Data: webhookCheck, Status: 200, Response: "webhook valid", PrepRequest: addValidSignature},
	{Label: "Failed Status Report", URL: receiveURL, Data: failedStatusReport, Status: 200, Response: `"status":"F"`, PrepRequest: addValidSignature},
	{Label: "Delivered Status Report", URL: receiveURL, Data: deliveredStatusReport, Status: 200, Response: `Ignored`, PrepRequest: addValidSignature},
	{Label: "Subcribe", URL: receiveURL, Data: validSubscribed, Status: 200, Response: "Accepted", PrepRequest: addValidSignature},
	{Label: "Subcribe Invalid URN", URL: receiveURL, Data: invalidURNSubscribed, Status: 400, Response: "invalid viber id", PrepRequest: addValidSignature},
	{Label: "Unsubcribe", URL: receiveURL, Data: validUnsubscribed, Status: 200, Response: "Accepted", ChannelEvent: Sp(string(courier.StopContact)), PrepRequest: addValidSignature},
	{Label: "Unsubcribe Invalid URN", URL: receiveURL, Data: invalidURNUnsubscribed, Status: 400, Response: "invalid viber id", PrepRequest: addValidSignature},
	{Label: "Conversation Started", URL: receiveURL, Data: validConversationStarted, Status: 200, Response: "ignored conversation start", PrepRequest: addValidSignature},
	{Label: "Unexpected event", URL: receiveURL, Data: unexpectedEvent, Status: 400,
		Response: "not handled, unknown event: unexpected", PrepRequest: addValidSignature},
	{Label: "Message missing text", URL: receiveURL, Data: rejectedMessage, Status: 400, Response: "missing text or media in message in request body", PrepRequest: addValidSignature},
	{Label: "Picture missing media", URL: receiveURL, Data: rejectedPicture, Status: 400, Response: "missing text or media in message in request body", PrepRequest: addValidSignature},
	{Label: "Video missing media", URL: receiveURL, Data: rejectedVideo, Status: 400, Response: "missing text or media in message in request body", PrepRequest: addValidSignature},

	{Label: "Valid Contact receive", URL: receiveURL, Data: validReceiveContact, Status: 200, Response: "Accepted",
		Text: Sp("Alex: +12067799191"), URN: Sp("viber:xy5/5y6O81+/kbWHpLhBoA=="), ExternalID: Sp("4987381189870374000"),
		PrepRequest: addValidSignature},
	{Label: "Valid URL receive", URL: receiveURL, Data: validReceiveURL, Status: 200, Response: "Accepted",
		Text: Sp("http://foo.com/"), URN: Sp("viber:xy5/5y6O81+/kbWHpLhBoA=="), ExternalID: Sp("4987381189870374000"),
		PrepRequest: addValidSignature},

	{Label: "Valid Location receive", URL: receiveURL, Data: validReceiveLocation, Status: 200, Response: "Accepted",
		Text: Sp("incoming msg"), URN: Sp("viber:xy5/5y6O81+/kbWHpLhBoA=="), ExternalID: Sp("4987381189870374000"),
		Attachment: Sp("geo:1.200000,-1.300000"), PrepRequest: addValidSignature},
}

var testWelcomeMessageCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: validMsg, Status: 200, Response: "Accepted",
		Text: Sp("incoming msg"), URN: Sp("viber:xy5/5y6O81+/kbWHpLhBoA=="), ExternalID: Sp("4987381189870374000"),
		PrepRequest: addValidSignature},
	{Label: "Conversation Started", URL: receiveURL, Data: validConversationStarted, Status: 200, Response: `{"auth_token":"Token","text":"Welcome to VP, Please subscribe here for more.","type":"text","tracking_data":"\u0000"}`, PrepRequest: addValidSignature},
}

func addValidSignature(r *http.Request) {
	body, _ := ioutil.ReadAll(r.Body)
	r.Body = ioutil.NopCloser(bytes.NewBuffer(body))
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

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
	RunChannelTestCases(t, testChannelsWithWelcomeMessage, newHandler(), testWelcomeMessageCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
	RunChannelBenchmarks(b, testChannelsWithWelcomeMessage, newHandler(), testWelcomeMessageCases)
}
