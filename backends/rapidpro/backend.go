package rapidpro

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/batch"
	"github.com/nyaruka/courier/queue"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/dbutil"
	"github.com/nyaruka/gocommon/storage"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/librato"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// the name for our message queue
const msgQueueName = "msgs"

// the name of our set for tracking sends
const sentSetName = "msgs_sent_%s"

// our timeout for backend operations
const backendTimeout = time.Second * 20

func init() {
	courier.RegisterBackend("rapidpro", newBackend)
}

// GetChannel returns the channel for the passed in type and UUID
func (b *backend) GetChannel(ctx context.Context, ct courier.ChannelType, uuid courier.ChannelUUID) (courier.Channel, error) {
	timeout, cancel := context.WithTimeout(ctx, backendTimeout)
	defer cancel()

	return getChannel(timeout, b.db, ct, uuid)
}

// GetChannelByAddress returns the channel with the passed in type and address
func (b *backend) GetChannelByAddress(ctx context.Context, ct courier.ChannelType, address courier.ChannelAddress) (courier.Channel, error) {
	timeout, cancel := context.WithTimeout(ctx, backendTimeout)
	defer cancel()

	return getChannelByAddress(timeout, b.db, ct, address)
}

// GetContact returns the contact for the passed in channel and URN
func (b *backend) GetContact(ctx context.Context, c courier.Channel, urn urns.URN, auth string, name string) (courier.Contact, error) {
	dbChannel := c.(*DBChannel)
	return contactForURN(ctx, b, dbChannel.OrgID_, dbChannel, urn, auth, name)
}

// AddURNtoContact adds a URN to the passed in contact
func (b *backend) AddURNtoContact(ctx context.Context, c courier.Channel, contact courier.Contact, urn urns.URN) (urns.URN, error) {
	tx, err := b.db.BeginTxx(ctx, nil)
	if err != nil {
		return urns.NilURN, err
	}
	dbChannel := c.(*DBChannel)
	dbContact := contact.(*DBContact)
	_, err = contactURNForURN(tx, dbChannel, dbContact.ID_, urn, "")
	if err != nil {
		return urns.NilURN, err
	}
	err = tx.Commit()
	if err != nil {
		return urns.NilURN, err
	}

	return urn, nil
}

const removeURNFromContact = `
UPDATE
	contacts_contacturn
SET
	contact_id = NULL
WHERE
	contact_id = $1 AND
	identity = $2
`

// RemoveURNFromcontact removes a URN from the passed in contact
func (b *backend) RemoveURNfromContact(ctx context.Context, c courier.Channel, contact courier.Contact, urn urns.URN) (urns.URN, error) {
	dbContact := contact.(*DBContact)
	_, err := b.db.ExecContext(ctx, removeURNFromContact, dbContact.ID_, urn.Identity().String())
	if err != nil {
		return urns.NilURN, err
	}
	return urn, nil
}

const updateMsgVisibilityDeletedBySender = `
UPDATE
	msgs_msg
SET
	visibility = 'X',
	text = '',
	attachments = '{}'
WHERE
	msgs_msg.id = (SELECT m."id" FROM "msgs_msg" m INNER JOIN "channels_channel" c ON (m."channel_id" = c."id") WHERE (c."uuid" = $1 AND m."external_id" = $2 AND m."direction" = 'I'))
RETURNING
	msgs_msg.id
`

// DeleteMsgWithExternalID delete a message we receive an event that it should be deleted
func (b *backend) DeleteMsgWithExternalID(ctx context.Context, channel courier.Channel, externalID string) error {
	_, err := b.db.ExecContext(ctx, updateMsgVisibilityDeletedBySender, string(channel.UUID().String()), externalID)
	if err != nil {
		return err
	}
	return nil
}

// NewIncomingMsg creates a new message from the given params
func (b *backend) NewIncomingMsg(channel courier.Channel, urn urns.URN, text string) courier.Msg {
	// remove any control characters
	text = utils.CleanString(text)

	// create our msg
	msg := newMsg(MsgIncoming, channel, urn, text)

	// set received on to now
	msg.WithReceivedOn(time.Now().UTC())

	// have we seen this msg in the past period?
	prevUUID := checkMsgSeen(b, msg)
	if prevUUID != courier.NilMsgUUID {
		// if so, use its UUID and that we've been written
		msg.UUID_ = prevUUID
		msg.alreadyWritten = true
	}
	return msg
}

