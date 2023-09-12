package whatsapp

import "github.com/nyaruka/courier"

var WACStatusMapping = map[string]courier.MsgStatus{
	"sending":   courier.MsgStatusWired,
	"sent":      courier.MsgStatusSent,
	"delivered": courier.MsgStatusDelivered,
	"read":      courier.MsgStatusDelivered,
	"failed":    courier.MsgStatusFailed,
}

var WACIgnoreStatuses = map[string]bool{
	"deleted": true,
}
