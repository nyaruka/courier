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