// NewOutgoingMsg creates a new outgoing message from the given params
func (b *backend) NewOutgoingMsg(channel courier.Channel, urn urns.URN, text string) courier.Msg {
	return newMsg(MsgOutgoing, channel, urn, text)
}

// PopNextOutgoingMsg pops the next message that needs to be sent
func (b *backend) PopNextOutgoingMsg(ctx context.Context) (courier.Msg, error) {
	// pop the next message off our queue
	rc := b.redisPool.Get()
	defer rc.Close()

	token, msgJSON, err := queue.PopFromQueue(rc, msgQueueName)
	for token == queue.Retry {
		token, msgJSON, err = queue.PopFromQueue(rc, msgQueueName)
	}

	if msgJSON != "" {
		dbMsg := &DBMsg{}
		err = json.Unmarshal([]byte(msgJSON), dbMsg)
		if err != nil {
			queue.MarkComplete(rc, msgQueueName, token)
			return nil, fmt.Errorf("unable to unmarshal message '%s': %s", msgJSON, err)
		}
		// populate the channel on our db msg
		channel, err := b.GetChannel(ctx, courier.AnyChannelType, dbMsg.ChannelUUID_)
		if err != nil {
			queue.MarkComplete(rc, msgQueueName, token)
			return nil, err
		}
		dbMsg.channel = channel.(*DBChannel)
		dbMsg.workerToken = token

		// clear out our seen incoming messages
		clearMsgSeen(rc, dbMsg)

		return dbMsg, nil
	}

	return nil, nil
}

var luaSent = redis.NewScript(3,
	`-- KEYS: [TodayKey, YesterdayKey, MsgID]
     local found = redis.call("sismember", KEYS[1], KEYS[3])
     if found == 1 then
	   return 1
     end

     return redis.call("sismember", KEYS[2], KEYS[3])
`)

// WasMsgSent returns whether the passed in message has already been sent
func (b *backend) WasMsgSent(ctx context.Context, id courier.MsgID) (bool, error) {
	rc := b.redisPool.Get()
	defer rc.Close()

	todayKey := fmt.Sprintf(sentSetName, time.Now().UTC().Format("2006_01_02"))
	yesterdayKey := fmt.Sprintf(sentSetName, time.Now().Add(time.Hour*-24).UTC().Format("2006_01_02"))
	return redis.Bool(luaSent.Do(rc, todayKey, yesterdayKey, id.String()))
}

var luaClearSent = redis.NewScript(3,
	`-- KEYS: [TodayKey, YesterdayKey, MsgID]
	 redis.call("srem", KEYS[1], KEYS[3])
     redis.call("srem", KEYS[2], KEYS[3])
`)

func (b *backend) ClearMsgSent(ctx context.Context, id courier.MsgID) error {
	rc := b.redisPool.Get()
	defer rc.Close()

	todayKey := fmt.Sprintf(sentSetName, time.Now().UTC().Format("2006_01_02"))
	yesterdayKey := fmt.Sprintf(sentSetName, time.Now().Add(time.Hour*-24).UTC().Format("2006_01_02"))
	_, err := luaClearSent.Do(rc, todayKey, yesterdayKey, id.String())
	return err
}

