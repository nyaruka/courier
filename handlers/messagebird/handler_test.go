package messagebird

import (
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/courier/utils/clogs"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MBD", "18005551212", "US", []string{urns.Phone.Prefix}, map[string]any{
		"secret":     "my_super_secret", // secret key to sign for sig
		"auth_token": "authtoken",       //API bearer token
	}),
}

const (
	receiveURL            = "/c/mbd/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"
	validReceive          = "receiver=18005551515&body=Test+again&date=1690386569&date_utc=1690418969&reference=1&id=b6aae1b5dfb2427a8f7ea6a717ba31a9&id=3b53c137369242138120d6b0b2122607&recipient=18005551515&originator=18885551515&body=Test+3&CreatedDatetime=2023-07-27T00%3A49%3A29%2B00%3A00"
	invalidReceive        = "originator=18885551515&recipient=18005551515&&CreatedDatetime=2023-07-27T00%3A49%3A29%2B00%3A00"
	validReceiveShortCode = "shortcode=51515&message=Test+again&date=1690386569&date_utc=1690418969&reference=1&id=b6aae1b5dfb2427a8f7ea6a717ba31a9&mid=3b53c137369242138120d6b0b2122607&recipient=18005551515&originator=18885551515&body=Test+3&receive_datetime=20230727004929"
	validReceiveMMS       = "receiver=18005551515&message=Test+again&date=1690386569&date_utc=1690418969&reference=1&id=b6aae1b5dfb2427a8f7ea6a717ba31a9&message_id=3b53c137369242138120d6b0b2122607&recipient=18005551515&originator=18885551515&mediaURLs=https%3A%2F%2Ffoo.bar%2Fimage.jpg&CreatedDatetime=2023-07-27T00%3A49%3A29%2B00%3A00"
	statusBaseURL         = "/c/mbd/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?datacoding=plain&id=b6aae1b5dfb2427a8f7ea6a717ba31a9&mccmnc=310010&messageLength=4&messagePartCount=1&ported=0&price%5Bamount%5D=0.000&price%5Bcurrency%5D=USD&recipient=18885551515&reference=26&statusDatetime=2023-07-28T17%3A57%3A12%2B00%3A00"
	validSecret           = "my_super_secret"
	validResponse         = `{"id":"efa6405d518d4c0c88cce11f7db775fb","href":"https://rest.messagebird.com/mms/efa6405d518d4c0c88cce11f7db775fb","direction":"mt","originator":"+188885551515","subject":"Great logo","body":"Hi! Please have a look at this very nice logo of this cool company.","reference":"the-customers-reference","mediaUrls":["https://www.messagebird.com/assets/images/og/messagebird.gif"],"scheduledDatetime":null,"createdDatetime":"2017-09-01T10:00:00+00:00","recipients":{"totalCount":1,"totalSentCount":1,"totalDeliveredCount":0,"totalDeliveryFailedCount":0,"items":[{"recipient":18005551515,"status":"sent","statusDatetime":"2017-09-01T10:00:00+00:00"}]}}`
	invalidSecret         = "bad_secret"
)

func addValidSignature(r *http.Request) {
	var bodysig string
	if r.Body == nil {
		bodysig = calculateSignature([]byte(""))
	} else {
		body, _ := ReadBody(r, maxRequestBodyBytes)
		if len(body) > 0 {
			bodysig = calculateSignature(body)
		} else {
			bodysig = calculateSignature([]byte(""))
		}
	}

	urlsig := calculateSignature([]byte("https://localhost" + r.URL.Path))
	t := jwt.NewWithClaims(jwt.SigningMethodHS256,
		jwt.MapClaims{
			"iss":          "MessageBird",
			"nbf":          1690306305,
			"jti":          "e92cf079-362d-4813-ab40-bbdd938bdc6d",
			"payload_hash": bodysig,
			"url_hash":     urlsig,
		})

	signedJWT, _ := t.SignedString([]byte(validSecret))
	r.Header.Set(signatureHeader, signedJWT)
}

func addInvalidSignature(r *http.Request) {
	body, _ := ReadBody(r, maxRequestBodyBytes)
	bodysig := calculateSignature(body)
	urlsig := calculateSignature([]byte("https://localhost" + r.URL.Path))
	t := jwt.NewWithClaims(jwt.SigningMethodHS256,
		jwt.MapClaims{
			"iss":          "MessageBird",
			"nbf":          1690306305,
			"jti":          "e92cf079-362d-4813-ab40-bbdd938bdc6d",
			"payload_hash": bodysig,
			"url_hash":     urlsig,
		})

	signedJWT, _ := t.SignedString([]byte(invalidSecret))
	r.Header.Set("Messagebird-Signature-Jwt", signedJWT)
}

