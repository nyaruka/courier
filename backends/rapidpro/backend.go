package rapidpro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/garyburd/redigo/redis"
	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/config"
	"github.com/nyaruka/courier/queue"
	"github.com/nyaruka/courier/utils"
	"github.com/sirupsen/logrus"
)

// the name for our message queue
const msgQueueName = "msgs"

// the name of our set for tracking sends
const sentSetName = "msgs_sent_%s"

func init() {
	courier.RegisterBackend("rapidpro", newBackend)
}

// GetChannel returns the channel for the passed in type and UUID
func (b *backend) GetChannel(ct courier.ChannelType, uuid courier.ChannelUUID) (courier.Channel, error) {
	return getChannel(b, ct, uuid)
}

// NewIncomingMsg creates a new message from the given params
func (b *backend) NewIncomingMsg(channel courier.Channel, urn courier.URN, text string) courier.Msg {
	// remove any control characters
	text = utils.CleanString(text)

	// create our msg
	msg := newMsg(MsgIncoming, channel, urn, text)

	// have we seen this msg in the past period?
	prevUUID := checkMsgSeen(b, msg)
	if prevUUID != courier.NilMsgUUID {
		// if so, use its UUID and that we've been written
		msg.UUID_ = prevUUID
		msg.AlreadyWritten_ = true
	}
	return msg
}

// NewOutgoingMsg creates a new outgoing message from the given params
func (b *backend) NewOutgoingMsg(channel courier.Channel, urn courier.URN, text string) courier.Msg {
	return newMsg(MsgOutgoing, channel, urn, text)
}

// PopNextOutgoingMsg pops the next message that needs to be sent
func (b *backend) PopNextOutgoingMsg() (courier.Msg, error) {
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
			return nil, err
		}

		// populate the channel on our db msg
		channel, err := b.GetChannel(courier.AnyChannelType, dbMsg.ChannelUUID_)
		if err != nil {
			return nil, err
		}
		dbMsg.Channel_ = channel
		dbMsg.WorkerToken_ = token
		return dbMsg, nil
	}

	return nil, nil
}

var luaSent = redis.NewScript(3,
	`-- KEYS: [TodayKey, YesterdayKey, MsgId]
     local found = redis.call("sismember", KEYS[1], KEYS[3])
     if found == 1 then
	   return 1
     end

     return redis.call("sismember", KEYS[2], KEYS[3])
`)

// WasMsgSent returns whether the passed in message has already been sent
func (b *backend) WasMsgSent(msg courier.Msg) (bool, error) {
	rc := b.redisPool.Get()
	defer rc.Close()

	todayKey := fmt.Sprintf(sentSetName, time.Now().UTC().Format("2006_01_02"))
	yesterdayKey := fmt.Sprintf(sentSetName, time.Now().Add(time.Hour*-24).UTC().Format("2006_01_02"))
	return redis.Bool(luaSent.Do(rc, todayKey, yesterdayKey, msg.ID().String()))
}

// MarkOutgoingMsgComplete marks the passed in message as having completed processing, freeing up a worker for that channel
func (b *backend) MarkOutgoingMsgComplete(msg courier.Msg, status courier.MsgStatus) {
	rc := b.redisPool.Get()
	defer rc.Close()

	dbMsg := msg.(*DBMsg)
	queue.MarkComplete(rc, msgQueueName, dbMsg.WorkerToken_)

	// mark as sent in redis as well if this was actually wired or sent
	if status != nil && (status.Status() == courier.MsgSent || status.Status() == courier.MsgWired) {
		dateKey := fmt.Sprintf(sentSetName, time.Now().UTC().Format("2006_01_02"))
		rc.Do("sadd", dateKey, msg.ID().String())
	}
}

// StopMsgContact marks the contact for the passed in msg as stopped, that is they no longer want to receive messages
func (b *backend) StopMsgContact(m courier.Msg) {
	dbMsg := m.(*DBMsg)
	b.notifier.addStopContactNotification(dbMsg.ContactID_)
}

