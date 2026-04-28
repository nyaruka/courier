package hub9

import (
	"github.com/nyaruka/courier/v26"
	"github.com/nyaruka/courier/v26/handlers/dart"
)

var (
	sendURL      = "http://175.103.48.29:28078/testing/smsmt.php"
	maxMsgLength = 1600
)

func init() {
	courier.RegisterHandler(dart.NewHandler("H9", "Hub9", sendURL, maxMsgLength))
}