func addInvalidBodyHash(r *http.Request) {
	body, _ := ReadBody(r, maxRequestBodyBytes)
	bad_bytes := []byte("bad")
	body = append(body, bad_bytes[:]...)
	urlsig := calculateSignature([]byte("https://localhost" + r.URL.Path))
	bodysig := calculateSignature(body)
	t := jwt.NewWithClaims(jwt.SigningMethodHS256,
		jwt.MapClaims{
			"iss":          "MessageBird",
			"nbf":          1690306305,
			"jti":          "e92cf079-362d-4813-ab40-bbdd938bdc6d",
			"payload_hash": bodysig,
			"url_hash":     urlsig,
		})

	signedJWT, _ := t.SignedString([]byte(validSecret))
	r.Header.Set("Messagebird-Signature-Jwt", signedJWT)
}

var defaultReceiveTestCases = []IncomingTestCase{
	{
		Label:                "Receive Valid text w Signature",
		Headers:              map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		URL:                  receiveURL,
		Data:                 validReceive,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "OK",
		ExpectedMsgText:      Sp("Test 3"),
		ExpectedURN:          "tel:+18885551515",
		ExpectedDate:         time.Date(2023, time.July, 27, 00, 49, 29, 0, time.UTC),
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Valid text w shortcode date",
		Headers:              map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		URL:                  receiveURL,
		Data:                 validReceiveShortCode,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "OK",
		ExpectedMsgText:      Sp("Test again"),
		ExpectedURN:          "tel:+18885551515",
		ExpectedDate:         time.Date(2023, time.July, 27, 00, 49, 29, 0, time.UTC),
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Valid w image w Signature",
		Headers:              map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		URL:                  receiveURL,
		Data:                 validReceiveMMS,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "OK",
		ExpectedAttachments:  []string{"https://foo.bar/image.jpg"},
		ExpectedURN:          "tel:+18885551515",
		ExpectedDate:         time.Date(2023, time.July, 27, 00, 49, 29, 0, time.UTC),
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Bad JWT Signature",
		Headers:              map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		URL:                  receiveURL,
		Data:                 validReceive,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: `{"message":"Error","data":[{"type":"error","error":"token signature is invalid: signature is invalid"}]}`,
		PrepRequest:          addInvalidSignature,
	},
	{
		Label:                "Missing JWT Signature Header",
		Headers:              map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		URL:                  receiveURL,
		Data:                 validReceive,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: `{"message":"Error","data":[{"type":"error","error":"missing request signature"}]}`,
	},
	{
		Label:                "Receive Valid w Signature but non-matching body hash",
		Headers:              map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		URL:                  receiveURL,
		Data:                 validReceive,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: `{"message":"Error","data":[{"type":"error","error":"invalid request signature, signature doesn't match expected signature for body."}]}`,
		PrepRequest:          addInvalidBodyHash,
	},
	{
		Label:                "Empty Body",
		Headers:              map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		URL:                  receiveURL,
		Data:                 invalidReceive,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: `{"message":"Error","data":[{"type":"error","error":"no text or media"}]}`,
		PrepRequest:          addValidSignature,
	},
	{
		Label:              "Status Valid",
		URL:                statusBaseURL + "&status=sent",
		ExpectedRespStatus: 200,
		ExpectedStatuses:   []ExpectedStatus{{MsgID: 26, Status: courier.MsgStatusSent}},
	},
	{
		Label:              "Status- Stop Received",
		URL:                statusBaseURL + "&status=delivery_failed&statusErrorCode=103",
		ExpectedRespStatus: 200,
		ExpectedStatuses:   []ExpectedStatus{{MsgID: 26, Status: courier.MsgStatusFailed}},
		ExpectedEvents: []ExpectedEvent{
			{Type: courier.EventTypeStopContact, URN: "tel:+18885551515"},
		},
		ExpectedErrors: []*clogs.Error{courier.ErrorExternal("103", "Contact has sent 'stop'")},
	},
	{
		Label:                "Receive Invalid Status",
		URL:                  statusBaseURL + "&status=expiryttd",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: `{"message":"Error","data":[{"type":"error","error":"unknown status 'expiryttd', must be one of 'queued', 'failed', 'sent', 'delivered', or 'undelivered'"}]}`,
	},
}

