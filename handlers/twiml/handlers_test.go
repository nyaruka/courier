package twiml

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"fmt"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/courier/utils/clogs"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "T", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{"auth_token": "6789"}),
}

var tmsTestChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TMS", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{"auth_token": "6789"}),
}

var twTestChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TW", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{"auth_token": "6789"}),
}

var swTestChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SW", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{"auth_token": "6789"}),
}

var (
	receiveURL         = "/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"
	statusURL          = "/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status"
	statusIDURL        = "/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=12345"
	statusInvalidIDURL = "/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=asdf"

	tmsReceiveURL         = "/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"
	tmsStatusURL          = "/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status"
	tmsStatusIDURL        = "/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=12345"
	tmsStatusInvalidIDURL = "/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=asdf"

	twReceiveURL         = "/c/tw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"
	twStatusURL          = "/c/tw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status"
	twStatusIDURL        = "/c/tw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=12345"
	twStatusInvalidIDURL = "/c/tw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=asdf"

	swReceiveURL         = "/c/sw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"
	swStatusURL          = "/c/sw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status"
	swStatusIDURL        = "/c/sw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=12345"
	swStatusInvalidIDURL = "/c/sw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=asdf"

	twaReceiveURL         = "/c/twa/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"
	twaStatusURL          = "/c/twa/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status"
	twaStatusIDURL        = "/c/twa/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=12345"
	twaStatusInvalidIDURL = "/c/twa/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=asdf"

	receiveValid         = "ToCountry=US&ToState=District+Of+Columbia&SmsMessageSid=SMe287d7109a5a925f182f0e07fe5b223b&NumMedia=0&ToCity=&FromZip=01022&SmsSid=SMe287d7109a5a925f182f0e07fe5b223b&FromState=MA&SmsStatus=received&FromCity=CHICOPEE&Body=Msg&FromCountry=US&To=%2B12028831111&ToZip=&NumSegments=1&MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&AccountSid=acctid&From=%2B14133881111&ApiVersion=2010-04-01"
	receiveButtonIgnored = "ToCountry=US&ToState=District+Of+Columbia&SmsMessageSid=SMe287d7109a5a925f182f0e07fe5b223b&NumMedia=0&ToCity=&FromZip=01022&SmsSid=SMe287d7109a5a925f182f0e07fe5b223b&FromState=MA&SmsStatus=received&FromCity=CHICOPEE&Body=Msg&ButtonText=Confirm&FromCountry=US&To=%2B12028831111&ToZip=&NumSegments=1&MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&AccountSid=acctid&From=%2B14133881111&ApiVersion=2010-04-01"
	receiveMedia         = "ToCountry=US&ToState=District+Of+Columbia&SmsMessageSid=SMe287d7109a5a925f182f0e07fe5b223b&NumMedia=2&ToCity=&FromZip=01022&SmsSid=SMe287d7109a5a925f182f0e07fe5b223b&FromState=MA&SmsStatus=received&FromCity=CHICOPEE&FromCountry=US&To=%2B12028831111&ToZip=&NumSegments=1&MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&AccountSid=acctid&From=%2B14133881111&ApiVersion=2010-04-01&MediaUrl0=cat.jpg&MediaUrl1=dog.jpg"
	receiveMediaWithMsg  = "ToCountry=US&ToState=District+Of+Columbia&SmsMessageSid=SMe287d7109a5a925f182f0e07fe5b223b&NumMedia=2&ToCity=&Body=Msg&FromZip=01022&SmsSid=SMe287d7109a5a925f182f0e07fe5b223b&FromState=MA&SmsStatus=received&FromCity=CHICOPEE&FromCountry=US&To=%2B12028831111&ToZip=&NumSegments=1&MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&AccountSid=acctid&From=%2B14133881111&ApiVersion=2010-04-01&MediaUrl0=cat.jpg&MediaUrl1=dog.jpg"
	receiveBase64        = "ToCountry=US&ToState=District+Of+Columbia&SmsMessageSid=SMe287d7109a5a925f182f0e07fe5b223b&NumMedia=0&ToCity=&FromZip=01022&SmsSid=SMe287d7109a5a925f182f0e07fe5b223b&FromState=MA&SmsStatus=received&FromCity=CHICOPEE&Body=QmFubm9uIEV4cGxhaW5zIFRoZSBXb3JsZCAuLi4K4oCcVGhlIENhbXAgb2YgdGhlIFNhaW50c%2BKA&FromCountry=US&To=%2B12028831111&ToZip=&NumSegments=1&MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&AccountSid=acctid&From=%2B14133881111&ApiVersion=2010-04-01"

	statusStop = "ErrorCode=21610&MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&MessageStatus=failed&To=%2B12028831111"

	statusInvalid   = "MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&MessageStatus=huh"
	statusValid     = "MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&MessageStatus=delivered"
	statusRead      = "MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&MessageStatus=read"
	statusRateLimit = "MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&MessageStatus=failed&ErrorCode=63018"

	tmsStatusExtra  = "SmsStatus=sent&MessageStatus=sent&To=2021&MessagingServiceSid=MGdb23ec0f89ee2632e46e91d8128f5e2b&MessageSid=SM0b6e2697aae04182a9f5b5c7a8994c7f&AccountSid=acctid&From=%2B14133881111&ApiVersion=2010-04-01"
	tmsReceiveExtra = "ToCountry=US&ToState=&SmsMessageSid=SMbbf29aeb9d380ce2a1c0ae4635ff9dab&NumMedia=0&ToCity=&FromZip=27609&SmsSid=SMbbf29aeb9d380ce2a1c0ae4635ff9dab&FromState=NC&SmsStatus=received&FromCity=RALEIGH&Body=John+Cruz&FromCountry=US&To=384387&ToZip=&NumSegments=1&MessageSid=SMbbf29aeb9d380ce2a1c0ae4635ff9dab&AccountSid=acctid&From=%2B14133881111&ApiVersion=2010-04-01"

	waReceiveValid         = "ToCountry=US&ToState=District+Of+Columbia&SmsMessageSid=SMe287d7109a5a925f182f0e07fe5b223b&NumMedia=0&ToCity=&FromZip=01022&SmsSid=SMe287d7109a5a925f182f0e07fe5b223b&FromState=MA&SmsStatus=received&FromCity=CHICOPEE&Body=Msg&FromCountry=US&To=whatsapp:%2B12028831111&ToZip=&NumSegments=1&MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&AccountSid=acctid&From=whatsapp:%2B14133881111&ApiVersion=2010-04-01"
	waReceiveButtonValid   = "ToCountry=US&ToState=District+Of+Columbia&SmsMessageSid=SMe287d7109a5a925f182f0e07fe5b223b&NumMedia=0&ToCity=&FromZip=01022&SmsSid=SMe287d7109a5a925f182f0e07fe5b223b&FromState=MA&SmsStatus=received&FromCity=CHICOPEE&Body=Msg&ButtonText=Confirm&FromCountry=US&To=whatsapp:%2B12028831111&ToZip=&NumSegments=1&MessageSid=SMe287d7109a5a925f182f0e07fe5b223b&AccountSid=acctid&From=whatsapp:%2B14133881111&ApiVersion=2010-04-01"
	waReceivePrefixlessURN = "ToCountry=US&ToState=CA&SmsMessageSid=SM681a1f26d9ec591431ce406e8f399525&NumMedia=0&ToCity=&FromZip=60625&SmsSid=SM681a1f26d9ec591431ce406e8f399525&FromState=IL&SmsStatus=received&FromCity=CHICAGO&Body=Msg&FromCountry=US&To=%2B12028831111&ToZip=&NumSegments=1&MessageSid=SM681a1f26d9ec591431ce406e8f399525&AccountSid=acctid&From=%2B14133881111&ApiVersion=2010-04-01"
)

