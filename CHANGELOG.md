v8.3.28 (2023-11-23)
-------------------------
 * Logging tweak

v8.3.27 (2023-10-31)
-------------------------
 * Prevent all courier HTTP requests from accessing local networks

v8.3.26 (2023-10-30)
-------------------------
 * Update to latest gocommon

v8.3.25 (2023-10-25)
-------------------------
 * Update docker image to go 1.21
 * Remove use of logrus and use slog with sentry
 * Bump golang.org/x/net from 0.14.0 to 0.17.0

v8.3.24 (2023-10-10)
-------------------------
 * Fix handling IG like hearts
 * Ignore attachments of type fallback on FBA channels
 * More logrus replacement to use slog

v8.3.23 (2023-10-04)
-------------------------
 * Switch channelevent.extra to always be strings
 * Add optin_id to channels_channelevent
 * Allow outgoing tests to check multiple requests

v8.3.22 (2023-09-27)
-------------------------
 * Use Facebook API v17.0

v8.3.21 (2023-09-25)
-------------------------
 * Support sending facebook message with opt-in auth token

v8.3.20 (2023-09-21)
-------------------------
 * Switch to using optin ids instead of uuids

v8.3.19 (2023-09-20)
-------------------------
 * Fix queueing of optin/optout events to mailroom
 * Implement sending opt-in requests for FBA channels
 * Simplfy handlers splitting up messages

v8.3.18 (2023-09-18)
-------------------------
 * Add separate MsgIn and MsgOut interface types
 * Use functional options pattern to create base handlers
 * Improve testing of status updates from handlers and allow testing of multiple status updates per request
 * Split up Meta notification payload into whatsapp and messenger specific parts

v8.3.17 (2023-09-14)
-------------------------
 * Fix stop contact event task names
 * Add support for FB notificaiton messages optin and optout events

v8.3.16 (2023-09-13)
-------------------------
 * Simplify interfaces that handlers have access to
 * Allow handlers to create arbitrary auth tokens with messages and channel events
 * Rename legacy FB and WA handlers
 * Refactor whatsapp handlers to be more DRY

v8.3.15 (2023-09-12)
-------------------------
 * Stop reading from ContactURN.auth and remove from model

v8.3.14 (2023-09-11)
-------------------------
 * Move whatsapp language matching into own util package and use i18n.BCP47Matcher
 * Update to latest gocommon and use i18n.Locale
 * Read from ContactURN.auth_tokens instead of .auth

v8.3.13 (2023-09-06)
-------------------------
 * Start writing ContactURN.auth_tokens
 * Update to latest null library and use Map[string] for channel event extra

v8.3.12 (2023-09-06)
-------------------------
 * Do more debug logging and less info logging

v8.3.11 (2023-09-06)
-------------------------
 * Add logging of requests with no associated channel
 * No need to try making DB queries when all msg IDs got resolved from redis

v8.3.10 (2023-09-05)
-------------------------
 * Don't rely on knowing msg id to determine if a log is attached
 * Rework handler tests so that test cases must explicitly say if they don't generate a channel log

v8.3.9 (2023-09-05)
-------------------------
 * Try to resolve sent external ids from redis
 * For received messages without external id, de-dupe by hash of text+attachments instead of just text

v8.3.8 (2023-08-31)
-------------------------
 * Update to latest redisx which fixes accuracy for sub-minute interval hashes
 * Update to new batchers in gocommon which are more efficient

v8.3.7 (2023-08-30)
-------------------------
 * Sender deletion handled by mailroom task

v8.3.6 (2023-08-30)
-------------------------
 * Rework writing msg statuses to always use id resolving

v8.3.5 (2023-08-30)
-------------------------
 * Rework writing status updates so that updates by external id also use the batcher

v8.3.4 (2023-08-24)
-------------------------
 * Update channel type to save external ID for MO messages if we can, so we can dedupe by that
 * Test with PostgreSQL 15

v8.3.3 (2023-08-17)
-------------------------
 * Remove Legacy Twitter (TT) type registration
 * Remove Blackmyna, Junebug, old Zenvia channel type handlers

