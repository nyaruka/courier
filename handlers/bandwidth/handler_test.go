package bandwidth

import (
	"context"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/courier/utils/clogs"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
)

const (
	receiveURL = "/c/bw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusURL  = "/c/bw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"
)

var helloMsg = `[{
	  "type"          : "message-received",
	  "time"          : "2016-09-14T18:20:16Z",
	  "description"   : "Incoming message received",
	  "to"            : "12345678902",
	  "message"       : {
		"id"            : "14762070468292kw2fuqty55yp2b2",
		"time"          : "2016-09-14T18:20:16Z",
		"to"            : ["+12345678902"],
		"from"          : "+12065551234",
		"text"          : "hello world",
		"applicationId" : "93de2206-9669-4e07-948d-329f4b722ee2",
		"media"         : [
		  "https://messaging.bandwidth.com/api/v2/users/{accountId}/media/14762070468292kw2fuqty55yp2b2/0/bw.png"
		  ],
		"owner"         : "+12345678902",
		"direction"     : "in",
		"segmentCount"  : 1
	  }
	}
  ]`

var invalidURN = `[{
	"type"          : "message-received",
	"time"          : "2016-09-14T18:20:16Z",
	"description"   : "Incoming message received",
	"to"            : "12345678902",
	"message"       : {
	  "id"            : "14762070468292kw2fuqty55yp2b2",
	  "time"          : "2016-09-14T18:20:16Z",
	  "to"            : ["+12345678902"],
	  "from"          : "MTN",
	  "text"          : "hello world",
	  "applicationId" : "93de2206-9669-4e07-948d-329f4b722ee2",
	  "media"         : [
		"https://messaging.bandwidth.com/api/v2/users/{accountId}/media/14762070468292kw2fuqty55yp2b2/0/bw.png"
		],
	  "owner"         : "+12345678902",
	  "direction"     : "in",
	  "segmentCount"  : 1
	}
  }
]`

var invalidDateFormat = `[{
	"type"          : "message-received",
	"time"          : "2016-09-14 18:20:16",
	"description"   : "Incoming message received",
	"to"            : "12345678902",
	"message"       : {
	  "id"            : "14762070468292kw2fuqty55yp2b2",
	  "time"          : "2016-09-14 18:20:16",
	  "to"            : ["+12345678902"],
	  "from"          : "MTN",
	  "text"          : "hello world",
	  "applicationId" : "93de2206-9669-4e07-948d-329f4b722ee2",
	  "media"         : [
		"https://messaging.bandwidth.com/api/v2/users/{accountId}/media/14762070468292kw2fuqty55yp2b2/0/bw.png"
		],
	  "owner"         : "+12345678902",
	  "direction"     : "in",
	  "segmentCount"  : 1
	}
  }
]`

var validStatusSent = `[
    {
        "time": "2020-06-25T18:42:36.979Z",
        "type": "message-sending",
        "to": "+15554443333",
        "description": "Message is sending to carrier",
        "message": {
            "id": "12345",
            "owner": "+15552221111",
            "applicationId": "cfd4fb83-7531-4acc-b471-42d0bb76a65c",
            "time": "2020-06-25T18:42:35.876Z",
            "segmentCount": 1,
            "direction": "out",
            "to": ["+15554443333"],
            "from": "+15552221111",
            "text": "",
            "media": ["https://dev.bandwidth.com/images/bandwidth-logo.png"],
            "tag": "v2 lab"
        }
    }
]`

