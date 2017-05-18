package rapidpro

import (
	"bytes"
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
	"github.com/garyburd/redigo/redis"
	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/config"
	"github.com/nyaruka/courier/utils"
	"github.com/sirupsen/logrus"
)

func init() {
	courier.RegisterBackend("rapidpro", newBackend)
}

// GetChannel returns the channel for the passed in type and UUID
func (b *backend) GetChannel(ct courier.ChannelType, uuid courier.ChannelUUID) (courier.Channel, error) {
	return getChannel(b, ct, uuid)
}

// WriteMsg writes the passed in message to our store
func (b *backend) WriteMsg(m *courier.Msg) error {
	return writeMsg(b, m)
}

// WriteMsgStatus writes the passed in MsgStatus to our store
func (b *backend) WriteMsgStatus(status *courier.MsgStatusUpdate) error {
	return writeMsgStatus(b, status)
}

// Health returns the health of this backend as a string, returning "" if all is well
func (b *backend) Health() string {
	// test redis
	rc := b.redisPool.Get()
	_, redisErr := rc.Do("PING")
	defer rc.Close()

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

	// parse and test our redis config
	redisURL, err := url.Parse(b.config.Redis)
	if err != nil {
		return fmt.Errorf("unable to parse Redis URL '%s': %s", b.config.Redis, err)
	}

	// create our pool
	redisPool := &redis.Pool{
		Wait:        true,              // makes callers wait for a connection
		MaxActive:   5,                 // only open this many concurrent connections at once
		MaxIdle:     2,                 // only keep up to 2 idle
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
	if b.db != nil {
		b.db.Close()
	}

	b.redisPool.Close()

	// close our stop channel
	close(b.stopChan)

	// wait for our threads to exit
	b.waitGroup.Wait()

	return nil
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
	s3Client  *s3.S3
	awsCreds  *credentials.Credentials

	notifier *notifier

	stopChan  chan bool
	waitGroup *sync.WaitGroup
}