v8.3.2 (2023-08-16)
-------------------------
 * Fix retrieve media files for D3C

v8.3.1 (2023-08-09)
-------------------------
 * Revert validator dep upgrade

v8.3.0 (2023-08-09)
-------------------------
 * Update to go 1.20
 * Update deps
 * Add Messagebird channel type

v8.2.1 (2023-08-03)
-------------------------
 * Always save http_logs as [] rather than null

v8.2.0 (2023-07-31)
-------------------------
 * Add docker file for dev

v8.1.33 (2023-07-28)
-------------------------
 * Rework deduping of incoming messages to ignore message content if message has id we can use
 * Use bulk SQL operation for msg status flushing

v8.1.32 (2023-07-26)
-------------------------
 * Fix setting of log type of channel logs and add additional types

v8.1.31 (2023-07-20)
-------------------------
 * Update deps including gocommon which changes requirement for storage paths to start with slash

v8.1.30 (2023-07-14)
-------------------------
 * Adjust to make sure we set the name of the document for the WAC files attached

v8.1.29 (2023-07-12)
-------------------------
 * Add log_policy field to channel
 * Add cpAddress parameters to MTN outbound requests

v8.1.28 (2023-07-10)
-------------------------
 * Tweak error message on media parse failure

v8.1.27 (2023-07-04)
-------------------------
 * Add writer to write attached logs to storage instead of db

v8.1.26 (2023-07-03)
-------------------------
 * Rework writing msg statuses to use Batcher like channel logs

v8.1.25 (2023-07-03)
-------------------------
 * Rework log committer to use new generic Batcher type which will be easier to rework to support S3 logs as well
 * Support requesting attachments for Twilio with basic auth

v8.1.24 (2023-06-28)
-------------------------
 * Update README

v8.1.23 (2023-06-19)
-------------------------
 * Support Dialog360 Cloud API channels

v8.1.22 (2023-06-05)
-------------------------
 * Stop writing ChannelLog.msg

v8.1.21 (2023-05-25)
-------------------------
 * Use Basic auth for fetching BW media attachments

v8.1.20 (2023-05-25)
-------------------------
 * Save received MO media for BW channels

v8.1.19 (2023-05-24)
-------------------------
 * Use max_length from config for external channels

v8.1.18 (2023-04-20)
-------------------------
 * Change default for FBA channels to messaging_type=UPDATE

v8.1.17 (2023-04-20)
-------------------------
 * Use origin and contact last seen on to determine message type and tag for FBA channels
 * Use postgres and redis as services in github actions

v8.1.16 (2023-04-18)
-------------------------
 * Update github actions versions
 * Ignore dates for hormund mo as they're not reliable or accurate
 * Remove chikka handler (company no longer exists)

v8.1.15 (2023-04-17)
-------------------------
 * Remove JSON tags for msg fields not used in sending

v8.1.14 (2023-04-17)
-------------------------
 * Address the expired status for MTN msgs
 * Update test database credentials for consistency with other projects

v8.1.13 (2023-03-29)
-------------------------
 * Fetch attachments endpoint should return connection errors as unavailable attachments

v8.1.12 (2023-03-24)
-------------------------
 * Fix MTN status report payload

v8.1.11 (2023-03-22)
-------------------------
 * MTN status reports are sent to the MO callback URL

v8.1.10 (2023-03-20)
-------------------------
 * Add way to customize the API host for MTN channels in channel config
 * Convert more handlers to use JSONPayload wrapper

v8.1.9 (2023-03-17)
-------------------------
 * Fix config for MTN channel for getting token and setting expiration
 * Add generic JSON handler wrapper that takes care of decoding and validaing incoming JSON payloads

v8.1.8 (2023-03-16)
-------------------------
 * Add support for MTN Developer Portal channel

v8.1.7 (2023-03-14)
-------------------------
 * Remove recipient_id requirements on statuses part of payload
 * Reduce time allowed for attachment requests so that we return before server cancels us
 * Test with org and channel configs as non-null JSONB columns

