package rapidpro

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
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

	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/gomodule/redigo/redis"
	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/queue"
	"github.com/nyaruka/gocommon/aws/cwatch"
	"github.com/nyaruka/gocommon/aws/dynamo"
	"github.com/nyaruka/gocommon/aws/s3x"
	"github.com/nyaruka/gocommon/cache"
	"github.com/nyaruka/gocommon/dbutil"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/syncx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/redisx"
)

// the name for our message queue
const msgQueueName = "msgs"

// our timeout for backend operations
const backendTimeout = time.Second * 20

var uuidRegex = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

func init() {
	courier.RegisterBackend("rapidpro", newBackend)
}

type backend struct {
	config *courier.Config

	statusWriter *StatusWriter
	dbLogWriter  *DBLogWriter     // unattached logs being written to the database
	dyLogWriter  *DynamoLogWriter // all logs being written to dynamo
	writerWG     *sync.WaitGroup

	db     *sqlx.DB
	rp     *redis.Pool
	dynamo *dynamo.Service
	s3     *s3x.Service
	cw     *cwatch.Service

	channelsByUUID *cache.Local[courier.ChannelUUID, *Channel]
	channelsByAddr *cache.Local[courier.ChannelAddress, *Channel]

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

	// tracking of sent message ids to avoid dupe sends
	sentIDs *redisx.IntervalSet

	// tracking of external ids of messages we've sent in case we need one before its status update has been written
	sentExternalIDs *redisx.IntervalHash

	stats *StatsCollector

	// both sqlx and redis provide wait stats which are cummulative that we need to convert into increments by
	// tracking their previous values
	dbWaitDuration    time.Duration
	redisWaitDuration time.Duration
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
		sentIDs:             redisx.NewIntervalSet("sent-ids", time.Hour, 2),              // 1 - 2 hours
		sentExternalIDs:     redisx.NewIntervalHash("sent-external-ids", time.Hour, 2),    // 1 - 2 hours

		stats: NewStatsCollector(),
	}
}

// Start starts our RapidPro backend, this tests our various connections and starts our spool flushers
func (b *backend) Start() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// parse and test our redis config
	log := slog.With("comp", "backend", "state", "starting")
	log.Info("starting backend")

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
	if err := b.db.PingContext(ctx); err != nil {
		log.Error("db not reachable", "error", err)
	} else {
		log.Info("db ok")
	}

	b.rp, err = redisx.NewPool(b.config.Redis, redisx.WithMaxActive(b.config.MaxWorkers*2))
	if err != nil {
		log.Error("redis not reachable", "error", err)
	} else {
		log.Info("redis ok")
	}

	// start our dethrottler if we are going to be doing some sending
	if b.config.MaxWorkers > 0 {
		queue.StartDethrottler(b.rp, b.stopChan, b.waitGroup, msgQueueName)
	}

	// setup DynamoDB
	b.dynamo, err = dynamo.NewService(b.config.AWSAccessKeyID, b.config.AWSSecretAccessKey, b.config.AWSRegion, b.config.DynamoEndpoint, b.config.DynamoTablePrefix)
	if err != nil {
		return err
	}
	if err := b.dynamo.Test(ctx); err != nil {
		log.Error("dynamodb not reachable", "error", err)
	} else {
		log.Info("dynamodb ok")
	}

	// setup S3 storage
	b.s3, err = s3x.NewService(b.config.AWSAccessKeyID, b.config.AWSSecretAccessKey, b.config.AWSRegion, b.config.S3Endpoint, b.config.S3Minio)
	if err != nil {
		return err
	}

	b.cw, err = cwatch.NewService(b.config.AWSAccessKeyID, b.config.AWSSecretAccessKey, b.config.AWSRegion, b.config.CloudwatchNamespace, b.config.DeploymentID)
	if err != nil {
		return err
	}

	// check attachment bucket access
	if err := b.s3.Test(ctx, b.config.S3AttachmentsBucket); err != nil {
		log.Error("attachments bucket not accessible", "error", err)
	} else {
		log.Info("attachments bucket ok")
	}

	// create and start channel caches...
	b.channelsByUUID = cache.NewLocal(b.loadChannelByUUID, time.Minute)
	b.channelsByUUID.Start()
	b.channelsByAddr = cache.NewLocal(b.loadChannelByAddress, time.Minute)
	b.channelsByAddr.Start()

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

	b.dyLogWriter = NewDynamoLogWriter(b.dynamo, b.writerWG)
	b.dyLogWriter.Start()

	// register and start our spool flushers
	courier.RegisterFlusher(path.Join(b.config.SpoolDir, "msgs"), b.flushMsgFile)
	courier.RegisterFlusher(path.Join(b.config.SpoolDir, "statuses"), b.flushStatusFile)
	courier.RegisterFlusher(path.Join(b.config.SpoolDir, "events"), b.flushChannelEventFile)

	b.startMetricsReporter(time.Minute)

	slog.Info("backend started", "comp", "backend", "state", "started")
	return nil
}