// MarkOutgoingMsgComplete marks the passed in message as having completed processing, freeing up a worker for that channel
func (b *backend) MarkOutgoingMsgComplete(ctx context.Context, msg courier.Msg, status courier.MsgStatus) {
	rc := b.redisPool.Get()
	defer rc.Close()

	dbMsg := msg.(*DBMsg)

	queue.MarkComplete(rc, msgQueueName, dbMsg.workerToken)

	// mark as sent in redis as well if this was actually wired or sent
	if status != nil && (status.Status() == courier.MsgSent || status.Status() == courier.MsgWired) {
		dateKey := fmt.Sprintf(sentSetName, time.Now().UTC().Format("2006_01_02"))
		rc.Send("sadd", dateKey, msg.ID().String())
		rc.Send("expire", dateKey, 60*60*24*2)
		_, err := rc.Do("")
		if err != nil {
			logrus.WithError(err).WithField("sent_msgs_key", dateKey).Error("unable to add new unsent message")
		}

		// if our msg has an associated session and timeout, update that
		if dbMsg.SessionWaitStartedOn_ != nil {
			err = updateSessionTimeout(ctx, b, dbMsg.SessionID_, *dbMsg.SessionWaitStartedOn_, dbMsg.SessionTimeout_)
			if err != nil {
				logrus.WithError(err).WithField("session_id", dbMsg.SessionID_).Error("unable to update session timeout")
			}
		}
	}
}

// WriteMsg writes the passed in message to our store
func (b *backend) WriteMsg(ctx context.Context, m courier.Msg) error {
	timeout, cancel := context.WithTimeout(ctx, backendTimeout)
	defer cancel()

	return writeMsg(timeout, b, m)
}

// NewStatusUpdateForID creates a new Status object for the given message id
func (b *backend) NewMsgStatusForID(channel courier.Channel, id courier.MsgID, status courier.MsgStatusValue) courier.MsgStatus {
	return newMsgStatus(channel, id, "", status)
}

// NewStatusUpdateForID creates a new Status object for the given message id
func (b *backend) NewMsgStatusForExternalID(channel courier.Channel, externalID string, status courier.MsgStatusValue) courier.MsgStatus {
	return newMsgStatus(channel, courier.NilMsgID, externalID, status)
}

// WriteMsgStatus writes the passed in MsgStatus to our store
func (b *backend) WriteMsgStatus(ctx context.Context, status courier.MsgStatus) error {
	timeout, cancel := context.WithTimeout(ctx, backendTimeout)
	defer cancel()

	if status.HasUpdatedURN() {
		err := b.updateContactURN(ctx, status)
		if err != nil {
			return errors.Wrap(err, "error updating contact URN")
		}
	}
	// if we have an ID, we can have our batch commit for us
	if status.ID() != courier.NilMsgID {
		b.statusCommitter.Queue(status.(*DBMsgStatus))
	} else {
		// otherwise, write normally (synchronously)
		err := writeMsgStatus(timeout, b, status)
		if err != nil {
			return err
		}
	}

	// if we have an id and are marking an outgoing msg as errored, then clear our sent flag
	if status.ID() != courier.NilMsgID && status.Status() == courier.MsgErrored {
		err := b.ClearMsgSent(ctx, status.ID())
		if err != nil {
			logrus.WithError(err).WithField("msg", status.ID().String()).Error("error clearing sent flags")
		}
	}

	return nil
}

// updateContactURN updates contact URN according to the old/new URNs from status
func (b *backend) updateContactURN(ctx context.Context, status courier.MsgStatus) error {
	old, new := status.UpdatedURN()

	// retrieve channel
	channel, err := b.GetChannel(ctx, courier.AnyChannelType, status.ChannelUUID())
	if err != nil {
		return errors.Wrap(err, "error retrieving channel")
	}
	dbChannel := channel.(*DBChannel)
	tx, err := b.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	// retrieve the old URN
	oldContactURN, err := selectContactURN(tx, dbChannel.OrgID(), old)
	if err != nil {
		return errors.Wrap(err, "error retrieving old contact URN")
	}
	// retrieve the new URN
	newContactURN, err := selectContactURN(tx, dbChannel.OrgID(), new)
	if err != nil {
		// only update the old URN path if the new URN doesn't exist
		if err == sql.ErrNoRows {
			oldContactURN.Path = new.Path()
			oldContactURN.Identity = string(new.Identity())

			err = fullyUpdateContactURN(tx, oldContactURN)
			if err != nil {
				tx.Rollback()
				return errors.Wrap(err, "error updating old contact URN")
			}
			return tx.Commit()
		}
		return errors.Wrap(err, "error retrieving new contact URN")
	}

	// only update the new URN if it doesn't have an associated contact
	if newContactURN.ContactID == NilContactID {
		newContactURN.ContactID = oldContactURN.ContactID
	}
	// remove contact association from old URN
	oldContactURN.ContactID = NilContactID

	// update URNs
	err = fullyUpdateContactURN(tx, newContactURN)
	if err != nil {
		tx.Rollback()
		return errors.Wrap(err, "error updating new contact URN")
	}
	err = fullyUpdateContactURN(tx, oldContactURN)
	if err != nil {
		tx.Rollback()
		return errors.Wrap(err, "error updating old contact URN")
	}
	return tx.Commit()
}