v8.1.6 (2023-03-08)
-------------------------
 * Create messages with msg_type=T
 * Bump golang.org/x/net from 0.2.0 to 0.7.0
 * Switch to gocommon/uuids for UUID types

v8.1.5 (2023-02-02)
-------------------------
 * Read quick_replies from msg field instead of inside metadata

v8.1.4 (2023-02-01)
-------------------------
 * Update to v2 of nyaruka/null

v8.1.3 (2023-01-31)
-------------------------
 * Add support for localizing Menu header on facebook list messages

v8.1.2 (2023-01-30)
-------------------------
 * Use Msg.locale with on-prem WhatsApp channels too

v8.1.1 (2023-01-30)
-------------------------
 * Support reading msg locale and use that instead of language+country on templating

v8.1.0 (2023-01-23)
-------------------------
 * Remove passing the parameters as null for WA template components

v8.0.2 (2023-01-11)
-------------------------
 * Fix for BW handler not being loaded 

v8.0.0 (2023-01-09)
-------------------------
 * Fix typos in README

v7.5.66 (2023-01-09)
-------------------------
 * Support bandwidth channel type

v7.5.65 (2023-01-05)
-------------------------
 * Enable back Arabiacell SSL validation

v7.5.64 (2022-12-13)
-------------------------
 * Remove temp workaround to stop D360 channels taking longer than 5 seconds to request attachments

v7.5.63 (2022-11-30)
-------------------------
 * Add logs for Facebook, Instagram, Viber, Telgram and Line specific errors
 * Update deps

v7.5.62 (2022-11-21)
-------------------------
 * Rework channel log errors to have separate code and ext_code fields to remove the need for namespaces
 * Add logs for WhatsApp Cloud specific errors

v7.5.61 (2022-11-18)
-------------------------
 * Ensure that URN and contact name are valid utf8 before trying to write to DB
 * Update to latest gocommon which provides dbutil.ToValidUTF8
 * Resolve error codes to messages for Twilio and Vonage and log errors for Twilio DLRs
 * Don't add returned err to channel log if it has logged errors already

v7.5.60
----------
 * Allow msg id to be passed to fetch attachment requests and saved on the channel log
 * Update attachment fetching to handle non-200 response as an unavailable attachment

v7.5.59
----------
 * Fix returning non-nil courier.Channel for deleted channels

v7.5.58
----------
 * Set server idle timeout to 90 seconds
 * Test against redis 6.2 and postgres 14

v7.5.57
----------
 * Update to latest gocommon

v7.5.56
----------
 * Allow empty attachments, e.g. a txt file

v7.5.55
----------
 * Update deps including phonenumbers

v7.5.54
----------
 * Fetch access tokens for WeChat, JioChat channels as needed

v7.5.53
----------
 * Use redisx.IntervalHash for message de-duping checks

v7.5.52
----------
 * Add support for JustCall channel type

v7.5.51
----------
 * Don't try to download WA attachments with no mediaID
 * Add WAC interactive message support with attachments, quick replies and captions.

v7.5.50
----------
 * Fix recording overall time of an attachment-fetch channel log
 * Remove no longer used channel_uuid and channel_type fields from msg event payload queued to mailroom
 * Update to latest gocommon

v7.5.49
----------
 * Stop fetching attachments and let message handling service do that via endpoint

v7.5.48
----------
 * Fix handling empty and non-200 responses from attachment fetches

v7.5.47
----------
 * Fix handling of geo attachments

v7.5.46
----------
 * Rework attachment fetching to keep URL and content type separate

v7.5.45
----------
 * Basic auth on status endpoint should be optional

v7.5.44
----------
 * Add endpoint to download and store attachments by their URL

v7.5.43
----------
 * Update dependencies
 * Skip SSL verification for AC channels
 * Fix channel log type token_refresh

v7.5.42
----------
 * Add channel UUID and type to queued msg events
 * More jsonx.MustMarshal

v7.5.41
----------
 * Customize the http client for D3 attachment fetches to have a timeout of 3 secs

