package redrabbit

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	sendURL = s.URL
}

var defaultSendTestCases = []OutgoingTestCase{
	{Label: "Plain Send",
		MsgText: "Simple Message", MsgURN: "tel:+250788383383",
		ExpectedMsgStatus: "W",
		MockResponseBody:  "SENT", MockResponseStatus: 200,
		ExpectedURLParams: map[string]string{
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
		MsgText: "☺", MsgURN: "tel:+250788383383",
		ExpectedMsgStatus: "W",
		MockResponseBody:  "SENT", MockResponseStatus: 200,
		ExpectedURLParams: map[string]string{
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
		MsgText:           "This is a message more than seventy characters with some unicode ☺ in them",
		MsgURN:            "tel:+250788383383",
		ExpectedMsgStatus: "W",
		MockResponseBody:  "SENT", MockResponseStatus: 200,
		ExpectedURLParams: map[string]string{
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
		MsgText:           "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:            "tel:+250788383383",
		ExpectedMsgStatus: "W",
		MockResponseBody:  "SENT", MockResponseStatus: 200,
		ExpectedURLParams: map[string]string{
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
		MsgText: "My pic!", MsgURN: "tel:+250788383383", MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		ExpectedMsgStatus: "W",
		MockResponseBody:  "SENT", MockResponseStatus: 200,
		ExpectedURLParams: map[string]string{
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
		MsgText: "Error Sending", MsgURN: "tel:+250788383383",
		ExpectedMsgStatus: "E",
		MockResponseBody:  "Error", MockResponseStatus: 403,
		SendPrep: setSendURL},
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "RR", "2020", "US",
		map[string]any{
			"password": "Password",
			"username": "Username",
		},
	)

	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"Password"}, nil)
}
