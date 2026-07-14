package rapidpro

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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
	"github.com/nyaruka/courier/v26"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/utils"
	"github.com/nyaruka/courier/v26/utils/queue"
	"github.com/nyaruka/gocommon/aws/cwatch"
	"github.com/nyaruka/gocommon/aws/dynamo"
	"github.com/nyaruka/gocommon/cache"
	"github.com/nyaruka/gocommon/dbutil"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/spools"
	"github.com/nyaruka/gocommon/syncx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/vkutil"
)

const (
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

	// spools of items which couldn't be written to the database and will be retried later
	msgSpool    *spools.Spool[*MsgIn]
	statusSpool *spools.Spool[*models.StatusUpdate]
	eventSpool  *spools.Spool[*ChannelEvent]

	channelsByUUID *cache.Local[models.ChannelUUID, *models.Channel]
	channelsByAddr *cache.Local[models.ChannelAddress, *models.Channel]

	stopChan  chan bool
	waitGroup *sync.WaitGroup

	mediaCache   *vkutil.IntervalHash
	mediaMutexes syncx.HashMutex

	// tracking of recent messages received to avoid creating duplicates
	receivedExternalIDs *vkutil.IntervalHash // using external id
	receivedMsgs        *vkutil.IntervalHash // using content hash

	// tracking of sent message ids to avoid dupe sends
	sentMsgs *vkutil.IntervalSet // using msg UUID

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
	return &backend{
		rt: rt,

		stopChan:  make(chan bool),
		waitGroup: &sync.WaitGroup{},

		writerWG: &sync.WaitGroup{},

		mediaCache:   vkutil.NewIntervalHash("media-lookups", time.Hour*24, 2),
		mediaMutexes: *syncx.NewHashMutex(8),

		receivedMsgs:        vkutil.NewIntervalHash("seen-msgs", time.Second*2, 2),        // 2 - 4 seconds
		receivedExternalIDs: vkutil.NewIntervalHash("seen-external-ids", time.Hour*24, 2), // 24 - 48 hours
		sentMsgs:            vkutil.NewIntervalSet("sent-msgs", time.Hour, 2),             // 1 - 2 hours
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

	// test that the Centrifugo server is reachable and accepts our key
	if err := b.rt.Centrifugo.Client.Info(ctx); err != nil {
		log.Error("centrifugo not reachable", "error", err)
	} else {
		log.Info("centrifugo ok")
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

	// create our spools and start their background flushing - their Start fails if a spool directory isn't
	// writable so a misconfigured spool volume can't silently drop items during database outages
	b.msgSpool = spools.New(path.Join(b.rt.Config.SpoolDir, "msgs"), 30*time.Second, spools.MarshalJSON, spools.UnmarshalJSON, b.flushMsgs)
	b.statusSpool = spools.New(path.Join(b.rt.Config.SpoolDir, "statuses"), 30*time.Second, spools.MarshalJSON, spools.UnmarshalJSON, b.flushStatuses)
	b.eventSpool = spools.New(path.Join(b.rt.Config.SpoolDir, "events"), 30*time.Second, spools.MarshalJSON, spools.UnmarshalJSON, b.flushEvents)
	if err := b.msgSpool.Start(); err != nil {
		return err
	}
	if err := b.statusSpool.Start(); err != nil {
		return err
	}
	if err := b.eventSpool.Start(); err != nil {
		return err
	}
	log.Info("spools ok")

	// create our batched writers and start them
	b.statusWriter = NewStatusWriter(b)
	b.statusWriter.Start(b.writerWG)

	// store the system user id
	b.systemUserID, err = models.GetSystemUserID(ctx, b.rt.DB)
	if err != nil {
		return err
	}

	b.startMetricsReporter(time.Minute)

	log.Info("backend started")
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

	// stop our spools' background flushing (after the status writer since its failures are spooled)
	b.msgSpool.Stop()
	b.statusSpool.Stop()
	b.eventSpool.Stop()

	b.rt.Stop()

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

// DeleteMsgByExternalID resolves a message external id and queues a task to mailroom to delete it
// GetMsgExternalIdentifier returns the platform's identifier for the given incoming message
func (b *backend) GetMsgExternalIdentifier(ctx context.Context, channel courier.Channel, uuid models.MsgUUID) (string, error) {
	ch := channel.(*models.Channel)

	var extID string
	err := b.rt.DB.GetContext(ctx, &extID, `SELECT COALESCE(external_identifier, '') FROM msgs_msg WHERE uuid = $1 AND channel_id = $2 AND direction = 'I'`, uuid, ch.ID())
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("no incoming message with UUID %s on channel %s", uuid, ch.UUID())
	} else if err != nil {
		return "", fmt.Errorf("error querying msg external identifier: %w", err)
	}
	return extID, nil
}

func (b *backend) DeleteMsgByExternalID(ctx context.Context, channel courier.Channel, externalID string) error {
	ch := channel.(*models.Channel)
	row := b.rt.DB.QueryRowContext(ctx, `SELECT uuid, contact_id FROM msgs_msg WHERE channel_id = $1 AND external_identifier = $2 AND direction = 'I'`, ch.ID(), externalID)

	var msgUUID models.MsgUUID
	var contactID models.ContactID
	if err := row.Scan(&msgUUID, &contactID); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("error querying deleted msg: %w", err)
	}

	if msgUUID != "" && contactID != models.NilContactID {
		rc := b.rt.VK.Get()
		defer rc.Close()

		if err := queueMsgDeleted(ctx, rc, ch, msgUUID, contactID); err != nil {
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

	ch := channel.(*models.Channel)

	msg := models.NewIncomingMsg(ch, urn, text, extID, clog.UUID)

	return &MsgIn{MsgIn: msg, ChannelUUID_: channel.UUID(), URN_: urn, channel: ch}
}

// PopNextOutgoingMsg pops the next message that needs to be sent
func (b *backend) PopNextOutgoingMsg(ctx context.Context) (courier.MsgOut, error) {
	vc := b.rt.VK.Get()
	defer vc.Close()

	markComplete := func(token queue.WorkerToken) {
		if err := queue.MarkComplete(vc, msgQueueName, token); err != nil {
			slog.Error("error marking queue task complete", "error", err)
		}
	}

	// pop the next message off our queue
	token, msgJSON, err := queue.PopFromQueue(vc, msgQueueName)
	if err != nil {
		return nil, err
	}

	for token == queue.Retry {
		token, msgJSON, err = queue.PopFromQueue(vc, msgQueueName)
		if err != nil {
			return nil, err
		}
	}

	if msgJSON == "" {
		return nil, nil
	}

	msg := &MsgOut{}
	if err := json.Unmarshal([]byte(msgJSON), msg); err != nil {
		markComplete(token)
		return nil, fmt.Errorf("unable to unmarshal message: %s: %w", string(msgJSON), err)
	}

	if err := utils.Validate(msg); err != nil {
		markComplete(token)
		return nil, fmt.Errorf("queued message failed validation: %s: %w", string(msgJSON), err)
	}

	// populate the channel on our msg object
	channel, err := b.GetChannel(ctx, models.AnyChannelType, msg.ChannelUUID_)
	if err != nil {
		markComplete(token)
		return nil, err
	}

	// add some extra info to the popped message
	msg.channel = channel.(*models.Channel)
	msg.workerToken = token

	// clear out our seen incoming messages
	b.clearMsgSeen(ctx, vc, msg)

	return msg, nil
}

// WasMsgSent returns whether the passed in message has already been sent
func (b *backend) WasMsgSent(ctx context.Context, uuid models.MsgUUID) (bool, error) {
	rc := b.rt.VK.Get()
	defer rc.Close()

	return b.sentMsgs.IsMember(ctx, rc, string(uuid))
}

func (b *backend) ClearMsgSent(ctx context.Context, uuid models.MsgUUID) error {
	rc := b.rt.VK.Get()
	defer rc.Close()

	return b.sentMsgs.Rem(ctx, rc, string(uuid))
}

// OnSendComplete is called when the sender has finished trying to send a message
func (b *backend) OnSendComplete(ctx context.Context, msg courier.MsgOut, status courier.StatusUpdate, res *courier.SendResult, clog *courier.ChannelLog) {
	log := slog.With("channel", msg.Channel().UUID(), "msg", msg.UUID(), "clog", clog.UUID, "status", status)

	rc := b.rt.VK.Get()
	defer rc.Close()

	m := msg.(*MsgOut)

	if err := queue.MarkComplete(rc, msgQueueName, m.workerToken); err != nil {
		log.Error("unable to mark queue task complete", "error", err)
	}

	// if message won't be retried, mark as sent to avoid dupe sends
	if status.Status() != models.MsgStatusErrored {
		if err := b.sentMsgs.Add(ctx, rc, string(msg.UUID())); err != nil {
			log.Error("unable to mark message sent", "error", err)
		}
	}

	// if message was successfully sent, and we have a session timeout, update it
	wasSuccess := status.Status() == models.MsgStatusWired || status.Status() == models.MsgStatusSent || status.Status() == models.MsgStatusDelivered || status.Status() == models.MsgStatusRead
	if wasSuccess && m.Session_ != nil && m.Session_.Timeout > 0 {
		if err := b.insertTimeoutFire(ctx, m); err != nil {
			log.Error("unable to update session timeout", "error", err, "session_uuid", m.Session_.UUID)
		}
	}

	// if send result includes a new URN to add to the contact, queue a contact_changed task
	if wasSuccess && res != nil && res.NewURN() != urns.NilURN && !msg.Contact().HasOtherURN(res.NewURN()) {
		dbChannel := msg.Channel().(*models.Channel)
		err := queueMailroomTask(ctx, rc, "contact_changed", dbChannel.OrgID_, msg.Contact().ID, map[string]any{
			"channel_id": dbChannel.ID_,
			"new_urn": map[string]string{
				"value":  res.NewURN().String(),
				"action": "append",
			},
		})
		if err != nil {
			log.Error("unable to queue contact_changed task", "error", err)
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
	m := msg.(*MsgIn)

	// check if this message could be a duplicate and if so steal the original's UUID
	if prevUUID := b.checkMsgAlreadyReceived(ctx, m); prevUUID != "" {
		m.UUID_ = prevUUID
		m.duplicate = true
		return nil
	}

	timeout, cancel := context.WithTimeout(ctx, backendTimeout)
	defer cancel()

	if err := writeMsg(timeout, b, m, clog); err != nil {
		return err
	}

	b.recordMsgReceived(ctx, m)

	return nil
}

// NewStatusUpdateForID creates a new Status object for the given message id
func (b *backend) NewStatusUpdate(channel courier.Channel, uuid models.MsgUUID, status models.MsgStatus, clog *courier.ChannelLog) courier.StatusUpdate {
	return newStatusUpdate(channel, uuid, "", status, clog)
}

// NewStatusUpdateForID creates a new Status object for the given message id
func (b *backend) NewStatusUpdateByExternalID(channel courier.Channel, externalID string, status models.MsgStatus, clog *courier.ChannelLog) courier.StatusUpdate {
	return newStatusUpdate(channel, "", externalID, status, clog)
}

// WriteStatusUpdate writes the passed in MsgStatus to our store
func (b *backend) WriteStatusUpdate(ctx context.Context, status courier.StatusUpdate) error {
	log := slog.With("msg_uuid", status.MsgUUID(), "msg_external_id", status.ExternalIdentifier(), "status", status.Status())
	su := status.(*models.StatusUpdate)

	if status.MsgUUID() == "" && status.ExternalIdentifier() == "" {
		return errors.New("message status with no UUID or external id")
	}

	if status.MsgUUID() != "" {
		// this is a message we've just sent and were given an external id for
		if status.ExternalIdentifier() != "" {
			rc := b.rt.VK.Get()
			defer rc.Close()

			err := b.sentExternalIDs.Set(ctx, rc, fmt.Sprintf("%d|%s", su.ChannelID_, su.ExternalIdentifier_), string(status.MsgUUID()))
			if err != nil {
				log.Error("error recording external id", "error", err)
			}
		}

		// we sent a message that errored so clear our sent flag to allow it to be retried
		if status.Status() == models.MsgStatusErrored {
			err := b.ClearMsgSent(ctx, status.MsgUUID())
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
	if b.stripS3Region(u.Hostname()) != b.rt.Config.MediaDomain || mediaUUID == "" {
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
	if media == nil || (media.URL() != mediaUrl && media.URL() != b.stripS3Region(mediaUrl)) {
		return nil, nil
	}

	return media, nil
}

// strips the region qualifier that S3 adds to virtual-host style URLs so they can be compared against our
// unqualified media domain and URLs, e.g. foo.s3.us-east-1.amazonaws.com becomes foo.s3.amazonaws.com. No-op
// when there's no region (e.g. local dev setups using path-style URLs).
func (b *backend) stripS3Region(s string) string {
	if b.rt.S3.Region == "" {
		return s
	}
	return strings.Replace(s, fmt.Sprintf("%s.", b.rt.S3.Region), "", -1)
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

	// instance level metrics are published without an instance dimension so that instances (which come and go with
	// deploys) are just samples of the same metric, and can be aggregated with statistics like Max and Sum
	metrics = append(metrics,
		cwatch.Datum("DBConnectionsInUse", float64(dbStats.InUse), cwtypes.StandardUnitCount),
		cwatch.Datum("DBConnectionWaitDuration", float64(dbWaitDurationInPeriod)/float64(time.Second), cwtypes.StandardUnitSeconds),
		cwatch.Datum("ValkeyConnectionsInUse", float64(redisStats.ActiveCount), cwtypes.StandardUnitCount),
		cwatch.Datum("ValkeyConnectionsWaitDuration", float64(redisWaitDurationInPeriod)/float64(time.Second), cwtypes.StandardUnitSeconds),
		cwatch.Datum("QueuedMsgs", float64(bulkSize), cwtypes.StandardUnitCount, cwatch.Dimension("QueueName", "bulk")),
		cwatch.Datum("QueuedMsgs", float64(prioritySize), cwtypes.StandardUnitCount, cwatch.Dimension("QueueName", "priority")),
		cwatch.Datum("DynamoSpooledItems", float64(b.rt.Spool.Size()), cwtypes.StandardUnitCount),
		cwatch.Datum("PostgresSpooledItems", float64(b.msgSpool.Size()), cwtypes.StandardUnitCount, cwatch.Dimension("SpoolName", "msgs")),
		cwatch.Datum("PostgresSpooledItems", float64(b.statusSpool.Size()), cwtypes.StandardUnitCount, cwatch.Dimension("SpoolName", "statuses")),
		cwatch.Datum("PostgresSpooledItems", float64(b.eventSpool.Size()), cwtypes.StandardUnitCount, cwatch.Dimension("SpoolName", "events")),
	)

	if err := b.rt.CW.Send(ctx, metrics...); err != nil {
		return 0, fmt.Errorf("error sending metrics: %w", err)
	}

	return len(metrics), nil
}

// RedisPool returns the redisPool for this backend
func (b *backend) RedisPool() *redis.Pool {
	return b.rt.VK
}