v7.5.40
----------
 * Always return 200 status for all WA webhook requests
 * Remove temporary logging

v7.5.39
----------
 * Tweak large attachment logging

v7.5.38
----------
 * Tweak large attachment logging

v7.5.37
----------
 * Temp logging for large files

v7.5.36
----------
 * Update to latest gocommon and remove previous temp logging

v7.5.35
----------
 * More logging for large attachment downloads

v7.5.34
----------
 * Tweak error message

v7.5.33
----------
 * Add more detail to error message from S3 put
 * Update deps

v7.5.32
----------
 * Include requests to download attachments on the channel log for the incoming message
 * Add support for better channel error reporting

v7.5.31
----------
 * Allow twiml channels to send multiple media urls per message

v7.5.30
----------
 * Update msg status updating to allow skipping WIRED state

v7.5.29
----------
 * Simplify constructing responses and add tests
 * Make it easier to override responses per handler

v7.5.28
----------
 * Update to use SHA256 signature for FBA payload, increase max body bytes limit to 1MiB
 * Meta channels webhooks requests, should always return 200 status

v7.5.27
----------
 * Fix server logging when channel is nil

v7.5.26
----------
 * Fix junebug redaction values
 * Fix redaction on sends and add redaction of error messages

v7.5.25
----------
 * Adjust logging for WAC missing channel
 * Update to latest gocommon
 * Implement redaction of channel logs

v7.5.24
----------
 * Adjust to use the cache by address correctly
 * Rework handler tests to assert more state by default
 * Remove duplicate status writes
 * Append channel log UUIDs on status writes
 * Set log UUID on incoming messages and channel events
 * Use go 1.19
 * Fix some linter warnings

v7.5.23
----------
 * Support channels receiving embedded attachments and use with thinq handler

v7.5.22
----------
 * Save channel logs with UUID

v7.5.21
----------
 * Add codecov token to ci.yml
 * Add WAC support for sending captioned attachments
 * Cleanup tests
 * Include requests made by DescribeURN methods in the channel log for a receive

v7.5.20
----------
 * Fix writing errors to channel logs

v7.5.19
----------
 * Update to last gocommon
 * Fix local timezone dependent test
 * Don't fail CI for codecov problems
 * Add UUID to channel logs
 * Replace remaining usages of MakeHTTPRequest

v7.5.18
----------
 * Fix insert channel log SQL

v7.5.17
----------
 * Fix writing channel logs

v7.5.16
----------
 * Write channel logs in new format

v7.5.15
----------
 * Use logger for handler func calls

v7.5.14
----------
 * Update to latest gocommon and use new recorder reconstruct option

v7.5.13
----------
 * Use httpx.Recorder to generate traces of incoming requests
 * Rework WhatsApp handler to use logger, remove code for storing logs on status objects

v7.5.12
----------
 * Adjust LINE to support sending attachments with quick replies later

v7.5.11
----------
 * Rework more channel types to pass back traces and errors via logger instead of on status object

v7.5.10
----------
 * Update to latest gocommon and fix some go warnings
 * Support media attachments for LINE
 * Rework handler DescribeURN methods to take a channel logger
 * Update more sending to use channel logger

v7.5.9
----------
 * Rename S3MediaBucket to S3AttachmentsBucket and S3MediaPrefix to S3AttachmentsPrefix
 * More handlers to use new HTTP functions

v7.5.8
----------
 * Move testing code out of courier package and into new test package
 * Rework some handler sending to record logs via a logger rather than on the status object

v7.5.7
----------
 * Convert remaining channel types to use httpx.Trace

v7.5.6
----------
 * Fix URLs from non-resolved attachments that may not be properly escaped
 * Use httpx.DoTrace for some channels
 * Convert telegram handler to use ResolveAttachments
 * Add support for resolving media on the backend

v7.5.5
----------
 * Switch to using null.Map instead of utils.NullMap

v7.5.4
----------
 * Add AWS Cred Chain support for S3
 * Update deps and fix incorrect errors import in some handler packages

