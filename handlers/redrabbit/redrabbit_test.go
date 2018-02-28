package redrabbit

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: "SENT", ResponseStatus: 200,
		URLParams: map[string]string{
			"LoginName":         "Username",
			"Password":          "Password",
			"Tracking":          "1",
			"Mobtyp":            "1",
			"MessageRecipients": "250788383383",
			"MessageBody":       "Simple Message",
			"SenderName":        "2020",
		},
		SendPrep: setSendURL},
	{Label: "Unicode Send",
		Text: "☺", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: "SENT", ResponseStatus: 200,
		URLParams: map[string]string{
			"LoginName":         "Username",
			"Password":          "Password",
			"Tracking":          "1",
			"Mobtyp":            "1",
			"MessageRecipients": "250788383383",
			"MessageBody":       "☺",
			"SenderName":        "2020",
			"MsgTyp":            "9",
		},
		SendPrep: setSendURL},
	{Label: "Longer Unicode Send",
		Text:         "This is a message more than seventy characters with some unicode ☺ in them",
		URN:          "tel:+250788383383",
		Status:       "W",
		ResponseBody: "SENT", ResponseStatus: 200,
		URLParams: map[string]string{
			"LoginName":         "Username",
			"Password":          "Password",
			"Tracking":          "1",
			"Mobtyp":            "1",
			"MessageRecipients": "250788383383",
			"MessageBody":       "This is a message more than seventy characters with some unicode ☺ in them",
			"SenderName":        "2020",
			"MsgTyp":            "10",
		},
		SendPrep: setSendURL},
	{Label: "Long Send",
		Text:         "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		URN:          "tel:+250788383383",
		Status:       "W",
		ResponseBody: "SENT", ResponseStatus: 200,
		URLParams: map[string]string{
			"LoginName":         "Username",
			"Password":          "Password",
			"Tracking":          "1",
			"Mobtyp":            "1",
			"MessageRecipients": "250788383383",
			"MessageBody":       "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
			"SenderName":        "2020",
			"MsgTyp":            "5",
		},
		SendPrep: setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: "SENT", ResponseStatus: 200,
		URLParams: map[string]string{
			"LoginName":         "Username",
			"Password":          "Password",
			"Tracking":          "1",
			"Mobtyp":            "1",
			"MessageRecipients": "250788383383",
			"MessageBody":       "My pic!\nhttps://foo.bar/image.jpg",
			"SenderName":        "2020",
		},
		SendPrep: setSendURL},
	{Label: "Error Sending",
		Text: "Error Sending", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: "Error", ResponseStatus: 403,
		SendPrep: setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "RR", "2020", "US",
		map[string]interface{}{
			"password": "Password",
			"username": "Username",
		},
	)

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