// NewChannelEvent creates a new channel event with the passed in parameters
func (b *backend) NewChannelEvent(channel courier.Channel, eventType courier.ChannelEventType, urn urns.URN) courier.ChannelEvent {
	return newChannelEvent(channel, eventType, urn)
}

// WriteChannelEvent writes the passed in channel even returning any error
func (b *backend) WriteChannelEvent(ctx context.Context, event courier.ChannelEvent) error {
	timeout, cancel := context.WithTimeout(ctx, backendTimeout)
	defer cancel()

	return writeChannelEvent(timeout, b, event)
}

// WriteChannelLogs persists the passed in logs to our database, for rapidpro we swallow all errors, logging isn't critical
func (b *backend) WriteChannelLogs(ctx context.Context, logs []*courier.ChannelLog) error {
	timeout, cancel := context.WithTimeout(ctx, backendTimeout)
	defer cancel()

	for _, l := range logs {
		err := writeChannelLog(timeout, b, l)
		if err != nil {
			logrus.WithError(err).Error("error writing channel log")
		}
	}
	return nil
}

// Check if external ID has been seen in a period
func (b *backend) CheckExternalIDSeen(msg courier.Msg) courier.Msg {
	var prevUUID = checkExternalIDSeen(b, msg)
	m := msg.(*DBMsg)
	if prevUUID != courier.NilMsgUUID {
		// if so, use its UUID and that we've been written
		m.UUID_ = prevUUID
		m.alreadyWritten = true
	}
	return m
}

// Mark a external ID as seen for a period
func (b *backend) WriteExternalIDSeen(msg courier.Msg) {
	writeExternalIDSeen(b, msg)
}

// Health returns the health of this backend as a string, returning "" if all is well
func (b *backend) Health() string {
	// test redis
	rc := b.redisPool.Get()
	defer rc.Close()
	_, redisErr := rc.Do("PING")

	// test our db
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	dbErr := b.db.PingContext(ctx)
	cancel()

	health := bytes.Buffer{}
	if redisErr != nil {
		health.WriteString(fmt.Sprintf("\n% 16s: %v", "redis err", redisErr))
	}
	if dbErr != nil {
		health.WriteString(fmt.Sprintf("\n% 16s: %v", "db err", dbErr))
	}

	return health.String()
}