var testCases = []IncomingTestCase{
	{
		Label:                "Receive Valid",
		URL:                  receiveURL,
		Data:                 receiveValid,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "<Response/>",
		ExpectedMsgText:      Sp("Msg"),
		ExpectedURN:          "tel:+14133881111",
		ExpectedExternalID:   "SMe287d7109a5a925f182f0e07fe5b223b",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Button Ignored",
		URL:                  receiveURL,
		Data:                 receiveButtonIgnored,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "<Response/>",
		ExpectedMsgText:      Sp("Msg"),
		ExpectedURN:          "tel:+14133881111",
		ExpectedExternalID:   "SMe287d7109a5a925f182f0e07fe5b223b",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Invalid Signature",
		URL:                  receiveURL,
		Data:                 receiveValid,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "invalid request signature",
		PrepRequest:          addInvalidSignature,
	},
	{
		Label:                "Receive Missing Signature",
		URL:                  receiveURL,
		Data:                 receiveValid,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "missing request signature"},
	{
		Label: "Receive No Params", URL: receiveURL, Data: " ", ExpectedRespStatus: 400, ExpectedBodyContains: "field 'messagesid' required",
		PrepRequest: addValidSignature},
	{Label: "Receive Media", URL: receiveURL, Data: receiveMedia, ExpectedRespStatus: 200, ExpectedBodyContains: "<Response/>",
		ExpectedURN: "tel:+14133881111", ExpectedExternalID: "SMe287d7109a5a925f182f0e07fe5b223b", ExpectedAttachments: []string{"cat.jpg", "dog.jpg"},
		PrepRequest: addValidSignature},
	{Label: "Receive Media With Msg", URL: receiveURL, Data: receiveMediaWithMsg, ExpectedRespStatus: 200, ExpectedBodyContains: "<Response/>",
		ExpectedMsgText: Sp("Msg"), ExpectedURN: "tel:+14133881111", ExpectedExternalID: "SMe287d7109a5a925f182f0e07fe5b223b", ExpectedAttachments: []string{"cat.jpg", "dog.jpg"},
		PrepRequest: addValidSignature},
	{Label: "Receive Base64", URL: receiveURL, Data: receiveBase64, ExpectedRespStatus: 200, ExpectedBodyContains: "<Response/>",
		ExpectedMsgText: Sp("Bannon Explains The World ...\n“The Camp of the Saints"), ExpectedURN: "tel:+14133881111", ExpectedExternalID: "SMe287d7109a5a925f182f0e07fe5b223b",
		PrepRequest: addValidSignature},
	{
		Label:                "Status Stop contact",
		URL:                  statusURL,
		Data:                 statusStop,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"F"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "SMe287d7109a5a925f182f0e07fe5b223b", Status: courier.MsgStatusFailed},
		},
		ExpectedEvents: []ExpectedEvent{
			{Type: courier.EventTypeStopContact, URN: "tel:+12028831111"},
		},
		ExpectedErrors: []*clogs.Error{courier.ErrorExternal("21610", "Attempt to send to unsubscribed recipient")},
		PrepRequest:    addValidSignature,
	},
	{
		Label:                "Status No Params",
		URL:                  statusURL,
		Data:                 " ",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "no msg status, ignoring",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Status Invalid Status",
		URL:                  statusURL,
		Data:                 statusInvalid,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "unknown status 'huh'",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Status Valid",
		URL:                  statusURL,
		Data:                 statusValid,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "SMe287d7109a5a925f182f0e07fe5b223b", Status: courier.MsgStatusDelivered},
		},
		PrepRequest: addValidSignature,
	},
	{
		Label:                "Status Read",
		URL:                  statusURL,
		Data:                 statusRead,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"R"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "SMe287d7109a5a925f182f0e07fe5b223b", Status: courier.MsgStatusRead},
		},
		PrepRequest: addValidSignature,
	},
	{
		Label:                "Status ID Valid",
		URL:                  statusIDURL,
		Data:                 statusValid,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses: []ExpectedStatus{
			{MsgID: 12345, Status: courier.MsgStatusDelivered},
		},
		PrepRequest: addValidSignature,
	},
	{
		Label:                "Status ID Invalid",
		URL:                  statusInvalidIDURL,
		Data:                 statusValid,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "SMe287d7109a5a925f182f0e07fe5b223b", Status: courier.MsgStatusDelivered},
		},
		PrepRequest: addValidSignature,
	},
}

var tmsTestCases = []IncomingTestCase{
	{Label: "Receive Valid", URL: tmsReceiveURL, Data: receiveValid, ExpectedRespStatus: 200, ExpectedBodyContains: "<Response/>",
		ExpectedMsgText: Sp("Msg"), ExpectedURN: "tel:+14133881111", ExpectedExternalID: "SMe287d7109a5a925f182f0e07fe5b223b",
		PrepRequest: addValidSignature},
	{Label: "Receive TMS extra", URL: tmsReceiveURL, Data: tmsReceiveExtra, ExpectedRespStatus: 200, ExpectedBodyContains: "<Response/>",
		ExpectedMsgText: Sp("John Cruz"), ExpectedURN: "tel:+14133881111", ExpectedExternalID: "SMbbf29aeb9d380ce2a1c0ae4635ff9dab",
		PrepRequest: addValidSignature},
	{Label: "Receive Invalid Signature", URL: tmsReceiveURL, Data: receiveValid, ExpectedRespStatus: 400, ExpectedBodyContains: "invalid request signature",
		PrepRequest: addInvalidSignature},
	{Label: "Receive Missing Signature", URL: tmsReceiveURL, Data: receiveValid, ExpectedRespStatus: 400, ExpectedBodyContains: "missing request signature"},
	{Label: "Receive No Params", URL: tmsReceiveURL, Data: " ", ExpectedRespStatus: 400, ExpectedBodyContains: "field 'messagesid' required",
		PrepRequest: addValidSignature},
	{Label: "Receive Media", URL: tmsReceiveURL, Data: receiveMedia, ExpectedRespStatus: 200, ExpectedBodyContains: "<Response/>",
		ExpectedURN: "tel:+14133881111", ExpectedExternalID: "SMe287d7109a5a925f182f0e07fe5b223b", ExpectedAttachments: []string{"cat.jpg", "dog.jpg"},
		PrepRequest: addValidSignature},
	{Label: "Receive Media With Msg", URL: tmsReceiveURL, Data: receiveMediaWithMsg, ExpectedRespStatus: 200, ExpectedBodyContains: "<Response/>",
		ExpectedMsgText: Sp("Msg"), ExpectedURN: "tel:+14133881111", ExpectedExternalID: "SMe287d7109a5a925f182f0e07fe5b223b", ExpectedAttachments: []string{"cat.jpg", "dog.jpg"},
		PrepRequest: addValidSignature},
	{Label: "Receive Base64", URL: tmsReceiveURL, Data: receiveBase64, ExpectedRespStatus: 200, ExpectedBodyContains: "<Response/>",
		ExpectedMsgText: Sp("Bannon Explains The World ...\n“The Camp of the Saints"), ExpectedURN: "tel:+14133881111", ExpectedExternalID: "SMe287d7109a5a925f182f0e07fe5b223b",
		PrepRequest: addValidSignature},
	{
		Label:                "Status Stop contact",
		URL:                  tmsStatusURL,
		Data:                 statusStop,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"F"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "SMe287d7109a5a925f182f0e07fe5b223b", Status: courier.MsgStatusFailed},
		},
		ExpectedEvents: []ExpectedEvent{
			{Type: courier.EventTypeStopContact, URN: "tel:+12028831111"},
		},
		ExpectedErrors: []*clogs.Error{courier.ErrorExternal("21610", "Attempt to send to unsubscribed recipient")},
		PrepRequest:    addValidSignature,
	},
	{
		Label:                "Status TMS extra",
		URL:                  tmsStatusURL,
		Data:                 tmsStatusExtra,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"S"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "SM0b6e2697aae04182a9f5b5c7a8994c7f", Status: courier.MsgStatusSent},
		},
		PrepRequest: addValidSignature,
	},
	{
		Label:                "Status No Params",
		URL:                  tmsStatusURL,
		Data:                 " ",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "no msg status, ignoring",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Status Invalid Status",
		URL:                  tmsStatusURL,
		Data:                 statusInvalid,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "unknown status 'huh'",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Status Valid",
		URL:                  tmsStatusURL,
		Data:                 statusValid,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "SMe287d7109a5a925f182f0e07fe5b223b", Status: courier.MsgStatusDelivered},
		},
		PrepRequest: addValidSignature,
	},
	{
		Label:                "Status ID Valid",
		URL:                  tmsStatusIDURL,
		Data:                 statusValid,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses: []ExpectedStatus{
			{MsgID: 12345, Status: courier.MsgStatusDelivered},
		},
		PrepRequest: addValidSignature,
	},
	{
		Label:                "Status ID Invalid",
		URL:                  tmsStatusInvalidIDURL,
		Data:                 statusValid,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "SMe287d7109a5a925f182f0e07fe5b223b", Status: courier.MsgStatusDelivered},
		},
		PrepRequest: addValidSignature,
	},
}

