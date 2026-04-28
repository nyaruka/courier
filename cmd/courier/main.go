package main

import (
	"github.com/nyaruka/courier/cmd"

	// load available backends
	_ "github.com/nyaruka/courier/backends/rapidpro"

	// load channel handler packages
	_ "github.com/nyaruka/courier/handlers/africastalking"
	_ "github.com/nyaruka/courier/handlers/arabiacell"
	_ "github.com/nyaruka/courier/handlers/bandwidth"
	_ "github.com/nyaruka/courier/handlers/bongolive"
	_ "github.com/nyaruka/courier/handlers/burstsms"
	_ "github.com/nyaruka/courier/handlers/chip"
	_ "github.com/nyaruka/courier/handlers/clickatell"
	_ "github.com/nyaruka/courier/handlers/clickmobile"
	_ "github.com/nyaruka/courier/handlers/clicksend"
	_ "github.com/nyaruka/courier/handlers/dart"
	_ "github.com/nyaruka/courier/handlers/dialog360"
	_ "github.com/nyaruka/courier/handlers/dmark"
	_ "github.com/nyaruka/courier/handlers/external"
	_ "github.com/nyaruka/courier/handlers/facebook_legacy"
	_ "github.com/nyaruka/courier/handlers/firebase"
	_ "github.com/nyaruka/courier/handlers/freshchat"
	_ "github.com/nyaruka/courier/handlers/globe"
	_ "github.com/nyaruka/courier/handlers/highconnection"
	_ "github.com/nyaruka/courier/handlers/hormuud"
	_ "github.com/nyaruka/courier/handlers/hub9"
	_ "github.com/nyaruka/courier/handlers/i2sms"
	_ "github.com/nyaruka/courier/handlers/infobip"
	_ "github.com/nyaruka/courier/handlers/jasmin"
	_ "github.com/nyaruka/courier/handlers/jiochat"
	_ "github.com/nyaruka/courier/handlers/justcall"
	_ "github.com/nyaruka/courier/handlers/kaleyra"
	_ "github.com/nyaruka/courier/handlers/kannel"
	_ "github.com/nyaruka/courier/handlers/line"
	_ "github.com/nyaruka/courier/handlers/m3tech"
	_ "github.com/nyaruka/courier/handlers/macrokiosk"
	_ "github.com/nyaruka/courier/handlers/mblox"
	_ "github.com/nyaruka/courier/handlers/messagebird"
	_ "github.com/nyaruka/courier/handlers/messangi"
	_ "github.com/nyaruka/courier/handlers/meta"
	_ "github.com/nyaruka/courier/handlers/mtarget"
	_ "github.com/nyaruka/courier/handlers/mtn"
	_ "github.com/nyaruka/courier/handlers/nexmo"
	_ "github.com/nyaruka/courier/handlers/novo"
	_ "github.com/nyaruka/courier/handlers/playmobile"
	_ "github.com/nyaruka/courier/handlers/plivo"
	_ "github.com/nyaruka/courier/handlers/rocketchat"
	_ "github.com/nyaruka/courier/handlers/shaqodoon"
	_ "github.com/nyaruka/courier/handlers/slack"
	_ "github.com/nyaruka/courier/handlers/smscentral"
	_ "github.com/nyaruka/courier/handlers/start"
	_ "github.com/nyaruka/courier/handlers/telegram"
	_ "github.com/nyaruka/courier/handlers/telesom"
	_ "github.com/nyaruka/courier/handlers/test"
	_ "github.com/nyaruka/courier/handlers/thinq"
	_ "github.com/nyaruka/courier/handlers/turn"
	_ "github.com/nyaruka/courier/handlers/twiml"
	_ "github.com/nyaruka/courier/handlers/viber"
	_ "github.com/nyaruka/courier/handlers/vk"
	_ "github.com/nyaruka/courier/handlers/wavy"
	_ "github.com/nyaruka/courier/handlers/wechat"
	_ "github.com/nyaruka/courier/handlers/whatsapp_legacy"
	_ "github.com/nyaruka/courier/handlers/yo"
	_ "github.com/nyaruka/courier/handlers/zenvia"
)

var (
	// https://goreleaser.com/cookbooks/using-main.version
	version = "dev"
	date    = "unknown"
)

func main() {
	cmd.Run(cmd.Service(version, date))
}