v7.5.3
----------
 * Fix receiving attachments in WAC

v7.5.2
----------
 * Support receiving LINE attachments

v7.5.1
----------
 * Support Quick replies for LINE channels
 * Slack channel support

v7.5.0
----------
 * Fix receiving quick replies and list replies in WAC
 * Add link preview support in WAC

v7.4.0
----------
 * Update README
 * Use analytics package from gocommon

v7.3.10
----------
 * Make sure text are sent after audio attachments for WA channels

v7.3.9
----------
 * Add arm64 as a build target
 * Add support for WA Cloud API
 * Refactor FBA tests

v7.3.8
----------
 * Add log to status first when handling telegram opt outs

v7.3.7
----------
 * Fix to not stop contact for other errors

v7.3.6
----------
 * Update to go 1.18 and latest gocommon/phonenumbers/jsonparser

v7.3.5
----------
 * Update Start Mobile send URL

v7.3.4
----------
 * Update WhatsApp handler so that we update the URN if the returned ID doesn't match
 * Stop Telegram contact that have blocked the channel bot

v7.3.3
----------
 * Quick fix to stop JSON content being omitted in logs

v7.3.2
----------
 * Update to latest gocommon and start using httpx.DetectContentType
 * Add link preview attribute for sending whatsapp
 * Update golang.org/x/sys

v7.3.1
----------
 * Fix handling stops via status callbacks on Twilio

v7.3.0
----------
 * Support stopping contacts when we get stop events on status callbacks

v7.2.0
----------
 * CI testing with go 1.17.7

v7.1.19
----------
 * Update D3 handler to support check for whatsapp contact not in contact store

v7.1.18
----------
 * Fix type for IsDeleted field for IG unsend events
 * Fix metadata fetching for new Facebook contacts

v7.1.17
----------
 * Fix whatsapp uploaded attachment file name
 * Use deleted by sender visibity for message unsent on IG channels
 * Add missing languages from whatsapp template
 * Do not save any message when receiving IG story mentions

v7.1.16
----------
 * Update to latest gocommon
 * Pause WA channel bulk queue when we hit the spam rate limit

v7.1.15
----------
 * Fix Gujarati whatsapp language code
 * Send flow name as user_data to HX

v7.1.14
----------
 * Allow more active redis connections
 * Support sending WA quick replies when we have attachments too
 * Add support to receive button text from Twilio WhatsApp

v7.1.13
----------
 * Send db and redis stats to librato in backed heartbeat
 * Include session_status in FCM payloads

v7.1.12
----------
 * Update to latest gocommon
 * Add instagram handler

v7.1.11
----------
 * More bulk sql tweaks

v7.1.10
----------
 * Update to latest gocommon

v7.1.9
----------
 * Fix bulk status updates

v7.1.8
----------
 * Do more error wrapping when creating contacts and URNs

v7.1.7
----------
 * Use dbutil package from gocommon
 * Add quick replies for vk

v7.1.6
----------
 * Throttle WA queues when we get 429 responses

v7.1.5
----------
 * Add Msg.failed_reason and set when msg fails due to reaching error limit

v7.1.4
----------
 * Remove loop detection now that mailroom does this
 * Smarter organization of quick replies for viber keyboards

v7.1.3
----------
 * Use response_to_external_id instead of response_to_id

v7.1.2
----------
 * External channel handler should use headers config setting if provided

v7.1.1
----------
 * Pin to go 1.17.2

v7.1.0
----------
 * Remove chatbase support
 * Test with Redis 3.2.4
 * Add support for 'Expired' status in the AT handler

v7.0.0
----------
 * Tweak README

v6.5.9
----------
 * Fix Viber attachments
 * CI testing on PG12 and 13
 * Update to latest gocommon and go 1.17

v6.5.8
----------
 * Fix Facebook document attachment
 * Update to latest gocommon and phonenumbers

v6.5.7
----------
 * Fix to only set the quick replies keyboard for the last message
 * Update to latest gocommon