var twTestCases = []IncomingTestCase{
	{Label: "Receive Valid", URL: twReceiveURL, Data: receiveValid, ExpectedRespStatus: 200, ExpectedBodyContains: "<Response/>",
		ExpectedMsgText: Sp("Msg"), ExpectedURN: "tel:+14133881111", ExpectedExternalID: "SMe287d7109a5a925f182f0e07fe5b223b",
		PrepRequest: addValidSignature},
	{Label: "Receive Forwarded Valid", URL: twReceiveURL, Data: receiveValid,
		Headers:            map[string]string{forwardedPathHeader: "/handlers/twilio/receive/8eb23e93-5ecb-45ba-b726-3b064e0c56ab"},
		ExpectedRespStatus: 200, ExpectedBodyContains: "<Response/>",
		ExpectedMsgText: Sp("Msg"), ExpectedURN: "tel:+14133881111", ExpectedExternalID: "SMe287d7109a5a925f182f0e07fe5b223b",
		PrepRequest: addForwardSignature},
	{Label: "Receive Invalid Signature", URL: twReceiveURL, Data: receiveValid, ExpectedRespStatus: 400, ExpectedBodyContains: "invalid request signature",
		PrepRequest: addInvalidSignature},
	{Label: "Receive Missing Signature", URL: twReceiveURL, Data: receiveValid, ExpectedRespStatus: 400, ExpectedBodyContains: "missing request signature"},
	{Label: "Receive No Params", URL: twReceiveURL, Data: " ", ExpectedRespStatus: 400, ExpectedBodyContains: "field 'messagesid' required",
		PrepRequest: addValidSignature},
	{Label: "Receive Media", URL: twReceiveURL, Data: receiveMedia, ExpectedRespStatus: 200, ExpectedBodyContains: "<Response/>",
		ExpectedURN: "tel:+14133881111", ExpectedExternalID: "SMe287d7109a5a925f182f0e07fe5b223b", ExpectedAttachments: []string{"cat.jpg", "dog.jpg"},
		PrepRequest: addValidSignature},
	{Label: "Receive Media With Msg", URL: twReceiveURL, Data: receiveMediaWithMsg, ExpectedRespStatus: 200, ExpectedBodyContains: "<Response/>",
		ExpectedMsgText: Sp("Msg"), ExpectedURN: "tel:+14133881111", ExpectedExternalID: "SMe287d7109a5a925f182f0e07fe5b223b", ExpectedAttachments: []string{"cat.jpg", "dog.jpg"},
		PrepRequest: addValidSignature},
	{Label: "Receive Base64", URL: twReceiveURL, Data: receiveBase64, ExpectedRespStatus: 200, ExpectedBodyContains: "<Response/>",
		ExpectedMsgText: Sp("Bannon Explains The World ...\n“The Camp of the Saints"), ExpectedURN: "tel:+14133881111", ExpectedExternalID: "SMe287d7109a5a925f182f0e07fe5b223b",
		PrepRequest: addValidSignature},
	{
		Label:                "Status Stop contact",
		URL:                  twStatusURL,
		Data:                 statusStop,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"F"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "SMe287d7109a5a925f182f0e07fe5b223b", Status: courier.MsgStatusFailed},
		},
		ExpectedEvents: []ExpectedEvent{
			{Type: courier.EventTypeStopContact, URN: "tel:+12028831111"},
		},
		ExpectedErrors: []*clogs.Error{courier.ErrorExternal("21610", "Attempt to send to unsubscribed recipient")},
		PrepRequest:    addValidSignature,
	},
	{Label: "Status No Params", URL: twStatusURL, Data: " ", ExpectedRespStatus: 200, ExpectedBodyContains: "no msg status, ignoring",
		PrepRequest: addValidSignature},
	{Label: "Status Invalid Status", URL: twStatusURL, Data: statusInvalid, ExpectedRespStatus: 400, ExpectedBodyContains: "unknown status 'huh'",
		PrepRequest: addValidSignature},
	{
		Label:                "Status Valid",
		URL:                  twStatusURL,
		Data:                 statusValid,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "SMe287d7109a5a925f182f0e07fe5b223b", Status: courier.MsgStatusDelivered},
		},
		PrepRequest: addValidSignature,
	},
	{
		Label:                "Status ID Valid",
		URL:                  twStatusIDURL,
		Data:                 statusValid,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses: []ExpectedStatus{
			{MsgID: 12345, Status: courier.MsgStatusDelivered},
		},
		PrepRequest: addValidSignature,
	},
	{
		Label:                "Status ID Invalid",
		URL:                  twStatusInvalidIDURL,
		Data:                 statusValid,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "SMe287d7109a5a925f182f0e07fe5b223b", Status: courier.MsgStatusDelivered},
		},
		PrepRequest: addValidSignature,
	},
}

