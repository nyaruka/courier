package messagebird

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/golang-jwt/jwt/v5"
	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MBD", "18005551212", "US", map[string]interface{}{
		"secret":     "my_super_secret", // secret key to sign for sig
		"auth_token": "authtoken",       //API bearer token
	}),
}

const (
	receiveURL    = "/c/mbd/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	validReceive  = `{"receiver":"18005551515","sender":"188885551515","message":"Test again","date":1690386569,"date_utc":1690418969,"reference":"1","id":"b6aae1b5dfb2427a8f7ea6a717ba31a9","message_id":"3b53c137369242138120d6b0b2122607","recipient":"18005551515","originator":"188885551515","body":"Test 3","createdDatetime":"2023-07-27T00:49:29+00:00","mms":false}`
	validSecret   = "my_super_secret"
	invalidSecret = "bad_secret"
)

func addValidSignature(r *http.Request) {
	body, _ := ReadBody(r, maxRequestBodyBytes)
	sig := calculateSignature(body)
	t := jwt.NewWithClaims(jwt.SigningMethodHS256,
		jwt.MapClaims{
			"iss":          "MessageBird",
			"nbf":          1690306305,
			"jti":          "e92cf079-362d-4813-ab40-bbdd938bdc6d",
			"payload_hash": sig,
		})

	signedJWT, _ := t.SignedString([]byte(validSecret))
	r.Header.Set(signatureHeader, signedJWT)
}

func addInvalidSignature(r *http.Request) {
	body, _ := ReadBody(r, maxRequestBodyBytes)
	sig := calculateSignature(body)
	t := jwt.NewWithClaims(jwt.SigningMethodHS256,
		jwt.MapClaims{
			"iss":          "MessageBird",
			"nbf":          1690306305,
			"jti":          "e92cf079-362d-4813-ab40-bbdd938bdc6d",
			"payload_hash": sig,
		})

	signedJWT, _ := t.SignedString([]byte(invalidSecret))
	r.Header.Set("Messagebird-Signature-Jwt", signedJWT)
}

func addInvalidBodyHash(r *http.Request) {
	body, _ := ReadBody(r, maxRequestBodyBytes)
	body = body + []byte("bad")
	preHashSignature := sha256.Sum256(body)
	sig := hex.EncodeToString(preHashSignature[:])
	t := jwt.NewWithClaims(jwt.SigningMethodHS256,
		jwt.MapClaims{
			"iss":          "MessageBird",
			"nbf":          1690306305,
			"jti":          "e92cf079-362d-4813-ab40-bbdd938bdc6d",
			"payload_hash": sig,
		})

	signedJWT, _ := t.SignedString([]byte(validSecret))
	r.Header.Set("Messagebird-Signature-Jwt", signedJWT)
}

var sigtestCases = []ChannelHandleTestCase{
	{
		Label:                "Receive Valid w Signature",
		Headers:              map[string]string{"Content-Type": "application/json"},
		URL:                  receiveURL,
		Data:                 validReceive,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Message Accepted",
		ExpectedMsgText:      Sp("Test 3"),
		ExpectedURN:          "tel:188885551515",
		ExpectedDate:         time.Date(2023, time.July, 27, 00, 49, 29, 0, time.UTC),
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Bad JWT Signature",
		Headers:              map[string]string{"Content-Type": "application/json"},
		URL:                  receiveURL,
		Data:                 validReceive,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: `{"message":"Error","data":[{"type":"error","error":"token signature is invalid: signature is invalid"}]}`,
		PrepRequest:          addInvalidSignature,
	},
	{
		Label:                "Receive Valid w Signature but non-matching body hash",
		Headers:              map[string]string{"Content-Type": "application/json"},
		URL:                  receiveURL,
		Data:                 validReceive,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Message Accepted",
		ExpectedMsgText:      Sp("Test 3"),
		ExpectedURN:          "tel:188885551515",
		ExpectedDate:         time.Date(2023, time.July, 27, 00, 49, 29, 0, time.UTC),
		PrepRequest:          addInvalidBodyHash,
	},
	{
		Label:                "Bad JSON",
		Headers:              map[string]string{"Content-Type": "application/json"},
		URL:                  receiveURL,
		Data:                 "empty",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: `{"message":"Error","data":[{"type":"error","error":"unable to parse request JSON: invalid character 'e' looking for beginning of value"}]}`,
		PrepRequest:          addValidSignature,
	},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler("MBD", "Messagebird", true), sigtestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler("MBD", "Messagebird", true), sigtestCases)
}

func setSmsSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	smsURL = s.URL
}

func setMmsSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	mmsURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message ☺",
		MsgURN:              "tel:188885551515",
		MockResponseBody:    "",
		MockResponseStatus:  200,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Authorization": "AccessKey authtoken"},
		ExpectedRequestBody: `{"recipients":["188885551515"],"originator":"18005551212","body":"Simple Message ☺"}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "",
		SendPrep:            setSmsSendURL,
	},
	{
		Label:               "Send with text and image",
		MsgText:             "Simple Message ☺",
		MsgURN:              "tel:188885551515",
		MsgAttachments:      []string{"image:https://foo.bar/image.jpg"},
		MockResponseBody:    "",
		MockResponseStatus:  200,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Authorization": "AccessKey authtoken"},
		ExpectedRequestBody: `{"recipients":["188885551515"],"originator":"18005551212","body":"Simple Message ☺","mediaUrls":["https://foo.bar/image.jpg"]}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "",
		SendPrep:            setMmsSendURL,
	},
	{
		Label:               "Send with image only",
		MsgURN:              "tel:188885551515",
		MsgAttachments:      []string{"image/jpg:https://foo.bar/image.jpg"},
		MockResponseBody:    "",
		MockResponseStatus:  200,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Authorization": "AccessKey authtoken"},
		ExpectedRequestBody: `{"recipients":["188885551515"],"originator":"18005551212","mediaUrls":["https://foo.bar/image.jpg"]}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "",
		SendPrep:            setMmsSendURL,
	},
}

func TestSending(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MBD", "18005551212", "US", map[string]interface{}{
		"secret":     "my_super_secret", // secret key to sign for sig
		"auth_token": "authtoken",
	})
	RunChannelSendTestCases(t, defaultChannel, newHandler("MBD", "Messagebird", false), defaultSendTestCases, []string{"my_super_secret", "authtoken"}, nil)
}
