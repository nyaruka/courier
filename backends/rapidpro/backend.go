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
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/courier/runtime"
	"github.com/nyaruka/courier/utils/queue"
	"github.com/nyaruka/gocommon/aws/cwatch"
	"github.com/nyaruka/gocommon/aws/dynamo"
	"github.com/nyaruka/gocommon/cache"
	"github.com/nyaruka/gocommon/dbutil"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/syncx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/vkutil"
)

const (
	appNodesRunningKey = "app-nodes:running"

	// the name for our message queue
	msgQueueName = "msgs"

	// our timeout for backend operations
	backendTimeout = time.Second * 20
)

var uuidRegex = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

type backend struct {
	rt *runtime.Runtime

	systemUserID models.UserID

	statusWriter *StatusWriter
	writerWG     *sync.WaitGroup

	channelsByUUID *cache.Local[models.ChannelUUID, *models.Channel]
	channelsByAddr *cache.Local[models.ChannelAddress, *models.Channel]

	stopChan  chan bool
	waitGroup *sync.WaitGroup

	httpClient         *http.Client
	httpClientInsecure *http.Client
	httpAccess         *httpx.AccessConfig

	mediaCache   *vkutil.IntervalHash
	mediaMutexes syncx.HashMutex

	// tracking of recent messages received to avoid creating duplicates
	receivedExternalIDs *vkutil.IntervalHash // using external id
	receivedMsgs        *vkutil.IntervalHash // using content hash

	// tracking of sent message ids to avoid dupe sends
	sentIDs *vkutil.IntervalSet

	// tracking of external ids of messages we've sent in case we need one before its status update has been written
	sentExternalIDs *vkutil.IntervalHash

	stats *StatsCollector

	// both sqlx and redis provide wait stats which are cummulative that we need to convert into increments by
	// tracking their previous values
	dbWaitDuration    time.Duration
	redisWaitDuration time.Duration
}

// NewBackend creates a new RapidPro backend
func NewBackend(rt *runtime.Runtime) courier.Backend {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 64
	transport.MaxIdleConnsPerHost = 8
	transport.IdleConnTimeout = 15 * time.Second

	insecureTransport := http.DefaultTransport.(*http.Transport).Clone()
	insecureTransport.MaxIdleConns = 64
	insecureTransport.MaxIdleConnsPerHost = 8
	insecureTransport.IdleConnTimeout = 15 * time.Second
	insecureTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	disallowedIPs, disallowedNets, _ := rt.Config.ParseDisallowedNetworks()

	return &backend{
		rt: rt,

		httpClient:         &http.Client{Transport: transport, Timeout: 30 * time.Second},
		httpClientInsecure: &http.Client{Transport: insecureTransport, Timeout: 30 * time.Second},
		httpAccess:         httpx.NewAccessConfig(10*time.Second, disallowedIPs, disallowedNets),

		stopChan:  make(chan bool),
		waitGroup: &sync.WaitGroup{},

		writerWG: &sync.WaitGroup{},

		mediaCache:   vkutil.NewIntervalHash("media-lookups", time.Hour*24, 2),
		mediaMutexes: *syncx.NewHashMutex(8),

		receivedMsgs:        vkutil.NewIntervalHash("seen-msgs", time.Second*2, 2),        // 2 - 4 seconds
		receivedExternalIDs: vkutil.NewIntervalHash("seen-external-ids", time.Hour*24, 2), // 24 - 48 hours
		sentIDs:             vkutil.NewIntervalSet("sent-ids", time.Hour, 2),              // 1 - 2 hours
		sentExternalIDs:     vkutil.NewIntervalHash("sent-external-ids", time.Hour, 2),    // 1 - 2 hours

		stats: NewStatsCollector(),
	}
}