var swTestCases = []IncomingTestCase{
	{Label: "Receive Valid", URL: swReceiveURL, Data: receiveValid, ExpectedRespStatus: 200, ExpectedBodyContains: "<Response/>",
		ExpectedMsgText: Sp("Msg"), ExpectedURN: "tel:+14133881111", ExpectedExternalID: "SMe287d7109a5a925f182f0e07fe5b223b"},
	{Label: "Receive No Params", URL: swReceiveURL, Data: " ", ExpectedRespStatus: 400, ExpectedBodyContains: "field 'messagesid' required"},
	{Label: "Receive Media", URL: swReceiveURL, Data: receiveMedia, ExpectedRespStatus: 200, ExpectedBodyContains: "<Response/>",
		ExpectedURN: "tel:+14133881111", ExpectedExternalID: "SMe287d7109a5a925f182f0e07fe5b223b", ExpectedAttachments: []string{"cat.jpg", "dog.jpg"}},
	{Label: "Receive Media With Msg", URL: swReceiveURL, Data: receiveMediaWithMsg, ExpectedRespStatus: 200, ExpectedBodyContains: "<Response/>",
		ExpectedMsgText: Sp("Msg"), ExpectedURN: "tel:+14133881111", ExpectedExternalID: "SMe287d7109a5a925f182f0e07fe5b223b", ExpectedAttachments: []string{"cat.jpg", "dog.jpg"}},
	{Label: "Receive Base64", URL: swReceiveURL, Data: receiveBase64, ExpectedRespStatus: 200, ExpectedBodyContains: "<Response/>",
		ExpectedMsgText: Sp("Bannon Explains The World ...\n“The Camp of the Saints"), ExpectedURN: "tel:+14133881111", ExpectedExternalID: "SMe287d7109a5a925f182f0e07fe5b223b"},
	{
		Label:                "Status Stop contact",
		URL:                  swStatusURL,
		Data:                 statusStop,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"F"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "SMe287d7109a5a925f182f0e07fe5b223b", Status: courier.MsgStatusFailed},
		},
		ExpectedEvents: []ExpectedEvent{
			{Type: courier.EventTypeStopContact, URN: "tel:+12028831111"},
		},
		ExpectedErrors: []*clogs.Error{courier.ErrorExternal("21610", "Attempt to send to unsubscribed recipient")},
		PrepRequest:    addValidSignature,
	},
	{Label: "Status No Params", URL: swStatusURL, Data: " ", ExpectedRespStatus: 200, ExpectedBodyContains: "no msg status, ignoring"},
	{Label: "Status Invalid Status", URL: swStatusURL, Data: statusInvalid, ExpectedRespStatus: 400, ExpectedBodyContains: "unknown status 'huh'"},
	{
		Label:                "Status Valid",
		URL:                  swStatusURL,
		Data:                 statusValid,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "SMe287d7109a5a925f182f0e07fe5b223b", Status: courier.MsgStatusDelivered},
		},
		PrepRequest: addValidSignature,
	},
	{
		Label:                "Status ID Valid",
		URL:                  swStatusIDURL,
		Data:                 statusValid,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses: []ExpectedStatus{
			{MsgID: 12345, Status: courier.MsgStatusDelivered},
		},
	},
	{
		Label:                "Status ID Invalid",
		URL:                  swStatusInvalidIDURL,
		Data:                 statusValid,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "SMe287d7109a5a925f182f0e07fe5b223b", Status: courier.MsgStatusDelivered},
		},
	},
}

var waTestCases = []IncomingTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: waReceiveValid, ExpectedRespStatus: 200, ExpectedBodyContains: "<Response/>",
		ExpectedMsgText: Sp("Msg"), ExpectedURN: "whatsapp:14133881111", ExpectedExternalID: "SMe287d7109a5a925f182f0e07fe5b223b",
		PrepRequest: addValidSignature},
}

var twaTestCases = []IncomingTestCase{
	{Label: "Receive Valid", URL: twaReceiveURL, Data: waReceiveValid, ExpectedRespStatus: 200, ExpectedBodyContains: "<Response/>",
		ExpectedMsgText: Sp("Msg"), ExpectedURN: "whatsapp:14133881111", ExpectedExternalID: "SMe287d7109a5a925f182f0e07fe5b223b",
		PrepRequest: addValidSignature},
	{Label: "Receive Valid", URL: twaReceiveURL, Data: waReceiveButtonValid, ExpectedRespStatus: 200, ExpectedBodyContains: "<Response/>",
		ExpectedMsgText: Sp("Confirm"), ExpectedURN: "whatsapp:14133881111", ExpectedExternalID: "SMe287d7109a5a925f182f0e07fe5b223b",
		PrepRequest: addValidSignature},
	{Label: "Receive Prefixless URN", URL: twaReceiveURL, Data: waReceivePrefixlessURN, ExpectedRespStatus: 200, ExpectedBodyContains: "<Response/>",
		ExpectedMsgText: Sp("Msg"), ExpectedURN: "whatsapp:14133881111", ExpectedExternalID: "SM681a1f26d9ec591431ce406e8f399525",
		PrepRequest: addValidSignature},
	{Label: "Status No Params", URL: twaStatusURL, Data: " ", ExpectedRespStatus: 200, ExpectedBodyContains: "no msg status, ignoring",
		PrepRequest: addValidSignature},
	{Label: "Status Invalid Status", URL: twaStatusURL, Data: statusInvalid, ExpectedRespStatus: 400, ExpectedBodyContains: "unknown status 'huh'",
		PrepRequest: addValidSignature},
	{
		Label:                "Status Valid",
		URL:                  twaStatusURL,
		Data:                 statusValid,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "SMe287d7109a5a925f182f0e07fe5b223b", Status: courier.MsgStatusDelivered},
		},
		PrepRequest: addValidSignature,
	},
	{
		Label:                "Status ID Valid",
		URL:                  twaStatusIDURL,
		Data:                 statusValid,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses: []ExpectedStatus{
			{MsgID: 12345, Status: courier.MsgStatusDelivered},
		},
		PrepRequest: addValidSignature,
	},
	{
		Label:                "Status ID Rate limit",
		URL:                  twaStatusIDURL,
		Data:                 statusRateLimit,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"E"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "SMe287d7109a5a925f182f0e07fe5b223b", Status: courier.MsgStatusErrored},
		},
		PrepRequest:    addValidSignature,
		ExpectedErrors: []*clogs.Error{courier.ErrorExternal("63018", "Rate limit exceeded for Channel")},
	},
	{
		Label:                "Status ID Invalid",
		URL:                  twaStatusInvalidIDURL,
		Data:                 statusValid,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "SMe287d7109a5a925f182f0e07fe5b223b", Status: courier.MsgStatusDelivered},
		},
		PrepRequest: addValidSignature,
	},
}

func addValidSignature(r *http.Request) {
	r.ParseForm()
	sig, _ := twCalculateSignature(fmt.Sprintf("%s://%s%s", r.URL.Scheme, r.Host, r.URL.RequestURI()), r.PostForm, "6789")
	r.Header.Set(signatureHeader, string(sig))
}

func addForwardSignature(r *http.Request) {
	r.ParseForm()
	path := r.Header.Get(forwardedPathHeader)
	sig, _ := twCalculateSignature(fmt.Sprintf("%s://%s%s", r.URL.Scheme, r.Host, path), r.PostForm, "6789")
	r.Header.Set(signatureHeader, string(sig))
}

