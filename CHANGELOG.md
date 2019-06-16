v2.0.2
----------
0;95;0c# Enter any comments for inclusion in the CHANGELOG on this revision below, you can use markdown
 * ignore viber dlrs as they are sent for both in and out

v2.0.1
----------
 * add WhatsApp scheme support for TWIML channels

v2.0.0
----------
 * ignore flow server enabled attribute on orgs
 * stop looking / writing is_test on contact

v1.2.160
----------
 * add bearer before auth token for Hormuud

v1.2.159
----------
 * add SignalWire handler (https://www.signalwire.com)
 * refactor twilio->twiml
 * remove ignore DLR global config, make per channel for TWIML channels

v1.2.158
----------
 * add ClickSend channel

v1.2.157
----------
 * increase http timeouts to 60 seconds for AfricasTalking, Hormuud token lasts 90 minutes

v1.2.156
----------
 * update Portuguese mapping

v1.2.155
----------
 * new Hormuud channel for somalia
 * add video support for WhatsApp

v1.2.154
----------
 * have batch committer print when flushed
 * move stopping of bulk committers to cleanup phase

v1.2.153
----------
 * Switch to newer library for UUID generation

v1.2.152
----------
 * raise delay before bulk commits to 500ms

v1.2.151
----------
 * optimize sends via bulk inserts and updates

v1.2.150
----------
 * allow configuring custom mo fields for external channels

v1.2.149
----------
* implement sending whatsapp templates

v1.2.148
----------
 * Add maintenance mode to run without a DB and only spool inbound requests

v1.2.147
----------
 * Prevent Facebook duplicate messages, dedupe in external id

v1.2.146
----------
 * ignore deleted status for whatsapp

v1.2.145
----------
 * mark deleted WhatsApp messages as failed

v1.2.144
----------
 * include extra for channel events in response

v1.2.143
----------
 * deduplicate WA messages on external ID

v1.2.142
----------
 * normalize TEL urns with the country

v1.2.141
----------
 * latest phonenumbers

v1.2.140
----------
 * Queue welcome message event to be handle by mailroom

v1.2.139
----------
 * add sub-message ids for long messages on play mobile
 * send configured welcome message on converssation started for Viber

v1.2.138
----------
 * proper name for queues to check size

v1.2.137
----------
 * log queue sizes and new contact creations to librato

v1.2.136
----------
 * add queued on to all tasks

v1.2.135
----------
 * move queued on to task level

v1.2.134
----------
 * add queued_on to tasks sent to mailroom so we can calculate latency

v1.2.133
----------
 * fixes us creating an orphaned contact when we get two messages at the same instant

v1.2.132
----------
 * send fb attachments first instead of last, add quick replies to last message instead of first

v1.2.131
----------
 * Fix to use DLRID for Bongolive status reports

v1.2.130
----------
 * Use unix timestamp for MO receive on WAVy channels

v1.2.129
----------
 * Make bongolive inbound msg type optional
 * Properly handle long attachment description for Viber

v1.2.128
----------
 * Load BL handler package
 * Add support for Movile/Wavy channels, Thanks to MGov to fund the development of the integration

v1.2.127
----------
 * Use UPPERCASE parameters for BL channels
 * Migrate courier to PostgreSQL 10

v1.2.126
----------
 * Switch BL channels used API

v1.2.125
----------
 * add support for Bongo Live channels
 * Switch to use nyaruka/librato package
 * Complete conversion to module

v1.2.124
----------
 * Updated Zenvia endpoint according to new API

v1.2.123
----------
 * set session timeouts when specified by mailroom

v1.2.122
----------
 * Support using the custom configured content type for EX channels
 * Fix panicr on parsing SOAP body for EX channels
 * Support sending images and videos in Twitter

v1.2.121
----------
 * fix twitter sending

v1.2.120
----------
 * Twitter media attachments

v1.2.118
----------
 * Commit transaction when adding URN to contact with success
 * Fix typo
 * Simply remove URNs by update query
 * Fix params names
 * Fix Facebook for contact duplicates when using referral, save the proper Facebook URN when we first successfully send to the referral contact URN
 * Ignore error for Jiochat user name lookup

v1.2.117
----------
 * remove ipv6 binding for redis server

v1.2.116
----------
 * add urn id to channel events

v1.2.115
----------
 * do not return errors from whatsapp send during client errors

v1.2.114
----------
 * Better channel logs support for WA channels

v1.2.113
----------
 * prevent races in dupe detection by clearing before sending
 * use URN identity for URN fingerprint

v1.2.112
----------
 * return empty content when receiving i2sms messages

v1.2.111
----------
 * add i2sms channel

v1.2.110
----------
 * allow setting kannel dlr mask

v1.2.109
----------
 * Support receiving MO msgs in XML format

v1.2.108
----------
 * Add channel log for when we fail to get the response expected
 * Support checking configured response content for EX channels
 * Add stopped event handler for EX channels

v1.2.107
----------
 * queue tasks to mailroom for flow_server_enabled orgs, requires newest rapidpro

v1.2.106
----------
 * flush to librato every second
 * Add authorization token requirement to receive messages on Novo Channel

v1.2.105
----------
 * optimize writing message status for external case

v1.2.104
----------
 * optimize status update when we know message id

v1.2.103
----------
 * add media handling for whatsapp

v1.2.102
----------
 * clear dedupes on outgoing messages

v1.2.101
----------
 * AT date like 2006-01-02 15:04:05, without T nor Z

v1.2.100
----------
 * Accept AT requests with timestamps without Z
 * Ignore status update for incoming messsages

v1.2.99
----------
 * Support smart encoding for post requests on EX channels
 * Add novo channel with send capability
 * log the error when PQ fails to connect
 * Changed the default redis database to match rapid pro redis database

v1.2.98
----------
 * treat empty content type as text
 * updated go.mod and go.sum files for go modules support

v1.2.97
----------
 * add optional transliteration parameter for MT messages with infobip
 * add support to use configured encoding for EX channels

v1.2.96
----------
 * Add support for WeChat

v1.2.95
----------
 * use utf8 to shorten string so we don't end up with an invalid string

v1.2.94
----------
 * proper backdown for Nexmo retries

v1.2.93
----------
 * Trim contact names at 127 characters

v1.2.92
----------
 * move to gocommon, honor e164 numbers handed to us

v1.2.91
----------
 * update to latest phonenumbers, update tests

v1.2.90
----------
 * reduce spacing between messages to 3 seconds
 * add an address option to bind to a specific network interface address
 * honor rapidpro constants for content-type

v1.2.89
----------
 * Add burst sms handler / sender (Australia / New Zealand)

v1.2.88
----------
 * set expiration of sent sets in redis

v1.2.87
----------
 * update line channel to use v2 of API
 * add messangi channel

v1.2.86
----------
 * remove unacked, that's part of celery's job

v1.2.85
----------
 * update celery queuing to new kombu format

v1.2.84
----------
 * write UUID fields for incoming messages

v1.2.83
----------
 * implement unified webhook endpoint for whatsapp

v1.2.82
----------
 * Implement new WhatsApp API for sending

v1.2.81 
----------
 * Honor x-forwarded-path header for twilio signatures

v1.2.80
----------
 * Make sure the messageid is unique for multiple part messages for Dartmedia

v1.2.79
----------
 * Decode &amp; in Twitter message bodies

v1.2.78
----------
 * Accept Hub9/Dart encrypted phonenumber identifier and save then as external scheme

v1.2.77
----------
 * Update .gitignore

v1.2.76
----------
 * Update .gitignore

v1.2.75
----------
 * Update readme, formatting
 * Add more lines to show annotation format
 * More lines.. why not

v1.2.74
----------
 * Update changelog, remove spurious version

v1.2.73
----------
 * do not log illegal methods or 404s