v6.5.6
----------
 * Fix FB signing checks by trimming prefix instead of stripping
 * Improve layout of Telegram keyboards

v6.5.5
----------
 * Send WhatsApp buttons and list buttons when supported (thanks Weni)

v6.5.4
----------
 * trim prefix instead of strip when comparing FB sigs

v6.5.3
----------
 * log body when calculating signatures, include expected and calculated

v6.5.2
----------
 * Add ticket_count column to contact and set to zero when creating new contacts

v6.5.1
----------
 * Give S3 storage test new context on startup
 * Make DBMsg.SentOn nullable

v6.5.0
----------
 * Always set sent_on for W/S/D statuses if not already set
 * Update to latest gocommon

v6.4.0
----------
 * 6.4.0 Release Candidate

v6.3.5
----------
 * up max request size to 1M

v6.3.4
----------
 * Include filename when sending WhatsApp attachments

v6.3.3
----------
 * Support using namespace from the template translation
 * Add is_resend to Msg payload to allow for resending messages manually

v6.3.2
----------
 * Do not verify the SSL certificate for Bongo Live

v6.3.1
----------
 * Update BL to remove UDH parameter and use HTTPS URL

v6.2.2
----------
 * Handle whatsapp URNs sent to Twiml handler without prefix
 * Add support for Zenvia SMS

v6.2.1
----------
 * Add support for Zenvia WhatsApp

v6.2.0
----------
 * Add handling for button whatsapp message type
 * Bump CI testing to PG 11 and 12
 * Add Kaleyra channel type
 * 6.2.0 RC

v6.1.7
----------
 * switch id to bigserial

v6.1.6
----------
 * Cache media upload failures localy for 15m

v6.1.5
----------
 * include header when sanitizing request/response

v6.1.4
----------
 * Cleanup of whatsapp media handling
 * Detect media type for uploading media

v6.1.3
----------
 * Better logging of error cases when uploading WhatsApp media

v6.1.2
----------
 * use url.parse to build media URL

v6.1.1
----------
 * Add TextIt WhatsApp channel type

v6.1.0
----------
 * Check and log errors when building URLs for sending

v6.0.0
----------
 * Update README

v5.7.12
----------
 * URN channel change only for channels with SEND role
 * Update to gocommon v1.6.1
 * Add RocketChat handler
 * Add discord handler

v5.7.11
----------
 * Cache media ids for WhatsApp attachments

v5.7.10
----------
 * Support receiving Multipart form data requests for EX channels

v5.7.9
----------
 * Update to latest gocommon 1.5.3 and golang 1.15
 * Add session status from mailroom to MT message sent to external channel API call
 * Remove incoming message prefix for Play Mobile free accounts

v5.7.8
----------
 * deal with empty message in FreshChat incoming requests

v5.7.7
----------
 * Update to gocommon v1.5.1

v5.7.6
----------
 * Remove dummy values for AWS config values so you can use local file system for testing
 * Use gsm7, storage, dates and uuids packages from gocommon

v5.7.5
----------
 * No longer write contact.is_stopped or is_blocked

v5.7.4
----------
 * Support receiving XML for CM channels
 * Write status on new contacts
 * Add support for Whatsapp 360dialog

v5.7.3
----------
 * Include created_on in msg_event
 * Include occurred_on when queueing channel events for mailroom

v5.7.2
----------
 * Deal with Shaqodoon not properly escaping + in from

v5.7.1
----------
 * Add ClickMobile channel type

v5.7.0
----------
 * Save the Ad ID for Facebook postback referral 

v5.6.0
----------
 * 5.6.0 Candidate Release

v5.5.28 
----------
 * Fix FBA signature validation and channel lookup

v5.5.27
----------
 * Add country field and support for more template languages on WhatsApp handler

v5.5.26
----------
 * Only log channel events when we have a channel matched
 * HX channel sends MO using ISO 8859-1 encoding

v5.5.25
----------
 * Load FBA channel handler package

v5.5.24
----------
 * Support loading channels with null address

v5.5.23
----------
 * Add support for FBA channel type

