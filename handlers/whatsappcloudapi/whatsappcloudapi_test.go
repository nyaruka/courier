package whatsappcloudapi

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	. "github.com/nyaruka/courier/handlers"
	"github.com/stretchr/testify/assert"
)

var testChannelsCWA = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "CWA", "12345", "", map[string]interface{}{courier.ConfigAuthToken: "a123"}),
}

var helloCWA = `{
    "object": "whatsapp_business_account",
    "entry": [
        {
            "id": "8856996819413533",
            "changes": [
                {
                    "value": {
                        "messaging_product": "whatsapp",
                        "metadata": {
                            "display_phone_number": "12345",
                            "phone_number_id": "27681414235104944"
                        },
                        "contacts": [
                            {
                                "profile": {
                                    "name": "Kerry Fisher"
                                },
                                "wa_id": "5678"
                            }
                        ],
                        "messages": [
                            {
                                "from": "5678",
                                "id": "external_id",
                                "timestamp": "1454119029",
                                "text": {
                                    "body": "Hello World"
                                },
                                "type": "text"
                            }
                        ]
                    },
                    "field": "messages"
                }
            ]
        }
    ]
}`

var testCasesCWA = []ChannelHandleTestCase{
	{Label: "Receive Message CWA", URL: "/c/cwa/receive", Data: helloCWA, Status: 200, Response: "Handled", NoQueueErrorCheck: true, NoInvalidChannelCheck: true,
		Text: Sp("Hello World"), URN: Sp("whatsapp:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC)),
		PrepRequest: addValidSignature},
}

func addValidSignature(r *http.Request) {
	body, _ := handlers.ReadBody(r, 100000)
	sig, _ := fbCalculateSignature("fb_app_secret", body)
	r.Header.Set(signatureHeader, fmt.Sprintf("sha1=%s", string(sig)))
}

func addInvalidSignature(r *http.Request) {
	r.Header.Set(signatureHeader, "invalidsig")
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannelsCWA, newHandler("CWA", "Cloud API WhatsApp", false), testCasesCWA)
}

func TestSigning(t *testing.T) {
	tcs := []struct {
		Body      string
		Signature string
	}{
		{
			"hello world",
			"308de7627fe19e92294c4572a7f831bc1002809d",
		},
		{
			"hello world2",
			"ab6f902b58b9944032d4a960f470d7a8ebfd12b7",
		},
	}

	for i, tc := range tcs {
		sig, err := fbCalculateSignature("sesame", []byte(tc.Body))
		assert.NoError(t, err)
		assert.Equal(t, tc.Signature, sig, "%d: mismatched signature", i)
	}
}