func (b *backend) startMetricsReporter(interval time.Duration) {
	b.waitGroup.Add(1)

	report := func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		count, err := b.reportMetrics(ctx)
		cancel()
		if err != nil {
			slog.Error("error reporting metrics", "error", err)
		} else {
			slog.Info("sent metrics to cloudwatch", "count", count)
		}
	}

	go func() {
		defer func() {
			slog.Info("metrics reporter exiting")
			b.waitGroup.Done()
		}()

		for {
			select {
			case <-b.stopChan:
				report()
				return
			case <-time.After(interval):
				report()
			}
		}
	}()
}

// Stop stops our RapidPro backend, closing our db and redis connections
func (b *backend) Stop() error {
	// close our stop channel
	close(b.stopChan)

	b.channelsByUUID.Stop()
	b.channelsByAddr.Stop()

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
	if b.dyLogWriter != nil {
		b.dyLogWriter.Stop()
	}

	// wait for them to flush fully
	b.writerWG.Wait()

	// close our db and redis pool
	if b.db != nil {
		b.db.Close()
	}
	return b.rp.Close()
}

// GetChannel returns the channel for the passed in type and UUID
func (b *backend) GetChannel(ctx context.Context, typ courier.ChannelType, uuid courier.ChannelUUID) (courier.Channel, error) {
	timeout, cancel := context.WithTimeout(ctx, backendTimeout)
	defer cancel()

	ch, err := b.channelsByUUID.GetOrFetch(timeout, uuid)
	if err != nil {
		return nil, err // so we don't return a non-nil interface and nil ptr
	}

	if typ != courier.AnyChannelType && ch.ChannelType() != typ {
		return nil, courier.ErrChannelWrongType
	}

	return ch, nil
}