func TestReceiving(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler("MBD", "Messagebird", true), defaultReceiveTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler("MBD", "Messagebird", true), defaultReceiveTestCases)
}

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message ☺",
		MsgURN:  "tel:188885551515",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://rest.messagebird.com/messages": {
				httpx.NewMockResponse(200, nil, []byte(validResponse)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{"Content-Type": "application/json", "Authorization": "AccessKey authtoken"},
			Body:    `{"recipients":["188885551515"],"reference":"10","originator":"18005551212","body":"Simple Message ☺"}`,
		}},
		ExpectedExtIDs: []string{"efa6405d518d4c0c88cce11f7db775fb"},
	},
	{
		Label:          "Send with text and image",
		MsgText:        "Simple Message ☺",
		MsgURN:         "tel:188885551515",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://rest.messagebird.com/mms": {
				httpx.NewMockResponse(200, nil, []byte(validResponse)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{"Content-Type": "application/json", "Authorization": "AccessKey authtoken"},
			Body:    `{"recipients":["188885551515"],"reference":"10","originator":"18005551212","body":"Simple Message ☺","mediaUrls":["https://foo.bar/image.jpg"]}`,
		}},
		ExpectedExtIDs: []string{"efa6405d518d4c0c88cce11f7db775fb"},
	},
	{
		Label:          "Send with image only",
		MsgURN:         "tel:188885551515",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://rest.messagebird.com/mms": {
				httpx.NewMockResponse(200, nil, []byte(validResponse)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{"Content-Type": "application/json", "Authorization": "AccessKey authtoken"},
			Body:    `{"recipients":["188885551515"],"reference":"10","originator":"18005551212","mediaUrls":["https://foo.bar/image.jpg"]}`,
		}},
		ExpectedExtIDs: []string{"efa6405d518d4c0c88cce11f7db775fb"},
	},
	{
		Label:          "Send with two images",
		MsgURN:         "tel:188885551515",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg", "image/jpeg:https://foo.bar/image2.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://rest.messagebird.com/mms": {
				httpx.NewMockResponse(200, nil, []byte(validResponse)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{"Content-Type": "application/json", "Authorization": "AccessKey authtoken"},
			Body:    `{"recipients":["188885551515"],"reference":"10","originator":"18005551212","mediaUrls":["https://foo.bar/image.jpg","https://foo.bar/image2.jpg"]}`,
		}},
		ExpectedExtIDs: []string{"efa6405d518d4c0c88cce11f7db775fb"},
	},
	{
		Label:          "Send with video only",
		MsgURN:         "tel:188885551515",
		MsgAttachments: []string{"video/mp4:https://foo.bar/movie.mp4"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://rest.messagebird.com/mms": {
				httpx.NewMockResponse(200, nil, []byte(validResponse)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{"Content-Type": "application/json", "Authorization": "AccessKey authtoken"},
			Body:    `{"recipients":["188885551515"],"reference":"10","originator":"18005551212","mediaUrls":["https://foo.bar/movie.mp4"]}`,
		}},
		ExpectedExtIDs: []string{"efa6405d518d4c0c88cce11f7db775fb"},
	},
	{
		Label:          "Send with pdf",
		MsgURN:         "tel:188885551515",
		MsgAttachments: []string{"application/pdf:https://foo.bar/document.pdf"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://rest.messagebird.com/mms": {
				httpx.NewMockResponse(200, nil, []byte(validResponse)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{"Content-Type": "application/json", "Authorization": "AccessKey authtoken"},
			Body:    `{"recipients":["188885551515"],"reference":"10","originator":"18005551212","mediaUrls":["https://foo.bar/document.pdf"]}`,
		}},
		ExpectedExtIDs: []string{"efa6405d518d4c0c88cce11f7db775fb"},
	},
	{
		Label:   "500 on Send",
		MsgText: "Simple Message ☺",
		MsgURN:  "tel:188885551515",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://rest.messagebird.com/messages": {
				httpx.NewMockResponse(500, nil, []byte(validResponse)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{"Content-Type": "application/json", "Authorization": "AccessKey authtoken"},
			Body:    `{"recipients":["188885551515"],"reference":"10","originator":"18005551212","body":"Simple Message ☺"}`,
		}},
		ExpectedError: courier.ErrConnectionFailed,
	},
	{
		Label:   "404 on Send",
		MsgText: "Simple Message ☺",
		MsgURN:  "tel:188885551515",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://rest.messagebird.com/messages": {
				httpx.NewMockResponse(404, nil, []byte(validResponse)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{"Content-Type": "application/json", "Authorization": "AccessKey authtoken"},
			Body:    `{"recipients":["188885551515"],"reference":"10","originator":"18005551212","body":"Simple Message ☺"}`,
		}},
		ExpectedError: courier.ErrResponseStatus,
	},
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MBD", "18005551212", "US", []string{urns.Phone.Prefix}, map[string]any{
		"secret":     "my_super_secret", // secret key to sign for sig
		"auth_token": "authtoken",
	})
	RunOutgoingTestCases(t, defaultChannel, newHandler("MBD", "Messagebird", false), defaultSendTestCases, []string{"my_super_secret", "authtoken"}, nil)
}
