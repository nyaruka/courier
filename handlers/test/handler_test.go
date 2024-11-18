package test

import (
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/random"
	"github.com/nyaruka/gocommon/urns"
)

var sendTestCases = []OutgoingTestCase{
	{
		Label:            "Normal send",
		MsgText:          "Hi here",
		MsgURN:           "tel:+12067791234",
		ExpectedRequests: []ExpectedRequest{},
	},
	{
		Label:            "Error send",
		MsgText:          "Hi here \\error",
		MsgURN:           "tel:+12067791234",
		ExpectedRequests: []ExpectedRequest{},
		ExpectedError:    courier.ErrConnectionFailed,
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