// GetChannelByAddress returns the channel with the passed in type and address
func (b *backend) GetChannelByAddress(ctx context.Context, typ courier.ChannelType, address courier.ChannelAddress) (courier.Channel, error) {
	timeout, cancel := context.WithTimeout(ctx, backendTimeout)
	defer cancel()

	ch, err := b.channelsByAddr.GetOrFetch(timeout, address)
	if err != nil {
		return nil, err // so we don't return a non-nil interface and nil ptr
	}

	if typ != courier.AnyChannelType && ch.ChannelType() != typ {
		return nil, courier.ErrChannelWrongType
	}

	return ch, nil
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
		return fmt.Errorf("error querying deleted msg: %w", err)
	}

	if msgID != courier.NilMsgID && contactID != NilContactID {
		rc := b.rp.Get()
		defer rc.Close()

		if err := queueMsgDeleted(rc, ch, msgID, contactID); err != nil {
			return fmt.Errorf("error queuing message deleted task: %w", err)
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
	tryToPop := func() (queue.WorkerToken, string, error) {
		rc := b.rp.Get()
		defer rc.Close()
		return queue.PopFromQueue(rc, msgQueueName)
	}

	markComplete := func(token queue.WorkerToken) {
		rc := b.rp.Get()
		defer rc.Close()
		if err := queue.MarkComplete(rc, msgQueueName, token); err != nil {
			slog.Error("error marking queue task complete", "error", err)
		}
	}

	// pop the next message off our queue
	token, msgJSON, err := tryToPop()
	if err != nil {
		return nil, err
	}

	for token == queue.Retry {
		token, msgJSON, err = tryToPop()
		if err != nil {
			return nil, err
		}
	}

	if msgJSON == "" {
		return nil, nil
	}

	dbMsg := &Msg{}
	err = json.Unmarshal([]byte(msgJSON), dbMsg)
	if err != nil {
		markComplete(token)
		return nil, fmt.Errorf("unable to unmarshal message: %s: %w", string(msgJSON), err)
	}

	// populate the channel on our db msg
	channel, err := b.GetChannel(ctx, courier.AnyChannelType, dbMsg.ChannelUUID_)
	if err != nil {
		markComplete(token)
		return nil, err
	}

	dbMsg.Direction_ = MsgOutgoing
	dbMsg.channel = channel.(*Channel)
	dbMsg.workerToken = token

	// clear out our seen incoming messages
	b.clearMsgSeen(dbMsg)

	return dbMsg, nil
}

// WasMsgSent returns whether the passed in message has already been sent
func (b *backend) WasMsgSent(ctx context.Context, id courier.MsgID) (bool, error) {
	rc := b.rp.Get()
	defer rc.Close()

	return b.sentIDs.IsMember(rc, id.String())
}

func (b *backend) ClearMsgSent(ctx context.Context, id courier.MsgID) error {
	rc := b.rp.Get()
	defer rc.Close()

	return b.sentIDs.Rem(rc, id.String())
}

// OnSendComplete is called when the sender has finished trying to send a message
func (b *backend) OnSendComplete(ctx context.Context, msg courier.MsgOut, status courier.StatusUpdate, clog *courier.ChannelLog) {
	rc := b.rp.Get()
	defer rc.Close()

	dbMsg := msg.(*Msg)

	if err := queue.MarkComplete(rc, msgQueueName, dbMsg.workerToken); err != nil {
		slog.Error("unable to mark queue task complete", "error", err)
	}

	// if message won't be retried, mark as sent to avoid dupe sends
	if status.Status() != courier.MsgStatusErrored {
		if err := b.sentIDs.Add(rc, msg.ID().String()); err != nil {
			slog.Error("unable to mark message sent", "error", err)
		}
	}

	// if message was successfully sent, and we have a session timeout, update it
	wasSuccess := status.Status() == courier.MsgStatusWired || status.Status() == courier.MsgStatusSent || status.Status() == courier.MsgStatusDelivered || status.Status() == courier.MsgStatusRead
	if wasSuccess && dbMsg.SessionWaitStartedOn_ != nil {
		if err := updateSessionTimeout(ctx, b, dbMsg.SessionID_, *dbMsg.SessionWaitStartedOn_, dbMsg.SessionTimeout_); err != nil {
			slog.Error("unable to update session timeout", "error", err, "session_id", dbMsg.SessionID_)
		}
	}

	b.stats.RecordOutgoing(msg.Channel().ChannelType(), wasSuccess, clog.Elapsed)
}

// OnReceiveComplete is called when the server has finished handling an incoming request
func (b *backend) OnReceiveComplete(ctx context.Context, ch courier.Channel, events []courier.Event, clog *courier.ChannelLog) {
	b.stats.RecordIncoming(ch.ChannelType(), events, clog.Elapsed)
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
			return fmt.Errorf("error updating contact URN: %w", err)
		}
	}

	if status.MsgID() != courier.NilMsgID {
		// this is a message we've just sent and were given an external id for
		if status.ExternalID() != "" {
			rc := b.rp.Get()
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
		return fmt.Errorf("error retrieving channel: %w", err)
	}
	dbChannel := channel.(*Channel)
	tx, err := b.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	// retrieve the old URN
	oldContactURN, err := getContactURNByIdentity(tx, dbChannel.OrgID(), old)
	if err != nil {
		return fmt.Errorf("error retrieving old contact URN: %w", err)
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
				return fmt.Errorf("error updating old contact URN: %w", err)
			}
			return tx.Commit()
		}
		return fmt.Errorf("error retrieving new contact URN: %w", err)
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
		return fmt.Errorf("error updating new contact URN: %w", err)
	}
	err = fullyUpdateContactURN(tx, oldContactURN)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("error updating old contact URN: %w", err)
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
	queueChannelLog(b, clog)
	return nil
}

// SaveAttachment saves an attachment to backend storage
func (b *backend) SaveAttachment(ctx context.Context, ch courier.Channel, contentType string, data []byte, extension string) (string, error) {
	// create our filename
	filename := string(uuids.NewV4())
	if extension != "" {
		filename = fmt.Sprintf("%s.%s", filename, extension)
	}

	orgID := ch.(*Channel).OrgID()

	path := filepath.Join("attachments", strconv.FormatInt(int64(orgID), 10), filename[:4], filename[4:8], filename)

	storageURL, err := b.s3.PutObject(ctx, b.config.S3AttachmentsBucket, path, contentType, data, s3types.ObjectCannedACLPublicRead)
	if err != nil {
		return "", fmt.Errorf("error saving attachment to storage (bytes=%d): %w", len(data), err)
	}

	return storageURL, nil
}

