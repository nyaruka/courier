package redrabbit

import (
	"net/url"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

var defaultSendTestCases = []OutgoingTestCase{
	{Label: "Plain Send",
		MsgText: "Simple Message", MsgURN: "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://http1.javna.com/epicenter/GatewaySendG.asp*": {
				httpx.NewMockResponse(200, nil, []byte(`SENT`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Params: url.Values{
				"LoginName":         {"Username"},
				"Password":          {"Password"},
				"Tracking":          {"1"},
				"Mobtyp":            {"1"},
				"MessageRecipients": {"250788383383"},
				"MessageBody":       {"Simple Message"},
				"SenderName":        {"2020"},
			}}},
	},
	{Label: "Unicode Send",
		MsgText: "☺", MsgURN: "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://http1.javna.com/epicenter/GatewaySendG.asp*": {
				httpx.NewMockResponse(200, nil, []byte(`SENT`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Params: url.Values{
				"LoginName":         {"Username"},
				"Password":          {"Password"},
				"Tracking":          {"1"},
				"Mobtyp":            {"1"},
				"MessageRecipients": {"250788383383"},
				"MessageBody":       {"☺"},
				"SenderName":        {"2020"},
				"MsgTyp":            {"9"},
			},
		}},
	},
	{Label: "Longer Unicode Send",
		MsgText: "This is a message more than seventy characters with some unicode ☺ in them",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://http1.javna.com/epicenter/GatewaySendG.asp*": {
				httpx.NewMockResponse(200, nil, []byte(`SENT`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Params: url.Values{
				"LoginName":         {"Username"},
				"Password":          {"Password"},
				"Tracking":          {"1"},
				"Mobtyp":            {"1"},
				"MessageRecipients": {"250788383383"},
				"MessageBody":       {"This is a message more than seventy characters with some unicode ☺ in them"},
				"SenderName":        {"2020"},
				"MsgTyp":            {"10"},
			}}},
	},
	{Label: "Long Send",
		MsgText: "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://http1.javna.com/epicenter/GatewaySendG.asp*": {
				httpx.NewMockResponse(200, nil, []byte(`SENT`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Params: url.Values{
				"LoginName":         {"Username"},
				"Password":          {"Password"},
				"Tracking":          {"1"},
				"Mobtyp":            {"1"},
				"MessageRecipients": {"250788383383"},
				"MessageBody":       {"This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work"},
				"SenderName":        {"2020"},
				"MsgTyp":            {"5"},
			}}},
	},
	{Label: "Send Attachment",
		MsgText: "My pic!", MsgURN: "tel:+250788383383", MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"http://http1.javna.com/epicenter/GatewaySendG.asp*": {
				httpx.NewMockResponse(200, nil, []byte(`SENT`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Params: url.Values{
				"LoginName":         {"Username"},
				"Password":          {"Password"},
				"Tracking":          {"1"},
				"Mobtyp":            {"1"},
				"MessageRecipients": {"250788383383"},
				"MessageBody":       {"My pic!\nhttps://foo.bar/image.jpg"},
				"SenderName":        {"2020"},
			}}},
	},
	{Label: "Error Sending",
		MsgText: "Error Sending", MsgURN: "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://http1.javna.com/epicenter/GatewaySendG.asp*": {
				httpx.NewMockResponse(403, nil, []byte(`Error`)),
			},
		},
		ExpectedError: courier.ErrResponseStatus,
	},
	{Label: "Connection Error",
		MsgText: "Error Sending", MsgURN: "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://http1.javna.com/epicenter/GatewaySendG.asp*": {
				httpx.NewMockResponse(500, nil, []byte(`Error`)),
			},
		},
		ExpectedError: courier.ErrConnectionFailed,
	},
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "RR", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			"password": "Password",
			"username": "Username",
		},
	)

	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"Password"}, nil)
}