v5.5.22
----------
 * User reply endpoint when possible for LINE messages

v5.5.21
----------
 * Fix FB location attachment to be handled at geo attachment

v5.5.20
----------
 * TS expects national numbers only

v5.5.19
----------
 * Upgrade FB graph API to 3.3

v5.5.18
----------
 * TS sends should use mobile instead of from

v5.5.17
----------
 * Support sending document attachments for Telegram

v5.5.16
----------
 * Add option for Telesom Send URL
 * Ignore received message request in Telegram handler when a file cannot be resolved

v5.5.15
----------
 * Support using national number for EX channel if configured so

v5.5.14
----------
 * Add Telesom channel type support

v5.5.13
----------
 * Use Channel specific max_length config value if set

v5.5.12
----------
 * Increase ArabiaCell max length to 1530

v5.5.11
----------
 * Retry WhatsApp channel messaging after contact check with returned WhatsApp ID

v5.5.10
----------
 * Fix sending WA template messages on new WhatsApp docker

v5.5.9
----------
 * Add option for Kannel channels to ignore duplicative sent status

v5.5.8
----------
 * More tweaks to slowing down batching of status commits when approaching max queue size

v5.5.7
----------
 * slow queuing before reaching our max batch size

v5.5.6
----------
 * Slow queuing into a batch when batches are full

v5.5.5
----------
 * Increase buffer size
 * Add support for Viber stickers as image attachments for incoming messages

v5.5.4
----------
 * handle error cases for whatsapp callbacks

v5.5.3
----------
 * add native panic handling

v5.5.2
----------
 * Send msg in batches and add image msg type in the LINE channel

v5.5.1
----------
 * Add contacts not already present for WhatsApp when sending error detected (thanks @koallann)

v5.5.0
----------
 * add fabric to gitignore

v5.6.0
----------
 * add fabric to gitignore

v5.4.1
----------
 * Strip cookie from incoming requests

v5.4.0
----------
 * touch README for 5.4 release

v5.3.9
----------
 * Add VK Channel

v5.3.8
----------
 * Fix Chatbase request body

v5.3.7
----------
 * Fix quick replies variable replacement on external channel long msg

v5.3.6
----------
 * Allow configuring and sending of quick replies for external channels

v5.3.5
----------
 * Refactor FMC channel to support the fixed quick replies structure

v5.3.4
----------
 * Change Arabia Cell max length to 670, fixes #274
 * Add support for Twilio Whatsapp channel type
 * Convert to use Github actions for CI

v5.3.3
----------
 * Fix freshchat image handing

v5.3.2
----------
 * Set Facebook message type tag when topic is set on message

v5.3.1
----------
 * update changelog for v5.3

v5.3.0
----------
 * Send WhatsApp media via URL
 * Log Zenvia errors to ChannelLog instead of Sentry
 * Ignore status updates for incoming messages	

v5.2.0
----------
 * Sync version with RapidPro 5.2

v2.0.18
----------
 * Test matrix release

v2.0.17
----------
 * Test deploying with matrix build

v2.0.16
----------
 * test releasing only on pg10

v2.0.15
----------
 * Derive contact name for new WhatsApp contacts (thanks @devchima)

v2.0.14
----------
 * properly log connection errors for whatsapp

v2.0.13
----------
 * use latest librato library

v2.0.12
----------
 * tune HTTP transport settings

v2.0.11
----------
n
 * tune HTTPClient settings to better deal with slow hosts

v2.0.10
----------
 * Use multipart form encoding for thinQ

v2.0.9
----------
 * Add thinq handler

v2.0.8
----------
 * turn thumbs up stickers into thumbs up emoji

v2.0.7
----------
 * Tweak lua script for checking loops, add more tests

v2.0.6
----------
 * Make sure we never overflow our count when considering loops

v2.0.5
----------
 * Check whether outgoing message is in a loop before sending

v2.0.4
----------
 * Add FreshChat channel type
 * Latest phonenumbers library

v2.0.3
----------
 * Fix sending for ClickSend

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