// Heartbeat is called every minute, we log our queue depth to librato
func (b *backend) Heartbeat() error {
	rc := b.redisPool.Get()
	defer rc.Close()

	active, err := redis.Strings(rc.Do("ZRANGE", fmt.Sprintf("%s:active", msgQueueName), "0", "-1"))
	if err != nil {
		return errors.Wrapf(err, "error getting active queues")
	}
	throttled, err := redis.Strings(rc.Do("ZRANGE", fmt.Sprintf("%s:throttled", msgQueueName), "0", "-1"))
	if err != nil {
		return errors.Wrapf(err, "error getting throttled queues")
	}
	queues := append(active, throttled...)

	prioritySize := 0
	bulkSize := 0
	for _, queue := range queues {
		q := fmt.Sprintf("%s/1", queue)
		count, err := redis.Int(rc.Do("ZCARD", q))
		if err != nil {
			return errors.Wrapf(err, "error getting size of priority queue: %s", q)
		}
		prioritySize += count

		q = fmt.Sprintf("%s/0", queue)
		count, err = redis.Int(rc.Do("ZCARD", q))
		if err != nil {
			return errors.Wrapf(err, "error getting size of bulk queue: %s", q)
		}
		bulkSize += count
	}

	// get our DB and redis stats
	dbStats := b.db.Stats()
	redisStats := b.redisPool.Stats()

	dbWaitDurationInPeriod := dbStats.WaitDuration - b.dbWaitDuration
	dbWaitCountInPeriod := dbStats.WaitCount - b.dbWaitCount
	redisWaitDurationInPeriod := redisStats.WaitDuration - b.redisWaitDuration
	redisWaitCountInPeriod := redisStats.WaitCount - b.redisWaitCount

	b.dbWaitDuration = dbStats.WaitDuration
	b.dbWaitCount = dbStats.WaitCount
	b.redisWaitDuration = redisStats.WaitDuration
	b.redisWaitCount = redisStats.WaitCount

	librato.Gauge("courier.db_busy", float64(dbStats.InUse))
	librato.Gauge("courier.db_idle", float64(dbStats.Idle))
	librato.Gauge("courier.db_wait_ms", float64(dbWaitDurationInPeriod/time.Millisecond))
	librato.Gauge("courier.db_wait_count", float64(dbWaitCountInPeriod))
	librato.Gauge("courier.redis_wait_ms", float64(redisWaitDurationInPeriod/time.Millisecond))
	librato.Gauge("courier.redis_wait_count", float64(redisWaitCountInPeriod))
	librato.Gauge("courier.bulk_queue", float64(bulkSize))
	librato.Gauge("courier.priority_queue", float64(prioritySize))

	logrus.WithFields(logrus.Fields{
		"db_busy":          dbStats.InUse,
		"db_idle":          dbStats.Idle,
		"db_wait_time":     dbWaitDurationInPeriod,
		"db_wait_count":    dbWaitCountInPeriod,
		"redis_wait_time":  dbWaitDurationInPeriod,
		"redis_wait_count": dbWaitCountInPeriod,
		"priority_size":    prioritySize,
		"bulk_size":        bulkSize,
	}).Info("current analytics")

	return nil
}

// Status returns information on our queue sizes, number of workers etc..
func (b *backend) Status() string {
	rc := b.redisPool.Get()
	defer rc.Close()

	status := bytes.Buffer{}
	status.WriteString("------------------------------------------------------------------------------------\n")
	status.WriteString("     Size | Bulk Size | Workers | TPS | Type | Channel              \n")
	status.WriteString("------------------------------------------------------------------------------------\n")

	var queue string
	var workers float64

	// get all our queues
	rc.Send("zrevrangebyscore", fmt.Sprintf("%s:active", msgQueueName), "+inf", "-inf", "withscores")
	rc.Send("zrevrangebyscore", fmt.Sprintf("%s:throttled", msgQueueName), "+inf", "-inf", "withscores")
	rc.Flush()

	active, err := redis.Values(rc.Receive())
	if err != nil {
		return fmt.Sprintf("unable to read active queues: %v", err)
	}
	throttled, err := redis.Values(rc.Receive())
	if err != nil {
		return fmt.Sprintf("unable to read throttled queues: %v", err)
	}
	values := append(active, throttled...)

	for len(values) > 0 {
		values, err = redis.Scan(values, &queue, &workers)
		if err != nil {
			return fmt.Sprintf("error reading active queues: %v", err)
		}

		// our queue name is in the format msgs:uuid|tps, break it apart
		queue = strings.TrimPrefix(queue, "msgs:")
		parts := strings.Split(queue, "|")
		if len(parts) != 2 {
			return fmt.Sprintf("error parsing queue name '%s'", queue)
		}
		uuid := parts[0]
		tps := parts[1]

		// try to look up our channel
		channelUUID, _ := courier.NewChannelUUID(uuid)
		channel, err := getChannel(context.Background(), b.db, courier.AnyChannelType, channelUUID)
		channelType := "!!"
		if err == nil {
			channelType = channel.ChannelType().String()
		}

		// get # of items in our normal queue
		size, err := redis.Int64(rc.Do("ZCARD", fmt.Sprintf("%s:%s/1", msgQueueName, queue)))
		if err != nil {
			return fmt.Sprintf("error reading queue size: %v", err)
		}

		// get # of items in the bulk queue
		bulkSize, err := redis.Int64(rc.Do("ZCARD", fmt.Sprintf("%s:%s/0", msgQueueName, queue)))
		if err != nil {
			return fmt.Sprintf("error reading bulk queue size: %v", err)
		}

		status.WriteString(fmt.Sprintf("% 9d   % 9d   % 7d   % 3s   % 4s   %s\n", size, bulkSize, int(workers), tps, channelType, uuid))
	}

	return status.String()
}