var validStatusDelivered = `[
	{
	  "type"          : "message-delivered",
	  "time"          : "2016-09-14T18:20:16Z",
	  "description"   : "ok",
	  "to"            : "+12345678902",
	  "message"       : {
		"id"            : "12345",
		"time"          : "2016-09-14T18:20:16Z",
		"to"            : ["+12345678902"],
		"from"          : "+12345678901",
		"text"          : "",
		"applicationId" : "93de2206-9669-4e07-948d-329f4b722ee2",
		"owner"         : "+12345678902",
		"direction"     : "out",
		"segmentCount"  : 1
	  }
	}
  ]
`
var validStatusFailed = `[
	{
	  "type"          : "message-failed",
	  "time"          : "2016-09-14T18:20:16Z",
	  "description"   : "forbidden to country",
	  "to"            : "+52345678903",
	  "errorCode"     : 4432,
	  "message"       : {
		"id"            : "14762070468292kw2fuqty55yp2b2",
		"time"          : "2016-09-14T18:20:16Z",
		"to"            : [
			"+12345678902",
			"+52345678903"
		  ],
		"from"          : "+12345678901",
		"text"          : "",
		"applicationId" : "93de2206-9669-4e07-948d-329f4b722ee2",
		"media"         : [
			"https://dev.bandwidth.com/images/bandwidth-logo.png"
		  ],
		"owner"         : "+12345678901",
		"direction"     : "out",
		"segmentCount"  : 1
	  }
	}
  ]`

var invalidStatus = `[
    {
        "time": "2020-06-25T18:42:36.979Z",
        "type": "message-bla",
        "to": "+15554443333",
        "description": "Message is sending to carrier",
        "message": {
            "id": "12345",
            "owner": "+15552221111",
            "applicationId": "cfd4fb83-7531-4acc-b471-42d0bb76a65c",
            "time": "2020-06-25T18:42:35.876Z",
            "segmentCount": 1,
            "direction": "out",
            "to": ["+15554443333"],
            "from": "+15552221111",
            "text": "",
            "media": ["https://dev.bandwidth.com/images/bandwidth-logo.png"],
            "tag": "v2 lab"
        }
    }
]`

var incomingCases = []IncomingTestCase{
	{
		Label:                "Receive Valid",
		URL:                  receiveURL,
		Data:                 helloMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("hello world"),
		ExpectedURN:          "tel:+12065551234",
		ExpectedAttachments:  []string{"https://messaging.bandwidth.com/api/v2/users/{accountId}/media/14762070468292kw2fuqty55yp2b2/0/bw.png"},
	},
	{
		Label:                "Invalid URN",
		URL:                  receiveURL,
		Data:                 invalidURN,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "not a possible number",
	},
	{
		Label:                "Invalid URN",
		URL:                  receiveURL,
		Data:                 invalidDateFormat,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "invalid date format",
	},
	{
		Label:                "Invalid Status",
		URL:                  statusURL,
		Data:                 invalidStatus,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: `unknown status 'message-bla'`,
	},
	{
		Label:                "Status delivered",
		URL:                  statusURL,
		Data:                 validStatusSent,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"S"`,
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "12345", Status: courier.MsgStatusSent}},
	},
	{
		Label:                "Status delivered",
		URL:                  statusURL,
		Data:                 validStatusDelivered,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "12345", Status: courier.MsgStatusDelivered}},
	},
	{
		Label:                "Status failed",
		URL:                  statusURL,
		Data:                 validStatusFailed,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"F"`,
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "14762070468292kw2fuqty55yp2b2", Status: courier.MsgStatusFailed}},
		ExpectedErrors:       []*clogs.Error{courier.ErrorExternal("4432", "forbidden to country")},
	},
}

func TestIncoming(t *testing.T) {
	chs := []courier.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "BW", "2020", "US",
			[]string{urns.Phone.Prefix},
			map[string]any{courier.ConfigUsername: "user1", courier.ConfigPassword: "pass1", configAccountID: "accound-id", configMsgApplicationID: "application-id"},
		),
	}

	RunIncomingTestCases(t, chs, newHandler(), incomingCases)
}

