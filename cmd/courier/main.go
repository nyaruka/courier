package main

import (
	"github.com/nyaruka/courier/v26/cmd"

	// load available backends
	_ "github.com/nyaruka/courier/v26/backends/rapidpro"

	// load channel handler packages
	_ "github.com/nyaruka/courier/v26/handlers/africastalking"
	_ "github.com/nyaruka/courier/v26/handlers/arabiacell"
	_ "github.com/nyaruka/courier/v26/handlers/bandwidth"
	_ "github.com/nyaruka/courier/v26/handlers/bongolive"
	_ "github.com/nyaruka/courier/v26/handlers/burstsms"
	_ "github.com/nyaruka/courier/v26/handlers/chip"
	_ "github.com/nyaruka/courier/v26/handlers/clickatell"
	_ "github.com/nyaruka/courier/v26/handlers/clickmobile"
	_ "github.com/nyaruka/courier/v26/handlers/clicksend"
	_ "github.com/nyaruka/courier/v26/handlers/dart"
	_ "github.com/nyaruka/courier/v26/handlers/dialog360"
	_ "github.com/nyaruka/courier/v26/handlers/dmark"
	_ "github.com/nyaruka/courier/v26/handlers/external"
	_ "github.com/nyaruka/courier/v26/handlers/facebook_legacy"
	_ "github.com/nyaruka/courier/v26/handlers/firebase"
	_ "github.com/nyaruka/courier/v26/handlers/freshchat"
	_ "github.com/nyaruka/courier/v26/handlers/globe"
	_ "github.com/nyaruka/courier/v26/handlers/highconnection"
	_ "github.com/nyaruka/courier/v26/handlers/hormuud"
	_ "github.com/nyaruka/courier/v26/handlers/hub9"
	_ "github.com/nyaruka/courier/v26/handlers/i2sms"
	_ "github.com/nyaruka/courier/v26/handlers/infobip"
	_ "github.com/nyaruka/courier/v26/handlers/jasmin"
	_ "github.com/nyaruka/courier/v26/handlers/jiochat"
	_ "github.com/nyaruka/courier/v26/handlers/justcall"
	_ "github.com/nyaruka/courier/v26/handlers/kaleyra"
	_ "github.com/nyaruka/courier/v26/handlers/kannel"
	_ "github.com/nyaruka/courier/v26/handlers/line"
	_ "github.com/nyaruka/courier/v26/handlers/m3tech"
	_ "github.com/nyaruka/courier/v26/handlers/macrokiosk"
	_ "github.com/nyaruka/courier/v26/handlers/mblox"
	_ "github.com/nyaruka/courier/v26/handlers/messagebird"
	_ "github.com/nyaruka/courier/v26/handlers/messangi"
	_ "github.com/nyaruka/courier/v26/handlers/meta"
	_ "github.com/nyaruka/courier/v26/handlers/mtarget"
	_ "github.com/nyaruka/courier/v26/handlers/mtn"
	_ "github.com/nyaruka/courier/v26/handlers/nexmo"
	_ "github.com/nyaruka/courier/v26/handlers/novo"
	_ "github.com/nyaruka/courier/v26/handlers/playmobile"
	_ "github.com/nyaruka/courier/v26/handlers/plivo"
	_ "github.com/nyaruka/courier/v26/handlers/rocketchat"
	_ "github.com/nyaruka/courier/v26/handlers/shaqodoon"
	_ "github.com/nyaruka/courier/v26/handlers/slack"
	_ "github.com/nyaruka/courier/v26/handlers/smscentral"
	_ "github.com/nyaruka/courier/v26/handlers/start"
	_ "github.com/nyaruka/courier/v26/handlers/telegram"
	_ "github.com/nyaruka/courier/v26/handlers/telesom"
	_ "github.com/nyaruka/courier/v26/handlers/test"
	_ "github.com/nyaruka/courier/v26/handlers/thinq"
	_ "github.com/nyaruka/courier/v26/handlers/turn"
	_ "github.com/nyaruka/courier/v26/handlers/twiml"
	_ "github.com/nyaruka/courier/v26/handlers/viber"
	_ "github.com/nyaruka/courier/v26/handlers/vk"
	_ "github.com/nyaruka/courier/v26/handlers/wavy"
	_ "github.com/nyaruka/courier/v26/handlers/wechat"
	_ "github.com/nyaruka/courier/v26/handlers/whatsapp_legacy"
	_ "github.com/nyaruka/courier/v26/handlers/yo"
	_ "github.com/nyaruka/courier/v26/handlers/zenvia"
)

var (
	// https://goreleaser.com/cookbooks/using-main.version
	version = "dev"
	date    = "unknown"
)

func main() {
	cmd.Run(cmd.Service(version, date))
}