// Start starts our RapidPro backend, this tests our various connections and starts our spool flushers
func (b *backend) Start() error {
	// parse and test our redis config
	log := logrus.WithFields(logrus.Fields{
		"comp":  "backend",
		"state": "starting",
	})
	log.Info("starting backend")

	// parse and test our db config
	dbURL, err := url.Parse(b.config.DB)
	if err != nil {
		return fmt.Errorf("unable to parse DB URL '%s': %s", b.config.DB, err)
	}

	if dbURL.Scheme != "postgres" {
		return fmt.Errorf("invalid DB URL: '%s', only postgres is supported", b.config.DB)
	}

	// build our db
	db, err := sqlx.Open("postgres", b.config.DB)
	if err != nil {
		return fmt.Errorf("unable to open DB with config: '%s': %s", b.config.DB, err)
	}

	// configure our pool
	b.db = db
	b.db.SetMaxIdleConns(4)
	b.db.SetMaxOpenConns(16)

	// try connecting
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	err = b.db.PingContext(ctx)
	cancel()
	if err != nil {
		log.WithError(err).Error("db not reachable")
	} else {
		log.Info("db ok")
	}

	// parse and test our redis config
	redisURL, err := url.Parse(b.config.Redis)
	if err != nil {
		return fmt.Errorf("unable to parse Redis URL '%s': %s", b.config.Redis, err)
	}

	// create our pool
	redisPool := &redis.Pool{
		Wait:        true,              // makes callers wait for a connection
		MaxActive:   36,                // only open this many concurrent connections at once
		MaxIdle:     4,                 // only keep up to this many idle
		IdleTimeout: 240 * time.Second, // how long to wait before reaping a connection
		Dial: func() (redis.Conn, error) {
			conn, err := redis.Dial("tcp", redisURL.Host)
			if err != nil {
				return nil, err
			}

			// send auth if required
			if redisURL.User != nil {
				pass, authRequired := redisURL.User.Password()
				if authRequired {
					if _, err := conn.Do("AUTH", pass); err != nil {
						conn.Close()
						return nil, err
					}
				}
			}

			// switch to the right DB
			_, err = conn.Do("SELECT", strings.TrimLeft(redisURL.Path, "/"))
			return conn, err
		},
	}
	b.redisPool = redisPool

	// test our redis connection
	conn := redisPool.Get()
	defer conn.Close()
	_, err = conn.Do("PING")
	if err != nil {
		log.WithError(err).Error("redis not reachable")
	} else {
		log.Info("redis ok")
	}

	// start our dethrottler if we are going to be doing some sending
	if b.config.MaxWorkers > 0 {
		queue.StartDethrottler(redisPool, b.stopChan, b.waitGroup, msgQueueName)
	}

	// create our storage (S3 or file system)
	if b.config.AWSAccessKeyID != "" {
		s3Client, err := storage.NewS3Client(&storage.S3Options{
			AWSAccessKeyID:     b.config.AWSAccessKeyID,
			AWSSecretAccessKey: b.config.AWSSecretAccessKey,
			Endpoint:           b.config.S3Endpoint,
			Region:             b.config.S3Region,
			DisableSSL:         b.config.S3DisableSSL,
			ForcePathStyle:     b.config.S3ForcePathStyle,
			MaxRetries:         3,
		})
		if err != nil {
			return err
		}
		b.storage = storage.NewS3(s3Client, b.config.S3MediaBucket, b.config.S3Region, 32)
	} else {
		b.storage = storage.NewFS("_storage")
	}

	// test our storage
	ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	err = b.storage.Test(ctx)
	cancel()
	if err != nil {
		log.WithError(err).Error(b.storage.Name() + " storage not available")
	} else {
		log.Info(b.storage.Name() + " storage ok")
	}

	// make sure our spool dirs are writable
	err = courier.EnsureSpoolDirPresent(b.config.SpoolDir, "msgs")
	if err == nil {
		err = courier.EnsureSpoolDirPresent(b.config.SpoolDir, "statuses")
	}
	if err == nil {
		err = courier.EnsureSpoolDirPresent(b.config.SpoolDir, "events")
	}
	if err != nil {
		log.WithError(err).Error("spool directories not writable")
	} else {
		log.Info("spool directories ok")
	}

	// create our status committer and start it
	b.statusCommitter = batch.NewCommitter("status committer", b.db, bulkUpdateMsgStatusSQL, time.Millisecond*500, b.committerWG,
		func(err error, value batch.Value) {
			log := logrus.WithField("comp", "status committer")

			if qerr := dbutil.AsQueryError(err); qerr != nil {
				query, params := qerr.Query()
				log = log.WithFields(logrus.Fields{"sql": query, "sql_params": params})
			}

			log.WithError(err).Error("error writing status")

			err = courier.WriteToSpool(b.config.SpoolDir, "statuses", value)
			if err != nil {
				logrus.WithField("comp", "status committer").WithError(err).Error("error writing status to spool")
			}
		})
	b.statusCommitter.Start()

	// create our log committer and start it
	b.logCommitter = batch.NewCommitter("log committer", b.db, insertLogSQL, time.Millisecond*500, b.committerWG,
		func(err error, value batch.Value) {
			log := logrus.WithField("comp", "log committer")

			if qerr := dbutil.AsQueryError(err); qerr != nil {
				query, params := qerr.Query()
				log = log.WithFields(logrus.Fields{"sql": query, "sql_params": params})
			}

			log.WithError(err).Error("error writing channel log")
		})
	b.logCommitter.Start()

	// register and start our spool flushers
	courier.RegisterFlusher(path.Join(b.config.SpoolDir, "msgs"), b.flushMsgFile)
	courier.RegisterFlusher(path.Join(b.config.SpoolDir, "statuses"), b.flushStatusFile)
	courier.RegisterFlusher(path.Join(b.config.SpoolDir, "events"), b.flushChannelEventFile)

	logrus.WithFields(logrus.Fields{
		"comp":  "backend",
		"state": "started",
	}).Info("backend started")

	return nil
}