var outgoingCases = []OutgoingTestCase{
	{
		Label:   "Plain send",
		MsgText: "Simple Message ☺",
		MsgURN:  "tel:+12067791234",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://messaging.bandwidth.com/api/v2/users/accound-id/messages": {
				httpx.NewMockResponse(200, nil, []byte(`{"id": "55555"}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{
					"Content-Type":  "application/json",
					"Accept":        "application/json",
					"Authorization": "Basic dXNlcjE6cGFzczE=",
				},
				Body: `{"applicationId":"application-id","to":["+12067791234"],"from":"2020","text":"Simple Message ☺"}`,
			},
		},
		ExpectedExtIDs: []string{"55555"},
	},
	{
		Label:          "Attachment",
		MsgText:        "My pic!",
		MsgURN:         "tel:+12067791234",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://messaging.bandwidth.com/api/v2/users/accound-id/messages": {
				httpx.NewMockResponse(200, nil, []byte(`{"id": "55555"}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{
					"Content-Type":  "application/json",
					"Accept":        "application/json",
					"Authorization": "Basic dXNlcjE6cGFzczE=",
				},
				Body: `{"applicationId":"application-id","to":["+12067791234"],"from":"2020","text":"My pic!","media":["https://foo.bar/image.jpg"]}`,
			},
		},
		ExpectedExtIDs: []string{"55555"},
	},
	{
		Label:          "Send Attachment no text",
		MsgText:        "",
		MsgURN:         "tel:+12067791234",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://messaging.bandwidth.com/api/v2/users/accound-id/messages": {
				httpx.NewMockResponse(200, nil, []byte(`{"id": "55555"}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{
					"Content-Type":  "application/json",
					"Accept":        "application/json",
					"Authorization": "Basic dXNlcjE6cGFzczE=",
				},
				Body: `{"applicationId":"application-id","to":["+12067791234"],"from":"2020","text":"","media":["https://foo.bar/image.jpg"]}`,
			},
		},
		ExpectedExtIDs: []string{"55555"},
	},
	{
		Label:   "No External ID",
		MsgText: "No External ID",
		MsgURN:  "tel:+12067791234",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://messaging.bandwidth.com/api/v2/users/accound-id/messages": {
				httpx.NewMockResponse(200, nil, []byte(`{}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{
					"Content-Type":  "application/json",
					"Accept":        "application/json",
					"Authorization": "Basic dXNlcjE6cGFzczE=",
				},
				Body: `{"applicationId":"application-id","to":["+12067791234"],"from":"2020","text":"No External ID"}`,
			},
		},
	},
	{
		Label:   "Error sending",
		MsgText: "Error Message",
		MsgURN:  "tel:+12067791234",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://messaging.bandwidth.com/api/v2/users/accound-id/messages": {
				httpx.NewMockResponse(401, nil, []byte(`{ "type": "request-validation", "description": "Your request could not be accepted" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{
					"Content-Type":  "application/json",
					"Accept":        "application/json",
					"Authorization": "Basic dXNlcjE6cGFzczE=",
				},
				Body: `{"applicationId":"application-id","to":["+12067791234"],"from":"2020","text":"Error Message"}`,
			},
		},
		ExpectedError: courier.ErrFailedWithReason("request-validation", "Your request could not be accepted"),
	},
}

func TestOutgoing(t *testing.T) {
	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "BW", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{courier.ConfigUsername: "user1", courier.ConfigPassword: "pass1", configAccountID: "accound-id", configMsgApplicationID: "application-id"},
	)

	RunOutgoingTestCases(t, ch, newHandler(), outgoingCases, []string{httpx.BasicAuth("user1", "pass1")}, nil)
}

func TestBuildAttachmentRequest(t *testing.T) {
	mb := test.NewMockBackend()
	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "BW", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{courier.ConfigUsername: "user1", courier.ConfigPassword: "pass1", configAccountID: "accound-id", configMsgApplicationID: "application-id"},
	)

	bwHandler := &handler{NewBaseHandler(courier.ChannelType("BW"), "Bandwidth")}
	req, _ := bwHandler.BuildAttachmentRequest(context.Background(), mb, ch, "https://example.org/v1/media/41", nil)
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, "Basic dXNlcjE6cGFzczE=", req.Header.Get("Authorization"))
}