// ResolveMedia resolves the passed in attachment URL to a media object
func (b *backend) ResolveMedia(ctx context.Context, mediaUrl string) (courier.Media, error) {
	u, err := url.Parse(mediaUrl)
	if err != nil {
		return nil, fmt.Errorf("error parsing media URL: %w", err)
	}

	mediaUUID := uuidRegex.FindString(u.Path)

	// if hostname isn't our media domain, or path doesn't contain a UUID, don't try to resolve
	if strings.Replace(u.Hostname(), fmt.Sprintf("%s.", b.config.AWSRegion), "", -1) != b.config.MediaDomain || mediaUUID == "" {
		return nil, nil
	}

	unlock := b.mediaMutexes.Lock(mediaUUID)
	defer unlock()

	rc := b.rp.Get()
	defer rc.Close()

	var media *Media
	mediaJSON, err := b.mediaCache.Get(rc, mediaUUID)
	if err != nil {
		return nil, fmt.Errorf("error looking up cached media: %w", err)
	}
	if mediaJSON != "" {
		jsonx.MustUnmarshal([]byte(mediaJSON), &media)
	} else {
		// lookup media in our database
		media, err = lookupMediaFromUUID(ctx, b.db, uuids.UUID(mediaUUID))
		if err != nil {
			return nil, fmt.Errorf("error looking up media: %w", err)
		}

		// cache it for future requests
		b.mediaCache.Set(rc, mediaUUID, string(jsonx.MustMarshal(media)))
	}

	// if we found a media record but it doesn't match the URL, don't use it
	if media == nil || (media.URL() != mediaUrl && media.URL() != strings.Replace(mediaUrl, fmt.Sprintf("%s.", b.config.AWSRegion), "", -1)) {
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	rc, redisErr := b.rp.GetContext(ctx)
	cancel()

	if redisErr == nil {
		defer rc.Close()
		_, redisErr = rc.Do("PING")
	}

	// test our db
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
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

func (b *backend) reportMetrics(ctx context.Context) (int, error) {
	metrics := b.stats.Extract().ToMetrics()

	// get queue sizes
	rc := b.rp.Get()
	defer rc.Close()
	active, err := redis.Strings(rc.Do("ZRANGE", fmt.Sprintf("%s:active", msgQueueName), "0", "-1"))
	if err != nil {
		return 0, fmt.Errorf("error getting active queues: %w", err)
	}
	throttled, err := redis.Strings(rc.Do("ZRANGE", fmt.Sprintf("%s:throttled", msgQueueName), "0", "-1"))
	if err != nil {
		return 0, fmt.Errorf("error getting throttled queues: %w", err)
	}
	queues := append(active, throttled...)

	prioritySize := 0
	bulkSize := 0
	for _, queue := range queues {
		q := fmt.Sprintf("%s/1", queue)
		count, err := redis.Int(rc.Do("ZCARD", q))
		if err != nil {
			return 0, fmt.Errorf("error getting size of priority queue: %s: %w", q, err)
		}
		prioritySize += count

		q = fmt.Sprintf("%s/0", queue)
		count, err = redis.Int(rc.Do("ZCARD", q))
		if err != nil {
			return 0, fmt.Errorf("error getting size of bulk queue: %s: %w", q, err)
		}
		bulkSize += count
	}

	// calculate DB and redis pool metrics
	dbStats := b.db.Stats()
	redisStats := b.rp.Stats()
	dbWaitDurationInPeriod := dbStats.WaitDuration - b.dbWaitDuration
	redisWaitDurationInPeriod := redisStats.WaitDuration - b.redisWaitDuration
	b.dbWaitDuration = dbStats.WaitDuration
	b.redisWaitDuration = redisStats.WaitDuration

	hostDim := cwatch.Dimension("Host", b.config.InstanceID)
	metrics = append(metrics,
		cwatch.Datum("DBConnectionsInUse", float64(dbStats.InUse), cwtypes.StandardUnitCount, hostDim),
		cwatch.Datum("DBConnectionWaitDuration", float64(dbWaitDurationInPeriod)/float64(time.Second), cwtypes.StandardUnitSeconds, hostDim),
		cwatch.Datum("RedisConnectionsInUse", float64(redisStats.ActiveCount), cwtypes.StandardUnitCount, hostDim),
		cwatch.Datum("RedisConnectionsWaitDuration", float64(redisWaitDurationInPeriod)/float64(time.Second), cwtypes.StandardUnitSeconds, hostDim),
		cwatch.Datum("QueuedMsgs", float64(bulkSize), cwtypes.StandardUnitCount, cwatch.Dimension("QueueName", "bulk")),
		cwatch.Datum("QueuedMsgs", float64(prioritySize), cwtypes.StandardUnitCount, cwatch.Dimension("QueueName", "priority")),
	)

	if err := b.cw.Send(ctx, metrics...); err != nil {
		return 0, fmt.Errorf("error sending metrics: %w", err)
	}

	return len(metrics), nil
}

// Status returns information on our queue sizes, number of workers etc..
func (b *backend) Status() string {
	rc := b.rp.Get()
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
		channel, err := b.GetChannel(context.Background(), courier.AnyChannelType, channelUUID)
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
	return b.rp
}
