package ussd

import (
	"github.com/nyaruka/courier"
)

var testChannels = []courier.Channel{
courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "US", "*202#", "US", nil),
}

var ignoreChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "US", "*202#", "US", map[string]interface{}{"ignore_sent": true}),
}