// Start starts our RapidPro backend, this tests our various connections and starts our spool flushers
func (b *backend) Start() error {
	log := slog.With("comp", "backend")
	log.Info("backend starting")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// test Postgres
	if err := b.rt.DB.PingContext(ctx); err != nil {
		log.Error("db not reachable", "error", err)
	} else {
		log.Info("db ok")
	}

	// test DynamoDB
	if err := dynamo.Test(ctx, b.rt.Dynamo, b.rt.Config.DynamoTablePrefix+"Main"); err != nil {
		log.Error("dynamodb not reachable", "error", err)
	} else {
		log.Info("dynamodb ok")
	}

	// test Valkey
	vc := b.rt.VK.Get()
	defer vc.Close()
	if _, err := vc.Do("PING"); err != nil {
		log.Error("valkey not reachable", "error", err)
	} else {
		log.Info("valkey ok")
	}

	// test S3 bucket access
	if err := b.rt.S3.Test(ctx, b.rt.Config.S3AttachmentsBucket); err != nil {
		log.Error("attachments bucket not accessible", "error", err)
	} else {
		log.Info("attachments bucket ok")
	}

	if err := b.rt.Start(); err != nil {
		return fmt.Errorf("error starting runtime: %w", err)
	} else {
		log.Info("runtime started")
	}

	var err error

	// start our dethrottler if we are going to be doing some sending
	if b.rt.Config.MaxWorkers > 0 {
		queue.StartDethrottler(b.rt.VK, b.stopChan, b.waitGroup, msgQueueName)
	}

	// create and start channel caches...
	b.channelsByUUID = cache.NewLocal(func(ctx context.Context, uuid models.ChannelUUID) (*models.Channel, error) {
		return models.GetChannelByUUID(ctx, b.rt, uuid)
	}, time.Minute)
	b.channelsByUUID.Start()
	b.channelsByAddr = cache.NewLocal(func(ctx context.Context, addr models.ChannelAddress) (*models.Channel, error) {
		return models.GetChannelByAddress(ctx, b.rt, addr)
	}, time.Minute)
	b.channelsByAddr.Start()

	// make sure our spool dirs are writable
	err = courier.EnsureSpoolDirPresent(b.rt.Config.SpoolDir, "msgs")
	if err == nil {
		err = courier.EnsureSpoolDirPresent(b.rt.Config.SpoolDir, "statuses")
	}
	if err == nil {
		err = courier.EnsureSpoolDirPresent(b.rt.Config.SpoolDir, "events")
	}
	if err != nil {
		log.Error("spool directories not writable", "error", err)
	} else {
		log.Info("spool directories ok")
	}

	// create our batched writers and start them
	b.statusWriter = NewStatusWriter(b, b.rt.Config.SpoolDir)
	b.statusWriter.Start(b.writerWG)

	// store the system user id
	b.systemUserID, err = models.GetSystemUserID(ctx, b.rt.DB)
	if err != nil {
		return err
	}

	// register and start our spool flushers
	courier.RegisterFlusher(path.Join(b.rt.Config.SpoolDir, "msgs"), b.flushMsgFile)
	courier.RegisterFlusher(path.Join(b.rt.Config.SpoolDir, "statuses"), b.flushStatusFile)
	courier.RegisterFlusher(path.Join(b.rt.Config.SpoolDir, "events"), b.flushChannelEventFile)

	b.startMetricsReporter(time.Minute)

	if err := b.checkLastShutdown(ctx); err != nil {
		return err
	}

	log.Info("backend started")
	return nil
}

func (b *backend) checkLastShutdown(ctx context.Context) error {
	nodeID := fmt.Sprintf("courier:%s", b.rt.Config.InstanceID)
	vc := b.rt.VK.Get()
	defer vc.Close()

	exists, err := redis.Bool(redis.DoContext(vc, ctx, "HEXISTS", appNodesRunningKey, nodeID))
	if err != nil {
		return fmt.Errorf("error checking last shutdown: %w", err)
	}

	if exists {
		slog.Error("node did not shutdown cleanly last time")
	} else {
		if _, err := redis.DoContext(vc, ctx, "HSET", appNodesRunningKey, nodeID, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return fmt.Errorf("error setting app node state: %w", err)
		}
	}
	return nil
}