// WriteMsg writes the passed in message to our store
func (b *backend) WriteMsg(m courier.Msg) error {
	return writeMsg(b, m)
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
func (b *backend) WriteMsgStatus(status courier.MsgStatus) error {
	err := writeMsgStatus(b, status)
	if err != nil {
		return err
	}

	// if we have an id and are marking an outgoing msg as errored, then clear our sent flag
	if status.ID() != courier.NilMsgID && status.Status() == courier.MsgErrored {
		rc := b.redisPool.Get()
		defer rc.Close()

		dateKey := fmt.Sprintf(sentSetName, time.Now().UTC().Format("2006_01_02"))
		prevDateKey := fmt.Sprintf(sentSetName, time.Now().Add(time.Hour*-24).UTC().Format("2006_01_02"))

		// we pipeline the removals because we don't care about the return value
		rc.Send("srem", dateKey, status.ID().String())
		rc.Send("srem", prevDateKey, status.ID().String())
		err := rc.Flush()
		if err != nil {
			logrus.WithError(err).WithField("msg", status.ID().String()).Error("error clearing sent flags")
		}
	}

	return nil
}

// WriteChannelLogs persists the passed in logs to our database, for rapidpro we swallow all errors, logging isn't critical
func (b *backend) WriteChannelLogs(logs []*courier.ChannelLog) error {
	for _, l := range logs {
		err := writeChannelLog(b, l)
		if err != nil {
			logrus.WithError(err).Error("error writing channel log")
		}
	}
	return nil
}

// Health returns the health of this backend as a string, returning "" if all is well
func (b *backend) Health() string {
	// test redis
	rc := b.redisPool.Get()
	defer rc.Close()
	_, redisErr := rc.Do("PING")

	// test our db
	_, dbErr := b.db.Exec("SELECT 1")
	health := bytes.Buffer{}

	if redisErr != nil {
		health.WriteString(fmt.Sprintf("\n% 16s: %v", "redis err", redisErr))
	}
	if dbErr != nil {
		health.WriteString(fmt.Sprintf("\n% 16s: %v", "db err", dbErr))
	}

	return health.String()
}

// Status returns information on our queue sizes, number of workers etc..
func (b *backend) Status() string {
	rc := b.redisPool.Get()
	defer rc.Close()

	// get all our active queues
	values, err := redis.Values(rc.Do("zrevrangebyscore", fmt.Sprintf("%s:active", msgQueueName), "+inf", "-inf", "withscores"))
	if err != nil {
		return fmt.Sprintf("unable to read active queue: %v", err)
	}

	status := bytes.Buffer{}
	status.WriteString("------------------------------------------------------------------------------------\n")
	status.WriteString("     Size | Bulk Size | Workers | TPS | Type | Channel              \n")
	status.WriteString("------------------------------------------------------------------------------------\n")
	var queue string
	var workers float64
	var uuid string
	var tps string
	var channelType = ""

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
		uuid = parts[0]
		tps = parts[1]

		// try to look up our channel
		channelUUID, _ := courier.NewChannelUUID(uuid)
		channel, err := getChannel(b, courier.AnyChannelType, channelUUID)
		if err != nil {
			channelType = "!!"
		} else {
			channelType = channel.ChannelType().String()
		}

		// get # of items in our normal queue
		size, err := redis.Int64(rc.Do("zcard", fmt.Sprintf("%s:%s/1", msgQueueName, queue)))
		if err != nil {
			return fmt.Sprintf("error reading queue size: %v", err)
		}

		// get # of items in the bulk queue
		bulkSize, err := redis.Int64(rc.Do("zcard", fmt.Sprintf("%s:%s/0", msgQueueName, queue)))
		if err != nil {
			return fmt.Sprintf("error reading bulk queue size: %v", err)
		}

		status.WriteString(fmt.Sprintf("% 9d   % 9d   % 7d   % 3s   % 4s   %s\n", size, bulkSize, int(workers), tps, channelType, uuid))
	}

	return status.String()
}

// Start starts our RapidPro backend, this tests our various connections and starts our spool flushers
func (b *backend) Start() error {
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

	// test our db connection
	db, err := sqlx.Connect("postgres", b.config.DB)
	if err != nil {
		log.Error("db not reachable")
	} else {
		log.Info("db ok")
	}
	b.db = db
	b.db.SetMaxIdleConns(4)
	b.db.SetMaxOpenConns(16)

	// parse and test our redis config
	redisURL, err := url.Parse(b.config.Redis)
	if err != nil {
		return fmt.Errorf("unable to parse Redis URL '%s': %s", b.config.Redis, err)
	}

	// create our pool
	redisPool := &redis.Pool{
		Wait:        true,              // makes callers wait for a connection
		MaxActive:   8,                 // only open this many concurrent connections at once
		MaxIdle:     4,                 // only keep up to this many idle
		IdleTimeout: 240 * time.Second, // how long to wait before reaping a connection
		Dial: func() (redis.Conn, error) {
			conn, err := redis.Dial("tcp", fmt.Sprintf("%s", redisURL.Host))
			if err != nil {
				return nil, err
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

	// start our dethrottler
	queue.StartDethrottler(redisPool, b.stopChan, b.waitGroup, msgQueueName)

	// create our s3 client
	s3Session, err := session.NewSession(&aws.Config{
		Credentials: credentials.NewStaticCredentials(b.config.AWSAccessKeyID, b.config.AWSSecretAccessKey, ""),
		Region:      aws.String(b.config.S3Region),
	})
	if err != nil {
		return err
	}
	b.s3Client = s3.New(s3Session)

	// test out our S3 credentials
	err = utils.TestS3(b.s3Client, b.config.S3MediaBucket)
	if err != nil {
		log.WithError(err).Error("s3 bucket not reachable")
	} else {
		log.Info("s3 bucket ok")
	}

	// make sure our spool dirs are writable
	err = courier.EnsureSpoolDirPresent(b.config.SpoolDir, "msgs")
	if err == nil {
		err = courier.EnsureSpoolDirPresent(b.config.SpoolDir, "statuses")
	}
	if err != nil {
		log.WithError(err).Error("spool directories not writable")
	} else {
		log.Info("spool directories ok")
	}

	// start our rapidpro notifier
	b.notifier = newNotifier(b.config)
	b.notifier.start(b)

	// register and start our msg spool flushers
	courier.RegisterFlusher(path.Join(b.config.SpoolDir, "msgs"), b.flushMsgFile)
	courier.RegisterFlusher(path.Join(b.config.SpoolDir, "statuses"), b.flushStatusFile)

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
	// close our db and redis pool
	if b.db != nil {
		b.db.Close()
	}
	return b.redisPool.Close()
}

// NewBackend creates a new RapidPro backend
func newBackend(config *config.Courier) courier.Backend {
	return &backend{
		config: config,

		stopChan:  make(chan bool),
		waitGroup: &sync.WaitGroup{},
	}
}

type backend struct {
	config *config.Courier

	db        *sqlx.DB
	redisPool *redis.Pool
	s3Client  s3iface.S3API
	awsCreds  *credentials.Credentials

	popScript *redis.Script

	notifier *notifier

	stopChan  chan bool
	waitGroup *sync.WaitGroup
}
