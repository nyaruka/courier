package rapidpro

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gomodule/redigo/redis"
	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/queue"
	"github.com/nyaruka/gocommon/analytics"
	"github.com/nyaruka/gocommon/dbutil"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/storage"
	"github.com/nyaruka/gocommon/syncx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/redisx"
	"github.com/pkg/errors"
)

// the name for our message queue
const msgQueueName = "msgs"

// the name of our set for tracking sends
const sentSetName = "msgs_sent_%s"

// our timeout for backend operations
const backendTimeout = time.Second * 20

// storage directory (only used with file system storage)
var storageDir = "_storage"

var uuidRegex = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

func init() {
	courier.RegisterBackend("rapidpro", newBackend)
}

type backend struct {
	config *courier.Config

	statusWriter *StatusWriter
	dbLogWriter  *DBLogWriter      // unattached logs being written to the database
	stLogWriter  *StorageLogWriter // attached logs being written to storage
	writerWG     *sync.WaitGroup

	db                *sqlx.DB
	redisPool         *redis.Pool
	attachmentStorage storage.Storage
	logStorage        storage.Storage

	stopChan  chan bool
	waitGroup *sync.WaitGroup

	httpClient         *http.Client
	httpClientInsecure *http.Client
	httpAccess         *httpx.AccessConfig

	mediaCache   *redisx.IntervalHash
	mediaMutexes syncx.HashMutex

	// tracking of recent messages received to avoid creating duplicates
	receivedExternalIDs *redisx.IntervalHash // using external id
	receivedMsgs        *redisx.IntervalHash // using content hash

	// tracking of external ids of messages we've sent in case we need one before its status update has been written
	sentExternalIDs *redisx.IntervalHash

	// both sqlx and redis provide wait stats which are cummulative that we need to convert into increments
	dbWaitDuration    time.Duration
	dbWaitCount       int64
	redisWaitDuration time.Duration
	redisWaitCount    int64
}

// NewBackend creates a new RapidPro backend
func newBackend(cfg *courier.Config) courier.Backend {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 64
	transport.MaxIdleConnsPerHost = 8
	transport.IdleConnTimeout = 15 * time.Second

	insecureTransport := http.DefaultTransport.(*http.Transport).Clone()
	insecureTransport.MaxIdleConns = 64
	insecureTransport.MaxIdleConnsPerHost = 8
	insecureTransport.IdleConnTimeout = 15 * time.Second
	insecureTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	disallowedIPs, disallowedNets, _ := cfg.ParseDisallowedNetworks()

	return &backend{
		config: cfg,

		httpClient:         &http.Client{Transport: transport, Timeout: 30 * time.Second},
		httpClientInsecure: &http.Client{Transport: insecureTransport, Timeout: 30 * time.Second},
		httpAccess:         httpx.NewAccessConfig(10*time.Second, disallowedIPs, disallowedNets),

		stopChan:  make(chan bool),
		waitGroup: &sync.WaitGroup{},

		writerWG: &sync.WaitGroup{},

		mediaCache:   redisx.NewIntervalHash("media-lookups", time.Hour*24, 2),
		mediaMutexes: *syncx.NewHashMutex(8),

		receivedMsgs:        redisx.NewIntervalHash("seen-msgs", time.Second*2, 2),        // 2 - 4 seconds
		receivedExternalIDs: redisx.NewIntervalHash("seen-external-ids", time.Hour*24, 2), // 24 - 48 hours
		sentExternalIDs:     redisx.NewIntervalHash("sent-external-ids", time.Hour, 2),    // 1 - 2 hours
	}
}