func (b *backend) recordShutdown(ctx context.Context) error {
	nodeID := fmt.Sprintf("courier:%s", b.rt.Config.InstanceID)
	vc := b.rt.VK.Get()
	defer vc.Close()

	if _, err := redis.DoContext(vc, ctx, "HDEL", appNodesRunningKey, nodeID); err != nil {
		return fmt.Errorf("error recording shutdown: %w", err)
	}
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
	log := slog.With("comp", "backend")
	log.Info("backend stopping")

	// close our stop channel
	close(b.stopChan)

	b.channelsByUUID.Stop()
	b.channelsByAddr.Stop()

	// wait for our threads to exit
	b.waitGroup.Wait()

	// stop our batched writers
	if b.statusWriter != nil {
		b.statusWriter.Stop()
	}

	// wait for them to flush fully
	b.writerWG.Wait()

	b.rt.Stop()

	if err := b.recordShutdown(context.TODO()); err != nil {
		return fmt.Errorf("error recording shutdown: %w", err)
	}

	log.Info("backend stopped")
	return nil
}

// GetChannel returns the channel for the passed in type and UUID
func (b *backend) GetChannel(ctx context.Context, typ models.ChannelType, uuid models.ChannelUUID) (courier.Channel, error) {
	timeout, cancel := context.WithTimeout(ctx, backendTimeout)
	defer cancel()

	ch, err := b.channelsByUUID.GetOrFetch(timeout, uuid)
	if err != nil {
		return nil, err // so we don't return a non-nil interface and nil ptr
	}

	if typ != models.AnyChannelType && ch.ChannelType() != typ {
		return nil, models.ErrChannelWrongType
	}

	return ch, nil
}

// GetChannelByAddress returns the channel with the passed in type and address
func (b *backend) GetChannelByAddress(ctx context.Context, typ models.ChannelType, address models.ChannelAddress) (courier.Channel, error) {
	timeout, cancel := context.WithTimeout(ctx, backendTimeout)
	defer cancel()

	ch, err := b.channelsByAddr.GetOrFetch(timeout, address)
	if err != nil {
		return nil, err // so we don't return a non-nil interface and nil ptr
	}

	if typ != models.AnyChannelType && ch.ChannelType() != typ {
		return nil, models.ErrChannelWrongType
	}

	return ch, nil
}

// GetContact returns the contact for the passed in channel and URN
func (b *backend) GetContact(ctx context.Context, c courier.Channel, urn urns.URN, authTokens map[string]string, name string, allowCreate bool, clog *courier.ChannelLog) (courier.Contact, error) {
	dbChannel := c.(*models.Channel)
	return contactForURN(ctx, b, dbChannel.OrgID_, dbChannel, urn, authTokens, name, allowCreate, clog)
}

// AddURNtoContact adds a URN to the passed in contact
func (b *backend) AddURNtoContact(ctx context.Context, c courier.Channel, contact courier.Contact, urn urns.URN, authTokens map[string]string) (urns.URN, error) {
	tx, err := b.rt.DB.BeginTxx(ctx, nil)
	if err != nil {
		return urns.NilURN, err
	}
	dbChannel := c.(*models.Channel)
	dbContact := contact.(*models.Contact)
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
	dbContact := contact.(*models.Contact)
	_, err := b.rt.DB.ExecContext(ctx, `UPDATE contacts_contacturn SET contact_id = NULL WHERE contact_id = $1 AND identity = $2`, dbContact.ID_, urn.Identity().String())
	if err != nil {
		return urns.NilURN, err
	}
	return urn, nil
}

// DeleteMsgByExternalID resolves a message external id and quees a task to mailroom to delete it
func (b *backend) DeleteMsgByExternalID(ctx context.Context, channel courier.Channel, externalID string) error {
	ch := channel.(*models.Channel)
	row := b.rt.DB.QueryRowContext(ctx, `SELECT id, contact_id FROM msgs_msg WHERE channel_id = $1 AND external_id = $2 AND direction = 'I'`, ch.ID(), externalID)

	var msgID models.MsgID
	var contactID models.ContactID
	if err := row.Scan(&msgID, &contactID); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("error querying deleted msg: %w", err)
	}

	if msgID != models.NilMsgID && contactID != models.NilContactID {
		rc := b.rt.VK.Get()
		defer rc.Close()

		if err := queueMsgDeleted(ctx, rc, ch, msgID, contactID); err != nil {
			return fmt.Errorf("error queuing message deleted task: %w", err)
		}
	}

	return nil
}

