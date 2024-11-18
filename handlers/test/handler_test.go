package test

import (
	"testing"

	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/random"
	"github.com/nyaruka/gocommon/urns"
)

var sendTestCases = []OutgoingTestCase{
	{
		Label:            "Plain Send",
		MsgText:          "Simple Message ☺",
		MsgURN:           "tel:+12067791234",
		ExpectedRequests: []ExpectedRequest{},
	},
}

func TestOutgoing(t *testing.T) {
	random.SetGenerator(random.NewSeededGenerator(123))
	defer random.SetGenerator(random.DefaultGenerator)

	var channel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TST", "+12065551212", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{},
	)
	RunOutgoingTestCases(t, channel, newHandler(), sendTestCases, nil, nil)
}