// Start starts our RapidPro backend, this tests our various connections and starts our spool flushers
func (b *backend) Start() error {
	// parse and test our redis config
	log := slog.With(
		"comp", "backend",
		"state", "starting",
	)
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
		log.Error("db not reachable", "error", err)
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
		log.Error("redis not reachable", "error", err)
	} else {
		log.Info("redis ok")
	}

	// start our dethrottler if we are going to be doing some sending
	if b.config.MaxWorkers > 0 {
		queue.StartDethrottler(redisPool, b.stopChan, b.waitGroup, msgQueueName)
	}

	// create our storage (S3 or file system)
	if b.config.AWSAccessKeyID != "" || b.config.AWSUseCredChain {
		s3config := &storage.S3Options{
			AWSAccessKeyID:     b.config.AWSAccessKeyID,
			AWSSecretAccessKey: b.config.AWSSecretAccessKey,
			Endpoint:           b.config.S3Endpoint,
			Region:             b.config.S3Region,
			DisableSSL:         b.config.S3DisableSSL,
			ForcePathStyle:     b.config.S3ForcePathStyle,
			MaxRetries:         3,
		}
		if b.config.AWSAccessKeyID != "" && !b.config.AWSUseCredChain {
			s3config.AWSAccessKeyID = b.config.AWSAccessKeyID
			s3config.AWSSecretAccessKey = b.config.AWSSecretAccessKey
		}
		s3Client, err := storage.NewS3Client(s3config)
		if err != nil {
			return err
		}
		b.attachmentStorage = storage.NewS3(s3Client, b.config.S3AttachmentsBucket, b.config.S3Region, s3.BucketCannedACLPublicRead, 32)
		b.logStorage = storage.NewS3(s3Client, b.config.S3LogsBucket, b.config.S3Region, s3.BucketCannedACLPrivate, 32)
	} else {
		b.attachmentStorage = storage.NewFS(storageDir+"/attachments", 0766)
		b.logStorage = storage.NewFS(storageDir+"/logs", 0766)
	}

	// check our storages
	if err := checkStorage(b.attachmentStorage); err != nil {
		log.Error(b.attachmentStorage.Name()+" attachment storage not available", "error", err)
	} else {
		log.Info(b.attachmentStorage.Name() + " attachment storage ok")
	}
	if err := checkStorage(b.logStorage); err != nil {
		log.Error(b.logStorage.Name()+" log storage not available", "error", err)
	} else {
		log.Info(b.logStorage.Name() + " log storage ok")
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
		log.Error("spool directories not writable", "error", err)
	} else {
		log.Info("spool directories ok")
	}

	// create our batched writers and start them
	b.statusWriter = NewStatusWriter(b, b.config.SpoolDir, b.writerWG)
	b.statusWriter.Start()

	b.dbLogWriter = NewDBLogWriter(b.db, b.writerWG)
	b.dbLogWriter.Start()

	b.stLogWriter = NewStorageLogWriter(b.logStorage, b.writerWG)
	b.stLogWriter.Start()

	// register and start our spool flushers
	courier.RegisterFlusher(path.Join(b.config.SpoolDir, "msgs"), b.flushMsgFile)
	courier.RegisterFlusher(path.Join(b.config.SpoolDir, "statuses"), b.flushStatusFile)
	courier.RegisterFlusher(path.Join(b.config.SpoolDir, "events"), b.flushChannelEventFile)

	slog.Info("backend started", "comp", "backend", "state", "started")
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
	// stop our batched writers
	if b.statusWriter != nil {
		b.statusWriter.Stop()
	}
	if b.dbLogWriter != nil {
		b.dbLogWriter.Stop()
	}
	if b.stLogWriter != nil {
		b.stLogWriter.Stop()
	}

	// wait for them to flush fully
	b.writerWG.Wait()

	// close our db and redis pool
	if b.db != nil {
		b.db.Close()
	}
	return b.redisPool.Close()
}

// GetChannel returns the channel for the passed in type and UUID
func (b *backend) GetChannel(ctx context.Context, ct courier.ChannelType, uuid courier.ChannelUUID) (courier.Channel, error) {
	timeout, cancel := context.WithTimeout(ctx, backendTimeout)
	defer cancel()

	ch, err := getChannel(timeout, b.db, ct, uuid)
	if err != nil {
		return nil, err // so we don't return a non-nil interface and nil ptr
	}
	return ch, err
}