// NewIncomingMsg creates a new message from the given params
func (b *backend) NewIncomingMsg(ctx context.Context, channel courier.Channel, urn urns.URN, text string, extID string, clog *courier.ChannelLog) courier.MsgIn {
	// strip out invalid UTF8 and NULL chars
	urn = urns.URN(dbutil.ToValidUTF8(string(urn)))
	text = dbutil.ToValidUTF8(text)
	extID = dbutil.ToValidUTF8(extID)

	msg := newMsg(models.MsgIncoming, channel, urn, text, extID, clog)
	msg.WithReceivedOn(time.Now().UTC())

	// check if this message could be a duplicate and if so use the original's UUID
	if prevUUID := b.checkMsgAlreadyReceived(ctx, msg); prevUUID != models.NilMsgUUID {
		msg.UUID_ = prevUUID
		msg.alreadyWritten = true
	}

	return msg
}

// PopNextOutgoingMsg pops the next message that needs to be sent
func (b *backend) PopNextOutgoingMsg(ctx context.Context) (courier.MsgOut, error) {
	tryToPop := func() (queue.WorkerToken, string, error) {
		rc := b.rt.VK.Get()
		defer rc.Close()
		return queue.PopFromQueue(rc, msgQueueName)
	}

	markComplete := func(token queue.WorkerToken) {
		rc := b.rt.VK.Get()
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
	channel, err := b.GetChannel(ctx, models.AnyChannelType, dbMsg.ChannelUUID_)
	if err != nil {
		markComplete(token)
		return nil, err
	}

	dbMsg.Direction_ = models.MsgOutgoing
	dbMsg.channel = channel.(*models.Channel)
	dbMsg.workerToken = token

	// clear out our seen incoming messages
	b.clearMsgSeen(ctx, dbMsg)

	return dbMsg, nil
}

// WasMsgSent returns whether the passed in message has already been sent
func (b *backend) WasMsgSent(ctx context.Context, id models.MsgID) (bool, error) {
	rc := b.rt.VK.Get()
	defer rc.Close()

	return b.sentIDs.IsMember(ctx, rc, id.String())
}

func (b *backend) ClearMsgSent(ctx context.Context, id models.MsgID) error {
	rc := b.rt.VK.Get()
	defer rc.Close()

	return b.sentIDs.Rem(ctx, rc, id.String())
}

// OnSendComplete is called when the sender has finished trying to send a message
func (b *backend) OnSendComplete(ctx context.Context, msg courier.MsgOut, status courier.StatusUpdate, clog *courier.ChannelLog) {
	log := slog.With("channel", msg.Channel().UUID(), "msg", msg.UUID(), "clog", clog.UUID, "status", status)

	rc := b.rt.VK.Get()
	defer rc.Close()

	dbMsg := msg.(*Msg)

	if err := queue.MarkComplete(rc, msgQueueName, dbMsg.workerToken); err != nil {
		log.Error("unable to mark queue task complete", "error", err)
	}

	// if message won't be retried, mark as sent to avoid dupe sends
	if status.Status() != models.MsgStatusErrored {
		if err := b.sentIDs.Add(ctx, rc, msg.ID().String()); err != nil {
			log.Error("unable to mark message sent", "error", err)
		}
	}

	// if message was successfully sent, and we have a session timeout, update it
	wasSuccess := status.Status() == models.MsgStatusWired || status.Status() == models.MsgStatusSent || status.Status() == models.MsgStatusDelivered || status.Status() == models.MsgStatusRead
	if wasSuccess && dbMsg.Session_ != nil && dbMsg.Session_.Timeout > 0 {
		if err := b.insertTimeoutFire(ctx, dbMsg); err != nil {
			log.Error("unable to update session timeout", "error", err, "session_uuid", dbMsg.Session_.UUID)
		}
	}

	b.stats.RecordOutgoing(msg.Channel().ChannelType(), wasSuccess, clog.Elapsed)
}

// OnReceiveComplete is called when the server has finished handling an incoming request
func (b *backend) OnReceiveComplete(ctx context.Context, ch courier.Channel, events []courier.Event, clog *courier.ChannelLog) {
	b.stats.RecordIncoming(ch.ChannelType(), events, clog.Elapsed)
}

// WriteMsg writes the passed in message to our store
func (b *backend) WriteMsg(ctx context.Context, msg courier.MsgIn, clog *courier.ChannelLog) error {
	m := msg.(*Msg)

	timeout, cancel := context.WithTimeout(ctx, backendTimeout)
	defer cancel()

	if err := writeMsg(timeout, b, m, clog); err != nil {
		return err
	}

	b.recordMsgReceived(ctx, m)

	return nil
}

// NewStatusUpdateForID creates a new Status object for the given message id
func (b *backend) NewStatusUpdate(channel courier.Channel, id models.MsgID, status models.MsgStatus, clog *courier.ChannelLog) courier.StatusUpdate {
	return newStatusUpdate(channel, id, "", status, clog)
}

// NewStatusUpdateForID creates a new Status object for the given message id
func (b *backend) NewStatusUpdateByExternalID(channel courier.Channel, externalID string, status models.MsgStatus, clog *courier.ChannelLog) courier.StatusUpdate {
	return newStatusUpdate(channel, models.NilMsgID, externalID, status, clog)
}

// WriteStatusUpdate writes the passed in MsgStatus to our store
func (b *backend) WriteStatusUpdate(ctx context.Context, status courier.StatusUpdate) error {
	log := slog.With("msg_id", status.MsgID(), "msg_external_id", status.ExternalID(), "status", status.Status())
	su := status.(*models.StatusUpdate)

	if status.MsgID() == models.NilMsgID && status.ExternalID() == "" {
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

	if status.MsgID() != models.NilMsgID {
		// this is a message we've just sent and were given an external id for
		if status.ExternalID() != "" {
			rc := b.rt.VK.Get()
			defer rc.Close()

			err := b.sentExternalIDs.Set(ctx, rc, fmt.Sprintf("%d|%s", su.ChannelID_, su.ExternalID_), fmt.Sprintf("%d", status.MsgID()))
			if err != nil {
				log.Error("error recording external id", "error", err)
			}
		}

		// we sent a message that errored so clear our sent flag to allow it to be retried
		if status.Status() == models.MsgStatusErrored {
			err := b.ClearMsgSent(ctx, status.MsgID())
			if err != nil {
				log.Error("error clearing sent flags", "error", err)
			}
		}
	}

	// queue the status to written by the batch writer
	b.statusWriter.Queue(status.(*models.StatusUpdate))
	log.Debug("status update queued")

	return nil
}

// updateContactURN updates contact URN according to the old/new URNs from status
func (b *backend) updateContactURN(ctx context.Context, status courier.StatusUpdate) error {
	old, new := status.URNUpdate()

	// retrieve channel
	channel, err := b.GetChannel(ctx, models.AnyChannelType, status.ChannelUUID())
	if err != nil {
		return fmt.Errorf("error retrieving channel: %w", err)
	}
	dbChannel := channel.(*models.Channel)
	tx, err := b.rt.DB.BeginTxx(ctx, nil)
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
	if newContactURN.ContactID == models.NilContactID {
		newContactURN.ContactID = oldContactURN.ContactID
	}
	// remove contact association from old URN
	oldContactURN.ContactID = models.NilContactID

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
func (b *backend) NewChannelEvent(channel courier.Channel, eventType models.ChannelEventType, urn urns.URN, clog *courier.ChannelLog) courier.ChannelEvent {
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

	orgID := ch.(*models.Channel).OrgID()

	path := filepath.Join("attachments", strconv.FormatInt(int64(orgID), 10), filename[:4], filename[4:8], filename)

	storageURL, err := b.rt.S3.PutObject(ctx, b.rt.Config.S3AttachmentsBucket, path, contentType, data, s3types.ObjectCannedACLPublicRead)
	if err != nil {
		return "", fmt.Errorf("error saving attachment to storage (bytes=%d): %w", len(data), err)
	}

	return storageURL, nil
}

// ResolveMedia resolves the passed in attachment URL to a media object
func (b *backend) ResolveMedia(ctx context.Context, mediaUrl string) (*models.Media, error) {
	u, err := url.Parse(mediaUrl)
	if err != nil {
		return nil, fmt.Errorf("error parsing media URL: %w", err)
	}

	mediaUUID := uuidRegex.FindString(u.Path)

	// if hostname isn't our media domain, or path doesn't contain a UUID, don't try to resolve
	if strings.Replace(u.Hostname(), fmt.Sprintf("%s.", b.rt.Config.AWSRegion), "", -1) != b.rt.Config.MediaDomain || mediaUUID == "" {
		return nil, nil
	}

	unlock := b.mediaMutexes.Lock(mediaUUID)
	defer unlock()

	rc := b.rt.VK.Get()
	defer rc.Close()

	var media *models.Media
	mediaJSON, err := b.mediaCache.Get(ctx, rc, mediaUUID)
	if err != nil {
		return nil, fmt.Errorf("error looking up cached media: %w", err)
	}
	if mediaJSON != "" {
		jsonx.MustUnmarshal([]byte(mediaJSON), &media)
	} else {
		// lookup media in our database
		media, err = models.LoadMediaByUUID(ctx, b.rt.DB, uuids.UUID(mediaUUID))
		if err != nil {
			return nil, fmt.Errorf("error looking up media: %w", err)
		}

		// cache it for future requests
		b.mediaCache.Set(ctx, rc, mediaUUID, string(jsonx.MustMarshal(media)))
	}

	// if we found a media record but it doesn't match the URL, don't use it
	if media == nil || (media.URL() != mediaUrl && media.URL() != strings.Replace(mediaUrl, fmt.Sprintf("%s.", b.rt.Config.AWSRegion), "", -1)) {
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
	rc, redisErr := b.rt.VK.GetContext(ctx)
	cancel()

	if redisErr == nil {
		defer rc.Close()
		_, redisErr = rc.Do("PING")
	}

	// test our db
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	dbErr := b.rt.DB.PingContext(ctx)
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
	if b.rt.Config.MetricsReporting == "off" {
		return 0, nil
	}

	metrics := b.stats.Extract().ToMetrics(b.rt.Config.MetricsReporting == "advanced")

	// get queue sizes
	rc := b.rt.VK.Get()
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
	dbStats := b.rt.DB.Stats()
	redisStats := b.rt.VK.Stats()
	dbWaitDurationInPeriod := dbStats.WaitDuration - b.dbWaitDuration
	redisWaitDurationInPeriod := redisStats.WaitDuration - b.redisWaitDuration
	b.dbWaitDuration = dbStats.WaitDuration
	b.redisWaitDuration = redisStats.WaitDuration

	hostDim := cwatch.Dimension("Host", b.rt.Config.InstanceID)
	metrics = append(metrics,
		cwatch.Datum("DBConnectionsInUse", float64(dbStats.InUse), cwtypes.StandardUnitCount, hostDim),
		cwatch.Datum("DBConnectionWaitDuration", float64(dbWaitDurationInPeriod)/float64(time.Second), cwtypes.StandardUnitSeconds, hostDim),
		cwatch.Datum("ValkeyConnectionsInUse", float64(redisStats.ActiveCount), cwtypes.StandardUnitCount, hostDim),
		cwatch.Datum("ValkeyConnectionsWaitDuration", float64(redisWaitDurationInPeriod)/float64(time.Second), cwtypes.StandardUnitSeconds, hostDim),
		cwatch.Datum("QueuedMsgs", float64(bulkSize), cwtypes.StandardUnitCount, cwatch.Dimension("QueueName", "bulk")),
		cwatch.Datum("QueuedMsgs", float64(prioritySize), cwtypes.StandardUnitCount, cwatch.Dimension("QueueName", "priority")),
		cwatch.Datum("DynamoSpooledItems", float64(b.rt.Spool.Size()), cwtypes.StandardUnitCount, hostDim),
	)

	if err := b.rt.CW.Send(ctx, metrics...); err != nil {
		return 0, fmt.Errorf("error sending metrics: %w", err)
	}

	return len(metrics), nil
}

// Status returns information on our queue sizes, number of workers etc..
func (b *backend) Status() string {
	rc := b.rt.VK.Get()
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
		channelUUID := models.ChannelUUID(uuid)
		channel, err := b.GetChannel(context.Background(), models.AnyChannelType, channelUUID)
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
	return b.rt.VK
}