func addInvalidSignature(r *http.Request) {
	r.Header.Set(signatureHeader, "invalidsig")
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newTWIMLHandler("T", "Twilio", true), testCases)
	RunIncomingTestCases(t, tmsTestChannels, newTWIMLHandler("TMS", "Twilio Messaging Service", true), tmsTestCases)
	RunIncomingTestCases(t, twTestChannels, newTWIMLHandler("TW", "TwiML API", true), twTestCases)
	RunIncomingTestCases(t, swTestChannels, newTWIMLHandler("SW", "SignalWire", false), swTestCases)

	waChannel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SW", "+12065551212", "US",
		[]string{urns.WhatsApp.Prefix},
		map[string]any{
			configAccountSID:        "accountSID",
			courier.ConfigAuthToken: "6789",
		},
	)
	RunIncomingTestCases(t, []courier.Channel{waChannel}, newTWIMLHandler("T", "TwilioWhatsApp", true), waTestCases)

	twaChannel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TWA", "+12065551212", "US",
		[]string{urns.WhatsApp.Prefix},
		map[string]any{
			configAccountSID:        "accountSID",
			courier.ConfigAuthToken: "6789",
		},
	)
	RunIncomingTestCases(t, []courier.Channel{twaChannel}, newTWIMLHandler("TWA", "Twilio WhatsApp", true), twaTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newTWIMLHandler("T", "Twilio", true), testCases)
	RunChannelBenchmarks(b, tmsTestChannels, newTWIMLHandler("TMS", "Twilio Messaging Service", true), tmsTestCases)
	RunChannelBenchmarks(b, twTestChannels, newTWIMLHandler("TW", "TwiML API", true), twTestCases)
}

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message ☺",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twilio.com/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(200, nil, []byte(`{ "sid": "1002" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
				Path:    "/2010-04-01/Accounts/accountSID/Messages.json",
				Form:    url.Values{"Body": {"Simple Message ☺"}, "To": {"+250788383383"}, "From": {"2020"}, "StatusCallback": {"https://localhost/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}},
			},
		},
		ExpectedExtIDs: []string{"1002"},
	},
	{
		Label:   "Long Send",
		MsgText: "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twilio.com/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(200, nil, []byte(`{ "sid": "1002" }`)),
				httpx.NewMockResponse(200, nil, []byte(`{ "sid": "1002" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
				Path:    "/2010-04-01/Accounts/accountSID/Messages.json",
				Form:    url.Values{"Body": {"This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say,"}, "To": {"+250788383383"}, "From": {"2020"}, "StatusCallback": {"https://localhost/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}},
			},
			{
				Headers: map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
				Path:    "/2010-04-01/Accounts/accountSID/Messages.json",
				Form:    url.Values{"Body": {"I need to keep adding more things to make it work"}, "To": {"+250788383383"}, "From": {"2020"}, "StatusCallback": {"https://localhost/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}},
			},
		},
		ExpectedExtIDs: []string{"1002", "1002"},
	},
	{
		Label:   "Error Sending",
		MsgText: "Error Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twilio.com/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(401, nil, []byte(`{ "error": "out of credits" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form: url.Values{"Body": {"Error Message"}, "To": {"+250788383383"}, "From": {"2020"}, "StatusCallback": {"https://localhost/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}},
		}},
		ExpectedError: courier.ErrResponseStatus,
	},
	{
		Label:   "Error Code",
		MsgText: "Error Code",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twilio.com/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(400, nil, []byte(`{ "code": 1001 }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form: url.Values{"Body": {"Error Code"}, "To": {"+250788383383"}, "From": {"2020"}, "StatusCallback": {"https://localhost/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}},
		}},
		ExpectedError: courier.ErrFailedWithReason("1001", "Service specific error: 1001."),
	},
	{
		Label:   "Stopped Contact Code",
		MsgText: "Stopped Contact",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twilio.com/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(400, nil, []byte(`{ "code": 21610 }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form: url.Values{"Body": {"Stopped Contact"}, "To": {"+250788383383"}, "From": {"2020"}, "StatusCallback": {"https://localhost/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}},
		}},
		ExpectedError: courier.ErrContactStopped,
	},
	{
		Label:   "No SID",
		MsgText: "No SID",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twilio.com/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(200, nil, []byte(`{ }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form: url.Values{"Body": {"No SID"}, "To": {"+250788383383"}, "From": {"2020"}, "StatusCallback": {"https://localhost/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}},
		}},
		ExpectedLogErrors: []*clogs.Error{courier.ErrorResponseValueMissing("sid")},
	},
	{
		Label:          "Single attachment and text",
		MsgText:        "My pic!",
		MsgURN:         "tel:+250788383383",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twilio.com/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(200, nil, []byte(`{ "sid": "1002" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Form: url.Values{
					"Body":           []string{"My pic!"},
					"To":             []string{"+250788383383"},
					"MediaUrl":       []string{"https://foo.bar/image.jpg"},
					"From":           []string{"2020"},
					"StatusCallback": []string{"https://localhost/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"},
				},
			},
		},
		ExpectedExtIDs: []string{"1002"},
	},
	{
		Label:          "Multiple attachments, no text",
		MsgURN:         "tel:+250788383383",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg", "audio/mp4:https://foo.bar/audio.m4a"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twilio.com/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(200, nil, []byte(`{ "sid": "1002" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Form: url.Values{
					"Body":           []string{""},
					"To":             []string{"+250788383383"},
					"MediaUrl":       []string{"https://foo.bar/image.jpg", "https://foo.bar/audio.m4a"},
					"From":           []string{"2020"},
					"StatusCallback": []string{"https://localhost/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"},
				},
			},
		},
		ExpectedExtIDs: []string{"1002"},
	},
}

var tmsDefaultSendTestCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message ☺",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twilio.com/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(200, nil, []byte(`{ "sid": "1002" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
				Path:    "/2010-04-01/Accounts/accountSID/Messages.json",
				Form:    url.Values{"Body": {"Simple Message ☺"}, "To": {"+250788383383"}, "MessagingServiceSid": {"messageServiceSID"}, "StatusCallback": {"https://localhost/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56cd/status?id=10&action=callback"}},
			},
		},
		ExpectedExtIDs: []string{"1002"},
	},
	{
		Label:   "Long Send",
		MsgText: "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twilio.com/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(200, nil, []byte(`{ "sid": "1002" }`)),
				httpx.NewMockResponse(200, nil, []byte(`{ "sid": "1002" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
				Path:    "/2010-04-01/Accounts/accountSID/Messages.json",
				Form:    url.Values{"Body": {"This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say,"}, "To": {"+250788383383"}, "MessagingServiceSid": {"messageServiceSID"}, "StatusCallback": {"https://localhost/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56cd/status?id=10&action=callback"}},
			},
			{
				Headers: map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
				Path:    "/2010-04-01/Accounts/accountSID/Messages.json",
				Form:    url.Values{"Body": {"I need to keep adding more things to make it work"}, "To": {"+250788383383"}, "MessagingServiceSid": {"messageServiceSID"}, "StatusCallback": {"https://localhost/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56cd/status?id=10&action=callback"}},
			},
		},
		ExpectedExtIDs: []string{"1002", "1002"},
	},
	{
		Label:   "Error Sending",
		MsgText: "Error Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twilio.com/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(401, nil, []byte(`{ "error": "out of credits" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
				Path:    "/2010-04-01/Accounts/accountSID/Messages.json",
				Form:    url.Values{"Body": {"Error Message"}, "To": {"+250788383383"}, "MessagingServiceSid": {"messageServiceSID"}, "StatusCallback": {"https://localhost/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56cd/status?id=10&action=callback"}},
			},
		},
		ExpectedError: courier.ErrResponseStatus,
	},
	{
		Label:   "Error Code",
		MsgText: "Error Code",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twilio.com/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(400, nil, []byte(`{ "code": 1001 }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form: url.Values{"Body": {"Error Code"}, "To": {"+250788383383"}, "MessagingServiceSid": {"messageServiceSID"}, "StatusCallback": {"https://localhost/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56cd/status?id=10&action=callback"}},
		}},
		ExpectedError: courier.ErrFailedWithReason("1001", "Service specific error: 1001."),
	},
	{
		Label:   "Stopped Contact Code",
		MsgText: "Stopped Contact",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twilio.com/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(400, nil, []byte(`{ "code": 21610 }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form: url.Values{"Body": {"Stopped Contact"}, "To": {"+250788383383"}, "MessagingServiceSid": {"messageServiceSID"}, "StatusCallback": {"https://localhost/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56cd/status?id=10&action=callback"}},
		}},
		ExpectedError: courier.ErrContactStopped,
	},
	{
		Label:   "No SID",
		MsgText: "No SID",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twilio.com/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(200, nil, []byte(`{ }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form: url.Values{"Body": {"No SID"}, "To": {"+250788383383"}, "MessagingServiceSid": {"messageServiceSID"}, "StatusCallback": {"https://localhost/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56cd/status?id=10&action=callback"}},
		}},
		ExpectedLogErrors: []*clogs.Error{courier.ErrorResponseValueMissing("sid")},
	},
	{
		Label:          "Send Attachment",
		MsgText:        "My pic!",
		MsgURN:         "tel:+250788383383",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twilio.com/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(200, nil, []byte(`{ "sid": "1002" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form: url.Values{"Body": {"My pic!"}, "To": {"+250788383383"}, "MediaUrl": {"https://foo.bar/image.jpg"}, "MessagingServiceSid": {"messageServiceSID"}, "StatusCallback": {"https://localhost/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56cd/status?id=10&action=callback"}},
		}},
		ExpectedExtIDs: []string{"1002"},
	},
}

var tmsShortenLinks = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message ☺",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twilio.com/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(200, nil, []byte(`{ "sid": "1002" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
				Path:    "/2010-04-01/Accounts/accountSID/Messages.json",
				Form:    url.Values{"Body": {"Simple Message ☺"}, "To": {"+250788383383"}, "MessagingServiceSid": {"messageServiceSID"}, "ShortenUrls": {"true"}, "StatusCallback": {"https://localhost/c/tms/8eb23e93-5ecb-45ba-b726-3b064e0c56cd/status?id=10&action=callback"}},
			},
		},
		ExpectedExtIDs: []string{"1002"},
	},
}

var twDefaultSendTestCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message ☺",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/twiml_api/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(200, nil, []byte(`{ "sid": "1002" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
				Path:    "/twiml_api/2010-04-01/Accounts/accountSID/Messages.json",
				Form:    url.Values{"Body": {"Simple Message ☺"}, "To": {"+250788383383"}, "From": {"2020"}, "StatusCallback": {"https://localhost/c/tw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}},
			},
		},
		ExpectedExtIDs: []string{"1002"},
	},
	{
		Label:   "Long Send",
		MsgText: "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/twiml_api/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(200, nil, []byte(`{ "sid": "1002" }`)),
				httpx.NewMockResponse(200, nil, []byte(`{ "sid": "1002" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
				Path:    "/twiml_api/2010-04-01/Accounts/accountSID/Messages.json",
				Form:    url.Values{"Body": {"This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say,"}, "To": {"+250788383383"}, "From": {"2020"}, "StatusCallback": {"https://localhost/c/tw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}},
			},
			{
				Headers: map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
				Path:    "/twiml_api/2010-04-01/Accounts/accountSID/Messages.json",
				Form:    url.Values{"Body": {"I need to keep adding more things to make it work"}, "To": {"+250788383383"}, "From": {"2020"}, "StatusCallback": {"https://localhost/c/tw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}},
			},
		},
		ExpectedExtIDs: []string{"1002", "1002"},
	},
	{
		Label:   "Error Sending",
		MsgText: "Error Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/twiml_api/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(401, nil, []byte(`{ "error": "out of credits" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form: url.Values{"Body": {"Error Message"}, "To": {"+250788383383"}, "From": {"2020"}, "StatusCallback": {"https://localhost/c/tw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}},
		}},
		ExpectedError: courier.ErrResponseStatus,
	},
	{
		Label:   "Error Code",
		MsgText: "Error Code",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/twiml_api/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(400, nil, []byte(`{ "code": 1001 }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form: url.Values{"Body": {"Error Code"}, "To": {"+250788383383"}, "From": {"2020"}, "StatusCallback": {"https://localhost/c/tw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}},
		}},
		ExpectedError: courier.ErrFailedWithReason("1001", "Service specific error: 1001."),
	},
	{
		Label:   "Stopped Contact Code",
		MsgText: "Stopped Contact",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/twiml_api/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(400, nil, []byte(`{ "code": 21610 }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form: url.Values{"Body": {"Stopped Contact"}, "To": {"+250788383383"}, "From": {"2020"}, "StatusCallback": {"https://localhost/c/tw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}},
		}},
		ExpectedError: courier.ErrContactStopped,
	},
	{
		Label:   "No SID",
		MsgText: "No SID",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/twiml_api/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(200, nil, []byte(`{ }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form: url.Values{"Body": {"No SID"}, "To": {"+250788383383"}, "From": {"2020"}, "StatusCallback": {"https://localhost/c/tw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}},
		}},
		ExpectedLogErrors: []*clogs.Error{courier.ErrorResponseValueMissing("sid")},
	},
	{
		Label:          "Send Attachment",
		MsgText:        "My pic!",
		MsgURN:         "tel:+250788383383",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/twiml_api/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(200, nil, []byte(`{ "sid": "1002" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form: url.Values{"Body": {"My pic!"}, "To": {"+250788383383"}, "MediaUrl": {"https://foo.bar/image.jpg"}, "From": {"2020"}, "StatusCallback": {"https://localhost/c/tw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}},
		}},
		ExpectedExtIDs: []string{"1002"},
	},
}

var swSendTestCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message ☺",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/sigware_api/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(200, nil, []byte(`{ "sid": "1002" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
				Path:    "/sigware_api/2010-04-01/Accounts/accountSID/Messages.json",
				Form:    url.Values{"Body": {"Simple Message ☺"}, "To": {"+250788383383"}, "From": {"2020"}, "StatusCallback": {"https://localhost/c/sw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}},
			},
		},
		ExpectedExtIDs: []string{"1002"},
	},
	{
		Label:   "Long Send",
		MsgText: "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/sigware_api/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(200, nil, []byte(`{ "sid": "1002" }`)),
				httpx.NewMockResponse(200, nil, []byte(`{ "sid": "1002" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
				Path:    "/sigware_api/2010-04-01/Accounts/accountSID/Messages.json",
				Form:    url.Values{"Body": {"This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say,"}, "To": {"+250788383383"}, "From": {"2020"}, "StatusCallback": {"https://localhost/c/sw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}},
			},
			{
				Headers: map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
				Path:    "/sigware_api/2010-04-01/Accounts/accountSID/Messages.json",
				Form:    url.Values{"Body": {"I need to keep adding more things to make it work"}, "To": {"+250788383383"}, "From": {"2020"}, "StatusCallback": {"https://localhost/c/sw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}},
			},
		},
		ExpectedExtIDs: []string{"1002", "1002"},
	},
	{
		Label:   "Error Sending",
		MsgText: "Error Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/sigware_api/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(401, nil, []byte(`{ "error": "out of credits" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form: url.Values{"Body": {"Error Message"}, "To": {"+250788383383"}, "From": {"2020"}, "StatusCallback": {"https://localhost/c/sw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}},
		}},
		ExpectedError: courier.ErrResponseStatus,
	},
	{
		Label:   "Error Code",
		MsgText: "Error Code",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/sigware_api/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(400, nil, []byte(`{ "code": 1001 }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form: url.Values{"Body": {"Error Code"}, "To": {"+250788383383"}, "From": {"2020"}, "StatusCallback": {"https://localhost/c/sw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}},
		}},
		ExpectedError: courier.ErrFailedWithReason("1001", "Service specific error: 1001."),
	},
	{
		Label:   "Stopped Contact Code",
		MsgText: "Stopped Contact",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/sigware_api/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(400, nil, []byte(`{ "code": 21610 }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form: url.Values{"Body": {"Stopped Contact"}, "To": {"+250788383383"}, "From": {"2020"}, "StatusCallback": {"https://localhost/c/sw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}},
		}},
		ExpectedError: courier.ErrContactStopped,
	},
	{
		Label:   "No SID",
		MsgText: "No SID",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/sigware_api/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(200, nil, []byte(`{ }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form: url.Values{"Body": {"No SID"}, "To": {"+250788383383"}, "From": {"2020"}, "StatusCallback": {"https://localhost/c/sw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}},
		}},
		ExpectedLogErrors: []*clogs.Error{courier.ErrorResponseValueMissing("sid")},
	},
	{
		Label:          "Send Attachment",
		MsgText:        "My pic!",
		MsgURN:         "tel:+250788383383",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/sigware_api/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(200, nil, []byte(`{ "sid": "1002" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form: url.Values{"Body": {"My pic!"}, "To": {"+250788383383"}, "MediaUrl": {"https://foo.bar/image.jpg"}, "From": {"2020"}, "StatusCallback": {"https://localhost/c/sw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}},
		}},
		ExpectedExtIDs: []string{"1002"},
	},
}

var waSendTestCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message ☺",
		MsgURN:  "whatsapp:250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/sigware_api/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(200, nil, []byte(`{ "sid": "1002" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form:    url.Values{"Body": {"Simple Message ☺"}, "To": {"whatsapp:+250788383383"}, "From": {"whatsapp:+12065551212"}, "StatusCallback": {"https://localhost/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}},
			Headers: map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
		}},
		ExpectedExtIDs: []string{"1002"},
	},
	{
		Label:     "Template Send",
		MsgText:   "templated message",
		MsgURN:    "whatsapp:250788383383",
		MsgLocale: "eng",
		MsgTemplating: `{
			"template": {"uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3", "name": "revive_issue"},
			"components": [
				{"type": "body", "name": "body", "variables": {"1": 0, "2": 1}}
			],
			"variables": [
				{"type": "text", "value": "Chef"},
				{"type": "text" , "value": "tomorrow"}
			],
			"external_id": "ext_id_revive_issue",
			"language": "en_US"
		}`,
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/sigware_api/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(200, nil, []byte(`{ "sid": "1002" }`)),
			},
		},

		ExpectedRequests: []ExpectedRequest{{
			Form:    url.Values{"To": {"whatsapp:+250788383383"}, "From": {"whatsapp:+12065551212"}, "StatusCallback": {"https://localhost/c/t/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}, "ContentSid": {"ext_id_revive_issue"}, "ContentVariables": {"{\"1\":\"Chef\",\"2\":\"tomorrow\"}"}},
			Headers: map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
		}},
		ExpectedExtIDs: []string{"1002"},
	},
}

var twaSendTestCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message ☺",
		MsgURN:  "whatsapp:250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twilio.com/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(200, nil, []byte(`{ "sid": "1002" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form:    url.Values{"Body": {"Simple Message ☺"}, "To": {"whatsapp:+250788383383"}, "From": {"whatsapp:+12065551212"}, "MessagingServiceSid": {"messageServiceSID"}, "StatusCallback": {"https://localhost/c/twa/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}},
			Headers: map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
		}},
		ExpectedExtIDs: []string{"1002"},
	},
	{
		Label:     "Template Send",
		MsgText:   "templated message",
		MsgURN:    "whatsapp:250788383383",
		MsgLocale: "eng",
		MsgTemplating: `{
			"template": {"uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3", "name": "revive_issue"},
			"components": [
				{"type": "body", "name": "body", "variables": {"1": 0, "2": 1}}
			],
			"variables": [
				{"type": "text", "value": "Chef"},
				{"type": "text" , "value": "tomorrow"}
			],
			"external_id": "ext_id_revive_issue",
			"language": "en_US"
		}`,
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twilio.com/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(200, nil, []byte(`{ "sid": "1002" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form:    url.Values{"To": {"whatsapp:+250788383383"}, "From": {"whatsapp:+12065551212"}, "MessagingServiceSid": {"messageServiceSID"}, "StatusCallback": {"https://localhost/c/twa/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}, "ContentSid": {"ext_id_revive_issue"}, "ContentVariables": {"{\"1\":\"Chef\",\"2\":\"tomorrow\"}"}},
			Headers: map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
		}},
		ExpectedExtIDs: []string{"1002"},
	},
	{
		Label:     "Template Send no attachment",
		MsgText:   "templated message",
		MsgURN:    "whatsapp:250788383383",
		MsgLocale: "eng",
		MsgTemplating: `{
			"template": {"uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3", "name": "revive_issue"},
			"components": [
				{"type": "body", "name": "body", "variables": {"1": 0, "2": 1}}
			],
			"variables": [
				{"type": "text", "value": "Chef: common resto"},
				{"type": "text" , "value": "tomorrow"}
			],
			"external_id": "ext_id_revive_issue",
			"language": "en_US"
		}`,
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twilio.com/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(200, nil, []byte(`{ "sid": "1002" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form:    url.Values{"To": {"whatsapp:+250788383383"}, "From": {"whatsapp:+12065551212"}, "MessagingServiceSid": {"messageServiceSID"}, "StatusCallback": {"https://localhost/c/twa/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}, "ContentSid": {"ext_id_revive_issue"}, "ContentVariables": {"{\"1\":\"Chef: common resto\",\"2\":\"tomorrow\"}"}},
			Headers: map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
		}},
		ExpectedExtIDs: []string{"1002"},
	},
	{
		Label:     "Template Send with image",
		MsgText:   "templated message",
		MsgURN:    "whatsapp:250788383383",
		MsgLocale: "eng",
		MsgTemplating: `{
			"template": {"uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3", "name": "revive_issue"},
			"components": [
				{"type": "header", "name": "header", "variables": {"1": 0, "2": 1}}
			],
			"variables": [
				{"type": "image", "value": "image/jpeg:http://example.com/cat2.jpg"},
				{"type": "text" , "value": "tomorrow"}
			],
			"external_id": "ext_id_revive_issue",
			"language": "en_US"
		}`,
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twilio.com/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(200, nil, []byte(`{ "sid": "1002" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form:    url.Values{"To": {"whatsapp:+250788383383"}, "From": {"whatsapp:+12065551212"}, "MessagingServiceSid": {"messageServiceSID"}, "StatusCallback": {"https://localhost/c/twa/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}, "ContentSid": {"ext_id_revive_issue"}, "ContentVariables": {"{\"1\":\"http://example.com/cat2.jpg\",\"2\":\"tomorrow\"}"}},
			Headers: map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
		}},
		ExpectedExtIDs: []string{"1002"},
	},
	{
		Label:     "Template Send missing external ID",
		MsgText:   "templated message",
		MsgURN:    "whatsapp:250788383383",
		MsgLocale: "eng",
		MsgTemplating: `{
			"template": {"uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3", "name": "revive_issue"},
			"components": [
				{"type": "body", "name": "body", "variables": {"1": 0, "2": 1}}
			],
			"variables": [
				{"type": "text", "value": "Chef"},
				{"type": "text" , "value": "tomorrow"}
			],
			"language": "en_US"
		}`,
		ExpectedError: courier.ErrMessageInvalid,
	},
	{
		Label:     "Error Code",
		MsgText:   "Error Code",
		MsgURN:    "whatsapp:250788383383",
		MsgLocale: "eng",
		MsgTemplating: `{
			"template": {"uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3", "name": "revive_issue"},
			"components": [
				{"type": "body", "name": "body", "variables": {"1": 0, "2": 1}}
			],
			"variables": [
				{"type": "text", "value": "Chef"},
				{"type": "text" , "value": "tomorrow"}
			],
			"external_id": "ext_id_revive_issue",
			"language": "en_US"
		}`,
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twilio.com/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(400, nil, []byte(`{ "code": 1001 }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form:    url.Values{"To": {"whatsapp:+250788383383"}, "From": {"whatsapp:+12065551212"}, "MessagingServiceSid": {"messageServiceSID"}, "StatusCallback": {"https://localhost/c/twa/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}, "ContentSid": {"ext_id_revive_issue"}, "ContentVariables": {"{\"1\":\"Chef\",\"2\":\"tomorrow\"}"}},
			Headers: map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
		}},
		ExpectedError: courier.ErrFailedWithReason("1001", "Service specific error: 1001."),
	},
	{
		Label:     "Stopped Contact Code",
		MsgText:   "Stopped Contact",
		MsgURN:    "whatsapp:250788383383",
		MsgLocale: "eng",
		MsgTemplating: `{
			"template": {"uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3", "name": "revive_issue"},
			"components": [
				{"type": "body", "name": "body", "variables": {"1": 0, "2": 1}}
			],
			"variables": [
				{"type": "text", "value": "Chef"},
				{"type": "text" , "value": "tomorrow"}
			],
			"external_id": "ext_id_revive_issue",
			"language": "en_US"
		}`,
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twilio.com/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(400, nil, []byte(`{ "code": 21610 }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form:    url.Values{"To": {"whatsapp:+250788383383"}, "From": {"whatsapp:+12065551212"}, "MessagingServiceSid": {"messageServiceSID"}, "StatusCallback": {"https://localhost/c/twa/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}, "ContentSid": {"ext_id_revive_issue"}, "ContentVariables": {"{\"1\":\"Chef\",\"2\":\"tomorrow\"}"}},
			Headers: map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
		}},
		ExpectedError: courier.ErrContactStopped,
	},
	{
		Label:     "No SID",
		MsgText:   "No SID",
		MsgURN:    "whatsapp:250788383383",
		MsgLocale: "eng",
		MsgTemplating: `{
			"template": {"uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3", "name": "revive_issue"},
			"components": [
				{"type": "body", "name": "body", "variables": {"1": 0, "2": 1}}
			],
			"variables": [
				{"type": "text", "value": "Chef"},
				{"type": "text" , "value": "tomorrow"}
			],
			"external_id": "ext_id_revive_issue",
			"language": "en_US"
		}`,
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twilio.com/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(200, nil, []byte(`{ }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form:    url.Values{"To": {"whatsapp:+250788383383"}, "From": {"whatsapp:+12065551212"}, "MessagingServiceSid": {"messageServiceSID"}, "StatusCallback": {"https://localhost/c/twa/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}, "ContentSid": {"ext_id_revive_issue"}, "ContentVariables": {"{\"1\":\"Chef\",\"2\":\"tomorrow\"}"}},
			Headers: map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
		}},
		ExpectedLogErrors: []*clogs.Error{courier.ErrorResponseValueMissing("sid")},
	},
	{
		Label:     "Error Sending",
		MsgText:   "Error Message",
		MsgURN:    "whatsapp:250788383383",
		MsgLocale: "eng",
		MsgTemplating: `{
			"template": {"uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3", "name": "revive_issue"},
			"components": [
				{"type": "body", "name": "body", "variables": {"1": 0, "2": 1}}
			],
			"variables": [
				{"type": "text", "value": "Chef"},
				{"type": "text" , "value": "tomorrow"}
			],
			"external_id": "ext_id_revive_issue",
			"language": "en_US"
		}`,
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twilio.com/2010-04-01/Accounts/accountSID/Messages.json": {
				httpx.NewMockResponse(401, nil, []byte(`{ "error": "out of credits" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form:    url.Values{"To": {"whatsapp:+250788383383"}, "From": {"whatsapp:+12065551212"}, "MessagingServiceSid": {"messageServiceSID"}, "StatusCallback": {"https://localhost/c/twa/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&action=callback"}, "ContentSid": {"ext_id_revive_issue"}, "ContentVariables": {"{\"1\":\"Chef\",\"2\":\"tomorrow\"}"}},
			Headers: map[string]string{"Authorization": "Basic YWNjb3VudFNJRDphdXRoVG9rZW4="},
		}},
		ExpectedError: courier.ErrResponseStatus,
	},
}

func TestOutgoing(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "T", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			configAccountSID:        "accountSID",
			courier.ConfigAuthToken: "authToken"})

	var tmsDefaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56cd", "TMS", "", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			configMessagingServiceSID: "messageServiceSID",
			configAccountSID:          "accountSID",
			courier.ConfigAuthToken:   "authToken"})

	var tmsShortenLinksChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56cd", "TMS", "", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			configLinkShortening:      true,
			configMessagingServiceSID: "messageServiceSID",
			configAccountSID:          "accountSID",
			courier.ConfigAuthToken:   "authToken"})

	var twDefaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TW", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			configAccountSID:        "accountSID",
			courier.ConfigAuthToken: "authToken",
			configSendURL:           "http://example.com/twiml_api/",
		})

	var swChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SW", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			configAccountSID:        "accountSID",
			courier.ConfigAuthToken: "authToken",
			configSendURL:           "http://example.com/sigware_api/",
		})

	RunOutgoingTestCases(t, defaultChannel, newTWIMLHandler("T", "Twilio", true), defaultSendTestCases, []string{httpx.BasicAuth("accountSID", "authToken")}, nil)
	RunOutgoingTestCases(t, tmsDefaultChannel, newTWIMLHandler("TMS", "Twilio Messaging Service", true), tmsDefaultSendTestCases, []string{httpx.BasicAuth("accountSID", "authToken")}, nil)
	RunOutgoingTestCases(t, tmsShortenLinksChannel, newTWIMLHandler("TMS", "Twilio Messaging Service", true), tmsShortenLinks, []string{httpx.BasicAuth("accountSID", "authToken")}, nil)
	RunOutgoingTestCases(t, twDefaultChannel, newTWIMLHandler("TW", "TwiML", true), twDefaultSendTestCases, []string{httpx.BasicAuth("accountSID", "authToken")}, nil)
	RunOutgoingTestCases(t, swChannel, newTWIMLHandler("SW", "SignalWire", false), swSendTestCases, []string{httpx.BasicAuth("accountSID", "authToken")}, nil)

	waChannel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SW", "+12065551212", "US",
		[]string{urns.WhatsApp.Prefix},
		map[string]any{
			configAccountSID:        "accountSID",
			courier.ConfigAuthToken: "authToken",
			configSendURL:           "http://example.com/sigware_api/",
		},
	)

	RunOutgoingTestCases(t, waChannel, newTWIMLHandler("T", "Twilio Whatsapp", true), waSendTestCases, []string{httpx.BasicAuth("accountSID", "authToken")}, nil)

	twaChannel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TWA", "+12065551212", "US",
		[]string{urns.WhatsApp.Prefix},
		map[string]any{
			configAccountSID:          "accountSID",
			courier.ConfigAuthToken:   "authToken",
			configMessagingServiceSID: "messageServiceSID",
		},
	)

	RunOutgoingTestCases(t, twaChannel, newTWIMLHandler("TWA", "Twilio Whatsapp", true), twaSendTestCases, []string{httpx.BasicAuth("accountSID", "authToken")}, nil)
}

func TestBuildAttachmentRequest(t *testing.T) {
	mb := test.NewMockBackend()

	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "T", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			configAccountSID:        "accountSID",
			courier.ConfigAuthToken: "authToken"})

	twHandler := &handler{NewBaseHandler(courier.ChannelType("T"), "Twilio"), true}
	req, _ := twHandler.BuildAttachmentRequest(context.Background(), mb, defaultChannel, "https://example.org/v1/media/41", nil)
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, "Basic YWNjb3VudFNJRDphdXRoVG9rZW4=", req.Header.Get("Authorization"))

	var swChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SW", "2020", "US",
		[]string{urns.WhatsApp.Prefix},
		map[string]any{
			configAccountSID:        "accountSID",
			courier.ConfigAuthToken: "authToken",
			configSendURL:           "BASE_URL",
		})
	swHandler := &handler{NewBaseHandler(courier.ChannelType("SW"), "SignalWire"), false}
	req, _ = swHandler.BuildAttachmentRequest(context.Background(), mb, swChannel, "https://example.org/v1/media/41", nil)
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, "", req.Header.Get("Authorization"))
}