// GetChannelByAddress returns the channel with the passed in type and address
func (b *backend) GetChannelByAddress(ctx context.Context, ct courier.ChannelType, address courier.ChannelAddress) (courier.Channel, error) {
	timeout, cancel := context.WithTimeout(ctx, backendTimeout)
	defer cancel()

	ch, err := getChannelByAddress(timeout, b.db, ct, address)
	if err != nil {
		return nil, err // so we don't return a non-nil interface and nil ptr
	}
	return ch, err
}

// GetContact returns the contact for the passed in channel and URN
func (b *backend) GetContact(ctx context.Context, c courier.Channel, urn urns.URN, authTokens map[string]string, name string, clog *courier.ChannelLog) (courier.Contact, error) {
	dbChannel := c.(*Channel)
	return contactForURN(ctx, b, dbChannel.OrgID_, dbChannel, urn, authTokens, name, clog)
}

// AddURNtoContact adds a URN to the passed in contact
func (b *backend) AddURNtoContact(ctx context.Context, c courier.Channel, contact courier.Contact, urn urns.URN, authTokens map[string]string) (urns.URN, error) {
	tx, err := b.db.BeginTxx(ctx, nil)
	if err != nil {
		return urns.NilURN, err
	}
	dbChannel := c.(*Channel)
	dbContact := contact.(*Contact)
	_, err = getOrCreateContactURN(tx, dbChannel, dbContact.ID_, urn, authTokens)
	if err != nil {
		return urns.NilURN, err
	}
	err = tx.Commit()
	if err != nil {
		return urns.NilURN, err
	}

	return urn, nil
}

// RemoveURNFromcontact removes a URN from the passed in contact
func (b *backend) RemoveURNfromContact(ctx context.Context, c courier.Channel, contact courier.Contact, urn urns.URN) (urns.URN, error) {
	dbContact := contact.(*Contact)
	_, err := b.db.ExecContext(ctx, `UPDATE contacts_contacturn SET contact_id = NULL WHERE contact_id = $1 AND identity = $2`, dbContact.ID_, urn.Identity().String())
	if err != nil {
		return urns.NilURN, err
	}
	return urn, nil
}

// DeleteMsgByExternalID resolves a message external id and quees a task to mailroom to delete it
func (b *backend) DeleteMsgByExternalID(ctx context.Context, channel courier.Channel, externalID string) error {
	ch := channel.(*Channel)
	row := b.db.QueryRowContext(ctx, `SELECT id, contact_id FROM msgs_msg WHERE channel_id = $1 AND external_id = $2 AND direction = 'I'`, ch.ID(), externalID)

	var msgID courier.MsgID
	var contactID ContactID
	if err := row.Scan(&msgID, &contactID); err != nil && err != sql.ErrNoRows {
		return errors.Wrap(err, "error querying deleted msg")
	}

	if msgID != courier.NilMsgID && contactID != NilContactID {
		rc := b.redisPool.Get()
		defer rc.Close()

		if err := queueMsgDeleted(rc, ch, msgID, contactID); err != nil {
			return errors.Wrap(err, "error queuing message deleted task")
		}
	}

	return nil
}

// NewIncomingMsg creates a new message from the given params
func (b *backend) NewIncomingMsg(channel courier.Channel, urn urns.URN, text string, extID string, clog *courier.ChannelLog) courier.MsgIn {
	// strip out invalid UTF8 and NULL chars
	urn = urns.URN(dbutil.ToValidUTF8(string(urn)))
	text = dbutil.ToValidUTF8(text)
	extID = dbutil.ToValidUTF8(extID)

	msg := newMsg(MsgIncoming, channel, urn, text, extID, clog)
	msg.WithReceivedOn(time.Now().UTC())

	// check if this message could be a duplicate and if so use the original's UUID
	if prevUUID := b.checkMsgAlreadyReceived(msg); prevUUID != courier.NilMsgUUID {
		msg.UUID_ = prevUUID
		msg.alreadyWritten = true
	}

	return msg
}