// Stop stops our RapidPro backend, closing our db and redis connections
func (b *backend) Stop() error {
	// close our stop channel
	close(b.stopChan)

	// wait for our threads to exit
	b.waitGroup.Wait()
	return nil
}

func (b *backend) Cleanup() error {
	// stop our status committer
	if b.statusCommitter != nil {
		b.statusCommitter.Stop()
	}

	// stop our log committer
	if b.logCommitter != nil {
		b.logCommitter.Stop()
	}

	// wait for them to flush fully
	b.committerWG.Wait()

	// close our db and redis pool
	if b.db != nil {
		b.db.Close()
	}
	return b.redisPool.Close()
}

// RedisPool returns the redisPool for this backend
func (b *backend) RedisPool() *redis.Pool {
	return b.redisPool
}

// NewBackend creates a new RapidPro backend
func newBackend(config *courier.Config) courier.Backend {
	return &backend{
		config: config,

		stopChan:  make(chan bool),
		waitGroup: &sync.WaitGroup{},

		committerWG: &sync.WaitGroup{},
	}
}

type backend struct {
	config *courier.Config

	statusCommitter batch.Committer
	logCommitter    batch.Committer
	committerWG     *sync.WaitGroup

	db        *sqlx.DB
	redisPool *redis.Pool
	storage   storage.Storage

	stopChan  chan bool
	waitGroup *sync.WaitGroup

	// both sqlx and redis provide wait stats which are cummulative that we need to convert into increments
	dbWaitDuration    time.Duration
	dbWaitCount       int64
	redisWaitDuration time.Duration
	redisWaitCount    int64
}
