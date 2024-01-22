package mailgun

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

func calculateSignature(timestamp, token, signingKey string) string {
	mac := hmac.New(sha256.New, []byte(signingKey))
	mac.Write([]byte(timestamp + token))
	return hex.EncodeToString(mac.Sum(nil))
}

var incomingCases = []IncomingTestCase{
	{
		Label: "Thread start",
		URL:   "/c/mlg/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive",
		MultipartForm: map[string]string{
			"recipient":     "test@example.com",
			"sender":        "bob@acme.com",
			"subject":       "Hi there",
			"stripped-text": "Need help",
			"timestamp":     "1705798597",
			"token":         "abcdef",
			"signature":     calculateSignature("1705798597", "abcdef", "1234567890"),
			"message-id":    "<1234567890@example.com>",
		},
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Accepted",
		ExpectedMsgText:       Sp("Hi there\n\nNeed help"),
		ExpectedURN:           "mailto:bob@acme.com",
		NoQueueErrorCheck:     true, // because these currently assume error status 400
		NoInvalidChannelCheck: true,
	},
	{
		Label: "Thread reply",
		URL:   "/c/mlg/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive",
		MultipartForm: map[string]string{
			"recipient":     "test@example.com",
			"sender":        "bob@acme.com",
			"subject":       "Re: Re: Hi there",
			"stripped-text": "Sounds good",
			"timestamp":     "1705798597",
			"token":         "abcdef",
			"signature":     calculateSignature("1705798597", "abcdef", "1234567890"),
			"message-id":    "<1234567890@example.com>",
		},
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Accepted",
		ExpectedMsgText:       Sp("Sounds good"),
		ExpectedURN:           "mailto:bob@acme.com",
		NoQueueErrorCheck:     true, // because these currently assume error status 400
		NoInvalidChannelCheck: true,
	},
}

func TestIncoming(t *testing.T) {
	chs := []courier.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MLG", "test@example.com", "",
			map[string]any{
				"default_subject": "Chat with Nyaruka",
				"signing_key":     "1234567890",
				"auth_token":      "0987654321",
			},
		),
	}

	RunIncomingTestCases(t, chs, newHandler(), incomingCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	defaultAPIURL = s.URL
}

var outgoingCases = []OutgoingTestCase{
	{
		Label:              "Flow message",
		MsgText:            "Simple message ☺",
		MsgURN:             "mailto:bob@acme.com",
		MockResponseBody:   `{"id":"<20240122160441.123456789@example.com>","message":"Queued. Thank you."}`,
		MockResponseStatus: 200,
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{"Authorization": "Basic YXBpOjA5ODc2NTQzMjE="},
				Path:    "/example.com/messages",
				Form: url.Values{
					"from":    []string{"test@example.com"},
					"to":      []string{"bob@acme.com"},
					"subject": []string{"Chat with Nyaruka"},
					"text":    []string{"Simple message ☺"},
				},
			},
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "<20240122160441.123456789@example.com>",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Chat message",
		MsgText:            "How can we help?",
		MsgURN:             "mailto:bob@acme.com",
		MsgUser:            &courier.UserReference{Email: "adam@example.com", Name: "Adam"},
		MockResponseBody:   `{"id":"<20240122160441.123456789@example.com>","message":"Queued. Thank you."}`,
		MockResponseStatus: 200,
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{"Authorization": "Basic YXBpOjA5ODc2NTQzMjE="},
				Path:    "/example.com/messages",
				Form: url.Values{
					"from":    []string{"Adam <test@example.com>"},
					"to":      []string{"bob@acme.com"},
					"subject": []string{"Chat with Nyaruka"},
					"text":    []string{"How can we help?"},
				},
			},
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "<20240122160441.123456789@example.com>",
		SendPrep:           setSendURL,
	},
}

func TestOutgoing(t *testing.T) {
	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MLG", "test@example.com", "",
		map[string]any{
			"default_subject": "Chat with Nyaruka",
			"signing_key":     "1234567890",
			"auth_token":      "0987654321",
		},
	)

	RunOutgoingTestCases(t, ch, newHandler(), outgoingCases, []string{"YXBpOjA5ODc2NTQzMjE="}, nil)
}