// PopNextOutgoingMsg pops the next message that needs to be sent
func (b *backend) PopNextOutgoingMsg(ctx context.Context) (courier.MsgOut, error) {
	// pop the next message off our queue
	rc := b.redisPool.Get()
	defer rc.Close()

	token, msgJSON, err := queue.PopFromQueue(rc, msgQueueName)
	if err != nil {
		return nil, err
	}

	for token == queue.Retry {
		token, msgJSON, err = queue.PopFromQueue(rc, msgQueueName)
		if err != nil {
			return nil, err
		}
	}

	if msgJSON != "" {
		dbMsg := &Msg{}
		err = json.Unmarshal([]byte(msgJSON), dbMsg)
		if err != nil {
			queue.MarkComplete(rc, msgQueueName, token)
			return nil, errors.Wrapf(err, "unable to unmarshal message: %s", string(msgJSON))
		}

		// populate the channel on our db msg
		channel, err := b.GetChannel(ctx, courier.AnyChannelType, dbMsg.ChannelUUID_)
		if err != nil {
			queue.MarkComplete(rc, msgQueueName, token)
			return nil, err
		}

		dbMsg.Direction_ = MsgOutgoing
		dbMsg.channel = channel.(*Channel)
		dbMsg.workerToken = token

		// clear out our seen incoming messages
		b.clearMsgSeen(rc, dbMsg)

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
func (b *backend) MarkOutgoingMsgComplete(ctx context.Context, msg courier.MsgOut, status courier.StatusUpdate) {
	rc := b.redisPool.Get()
	defer rc.Close()

	dbMsg := msg.(*Msg)

	queue.MarkComplete(rc, msgQueueName, dbMsg.workerToken)

	// mark as sent in redis as well if this was actually wired or sent
	if status != nil && (status.Status() == courier.MsgStatusSent || status.Status() == courier.MsgStatusWired) {
		dateKey := fmt.Sprintf(sentSetName, time.Now().UTC().Format("2006_01_02"))
		rc.Send("sadd", dateKey, msg.ID().String())
		rc.Send("expire", dateKey, 60*60*24*2)
		_, err := rc.Do("")
		if err != nil {
			slog.Error("unable to add new unsent message", "error", err, "sent_msgs_key", dateKey)
		}

		// if our msg has an associated session and timeout, update that
		if dbMsg.SessionWaitStartedOn_ != nil {
			err = updateSessionTimeout(ctx, b, dbMsg.SessionID_, *dbMsg.SessionWaitStartedOn_, dbMsg.SessionTimeout_)
			if err != nil {
				slog.Error("unable to update session timeout", "error", err, "session_id", dbMsg.SessionID_)
			}
		}
	}
}

// WriteMsg writes the passed in message to our store
func (b *backend) WriteMsg(ctx context.Context, m courier.MsgIn, clog *courier.ChannelLog) error {
	timeout, cancel := context.WithTimeout(ctx, backendTimeout)
	defer cancel()

	return writeMsg(timeout, b, m, clog)
}

// NewStatusUpdateForID creates a new Status object for the given message id
func (b *backend) NewStatusUpdate(channel courier.Channel, id courier.MsgID, status courier.MsgStatus, clog *courier.ChannelLog) courier.StatusUpdate {
	return newStatusUpdate(channel, id, "", status, clog)
}

// NewStatusUpdateForID creates a new Status object for the given message id
func (b *backend) NewStatusUpdateByExternalID(channel courier.Channel, externalID string, status courier.MsgStatus, clog *courier.ChannelLog) courier.StatusUpdate {
	return newStatusUpdate(channel, courier.NilMsgID, externalID, status, clog)
}

// WriteStatusUpdate writes the passed in MsgStatus to our store
func (b *backend) WriteStatusUpdate(ctx context.Context, status courier.StatusUpdate) error {
	log := slog.With("msg_id", status.MsgID(), "msg_external_id", status.ExternalID(), "status", status.Status())
	su := status.(*StatusUpdate)

	if status.MsgID() == courier.NilMsgID && status.ExternalID() == "" {
		return errors.New("message status with no id or external id")
	}

	// if we have a URN update, do that
	oldURN, newURN := status.URNUpdate()
	if oldURN != urns.NilURN && newURN != urns.NilURN {
		err := b.updateContactURN(ctx, status)
		if err != nil {
			return errors.Wrap(err, "error updating contact URN")
		}
	}

	if status.MsgID() != courier.NilMsgID {
		// this is a message we've just sent and were given an external id for
		if status.ExternalID() != "" {
			rc := b.redisPool.Get()
			defer rc.Close()

			err := b.sentExternalIDs.Set(rc, fmt.Sprintf("%d|%s", su.ChannelID_, su.ExternalID_), fmt.Sprintf("%d", status.MsgID()))
			if err != nil {
				log.Error("error recording external id", "error", err)
			}
		}

		// we sent a message that errored so clear our sent flag to allow it to be retried
		if status.Status() == courier.MsgStatusErrored {
			err := b.ClearMsgSent(ctx, status.MsgID())
			if err != nil {
				log.Error("error clearing sent flags", "error", err)
			}
		}
	}

	// queue the status to written by the batch writer
	b.statusWriter.Queue(status.(*StatusUpdate))
	log.Debug("status update queued")

	return nil
}

// updateContactURN updates contact URN according to the old/new URNs from status
func (b *backend) updateContactURN(ctx context.Context, status courier.StatusUpdate) error {
	old, new := status.URNUpdate()

	// retrieve channel
	channel, err := b.GetChannel(ctx, courier.AnyChannelType, status.ChannelUUID())
	if err != nil {
		return errors.Wrap(err, "error retrieving channel")
	}
	dbChannel := channel.(*Channel)
	tx, err := b.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	// retrieve the old URN
	oldContactURN, err := getContactURNByIdentity(tx, dbChannel.OrgID(), old)
	if err != nil {
		return errors.Wrap(err, "error retrieving old contact URN")
	}
	// retrieve the new URN
	newContactURN, err := getContactURNByIdentity(tx, dbChannel.OrgID(), new)
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
func (b *backend) NewChannelEvent(channel courier.Channel, eventType courier.ChannelEventType, urn urns.URN, clog *courier.ChannelLog) courier.ChannelEvent {
	return newChannelEvent(channel, eventType, urn, clog)
}

// WriteChannelEvent writes the passed in channel even returning any error
func (b *backend) WriteChannelEvent(ctx context.Context, event courier.ChannelEvent, clog *courier.ChannelLog) error {
	timeout, cancel := context.WithTimeout(ctx, backendTimeout)
	defer cancel()

	return writeChannelEvent(timeout, b, event, clog)
}

// WriteChannelLog persists the passed in log to our database, for rapidpro we swallow all errors, logging isn't critical
func (b *backend) WriteChannelLog(ctx context.Context, clog *courier.ChannelLog) error {
	timeout, cancel := context.WithTimeout(ctx, backendTimeout)
	defer cancel()

	queueChannelLog(timeout, b, clog)

	return nil
}

// SaveAttachment saves an attachment to backend storage
func (b *backend) SaveAttachment(ctx context.Context, ch courier.Channel, contentType string, data []byte, extension string) (string, error) {
	// create our filename
	filename := string(uuids.New())
	if extension != "" {
		filename = fmt.Sprintf("%s.%s", filename, extension)
	}

	orgID := ch.(*Channel).OrgID()

	path := filepath.Join(b.config.S3AttachmentsPrefix, strconv.FormatInt(int64(orgID), 10), filename[:4], filename[4:8], filename)

	storageURL, err := b.attachmentStorage.Put(ctx, path, contentType, data)
	if err != nil {
		return "", errors.Wrapf(err, "error saving attachment to storage (bytes=%d)", len(data))
	}

	return storageURL, nil
}

// ResolveMedia resolves the passed in attachment URL to a media object
func (b *backend) ResolveMedia(ctx context.Context, mediaUrl string) (courier.Media, error) {
	u, err := url.Parse(mediaUrl)
	if err != nil {
		return nil, errors.Wrapf(err, "error parsing media URL")
	}

	mediaUUID := uuidRegex.FindString(u.Path)

	// if hostname isn't our media domain, or path doesn't contain a UUID, don't try to resolve
	if u.Hostname() != b.config.MediaDomain || mediaUUID == "" {
		return nil, nil
	}

	unlock := b.mediaMutexes.Lock(mediaUUID)
	defer unlock()

	rc := b.redisPool.Get()
	defer rc.Close()

	var media *Media
	mediaJSON, err := b.mediaCache.Get(rc, mediaUUID)
	if err != nil {
		return nil, errors.Wrap(err, "error looking up cached media")
	}
	if mediaJSON != "" {
		jsonx.MustUnmarshal([]byte(mediaJSON), &media)
	} else {
		// lookup media in our database
		media, err = lookupMediaFromUUID(ctx, b.db, uuids.UUID(mediaUUID))
		if err != nil {
			return nil, errors.Wrap(err, "error looking up media")
		}

		// cache it for future requests
		b.mediaCache.Set(rc, mediaUUID, string(jsonx.MustMarshal(media)))
	}

	// if we found a media record but it doesn't match the URL, don't use it
	if media == nil || media.URL() != mediaUrl {
		return nil, nil
	}

	return media, nil
}

func (b *backend) HttpClient(secure bool) *http.Client {
	if secure {
		return b.httpClient
	}
	return b.httpClientInsecure
}

func (b *backend) HttpAccess() *httpx.AccessConfig {
	return b.httpAccess
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

	analytics.Gauge("courier.db_busy", float64(dbStats.InUse))
	analytics.Gauge("courier.db_idle", float64(dbStats.Idle))
	analytics.Gauge("courier.db_wait_ms", float64(dbWaitDurationInPeriod/time.Millisecond))
	analytics.Gauge("courier.db_wait_count", float64(dbWaitCountInPeriod))
	analytics.Gauge("courier.redis_wait_ms", float64(redisWaitDurationInPeriod/time.Millisecond))
	analytics.Gauge("courier.redis_wait_count", float64(redisWaitCountInPeriod))
	analytics.Gauge("courier.bulk_queue", float64(bulkSize))
	analytics.Gauge("courier.priority_queue", float64(prioritySize))

	slog.Info("current analytics", "db_busy", dbStats.InUse,
		"db_idle", dbStats.Idle,
		"db_wait_time", dbWaitDurationInPeriod,
		"db_wait_count", dbWaitCountInPeriod,
		"redis_wait_time", dbWaitDurationInPeriod,
		"redis_wait_count", dbWaitCountInPeriod,
		"priority_size", prioritySize,
		"bulk_size", bulkSize)

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
		channelUUID := courier.ChannelUUID(uuid)
		channel, err := getChannel(context.Background(), b.db, courier.AnyChannelType, channelUUID)
		channelType := "!!"
		if err == nil {
			channelType = string(channel.ChannelType())
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

// RedisPool returns the redisPool for this backend
func (b *backend) RedisPool() *redis.Pool {
	return b.redisPool
}

func checkStorage(s storage.Storage) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	err := s.Test(ctx)
	cancel()
	return err
}
