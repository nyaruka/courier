package models

import (
	"github.com/nyaruka/gocommon/aws/dynamo"
	"github.com/nyaruka/gocommon/spools"
)

// hooks to expose package internals to tests in the models_test package

var (
	ContactForURN     = contactForURN
	ContactForMsg     = contactForMsg
	AddContactURN     = addContactURN
	WriteMsgToDB      = writeMsgToDB
	InsertTimeoutFire = insertTimeoutFire
)

// SetURNSleep enables or disables the artificial sleep used to test URN creation races
func SetURNSleep(v bool) { urnSleep = v }

// MsgSpool returns the package level incoming msg spool
func MsgSpool() *spools.Spool[*MsgIn] { return msgSpool }

// StatusSpool returns the package level status update spool
func StatusSpool() *spools.Spool[*StatusUpdate] { return statusSpool }

// EventSpool returns the package level channel event spool
func EventSpool() *spools.Spool[*ChannelEvent] { return eventSpool }

// FlushStatusWriter flushes the package level batched status writer
func FlushStatusWriter() { statusWriter.Flush() }

// ChannelLogDynamoKey returns the DynamoDB key that the given channel log is written with
func ChannelLogDynamoKey(clog *ChannelLog) dynamo.Key { return (&dynamoChannelLog{clog}).DynamoKey() }
