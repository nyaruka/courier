package models

import (
	"context"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/gocommon/cache"
	"github.com/nyaruka/gocommon/spools"
	"github.com/nyaruka/gocommon/syncx"
	"github.com/nyaruka/vkutil"
)

const (
	// MsgQueueName is the name of the queue that outgoing messages are popped from
	MsgQueueName = "msgs"

	// our timeout for looking up channels and contacts
	fetchTimeout = time.Second * 20
)

// state used by the read and write paths, initialized by Start
var (
	systemUserID UserID

	channelsByUUID *cache.Local[ChannelUUID, *Channel]
	channelsByAddr *cache.Local[ChannelAddress, *Channel]

	// spools of items which couldn't be written to the database and will be retried later
	msgSpool    *spools.Spool[*MsgIn]
	statusSpool *spools.Spool[*StatusUpdate]
	eventSpool  *spools.Spool[*ChannelEvent]

	statusWriter *StatusWriter
	writerWG     *sync.WaitGroup
)

// valkey backed trackers which hold no local state
var (
	mediaCache   = vkutil.NewIntervalHash("media-lookups", time.Hour*24, 2)
	mediaMutexes = syncx.NewHashMutex(8)

	// tracking of recent messages received to avoid creating duplicates
	receivedExternalIDs = vkutil.NewIntervalHash("seen-external-ids", time.Hour*24, 2) // using external id, 24 - 48 hours
	receivedMsgs        = vkutil.NewIntervalHash("seen-msgs", time.Second*2, 2)        // using content hash, 2 - 4 seconds

	// tracking of sent message ids to avoid dupe sends
	sentMsgs = vkutil.NewIntervalSet("sent-msgs", time.Hour, 2) // using msg UUID, 1 - 2 hours

	// tracking of external ids of messages we've sent in case we need one before its status update has been written
	sentExternalIDs = vkutil.NewIntervalHash("sent-external-ids", time.Hour, 2) // 1 - 2 hours
)

// Start creates and starts the channel caches, spools and batched writers used by the read and write paths
func Start(rt *runtime.Runtime) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var err error

	// store the system user id
	systemUserID, err = GetSystemUserID(ctx, rt.DB)
	if err != nil {
		return fmt.Errorf("error looking up system user: %w", err)
	}

	// create and start channel caches...
	channelsByUUID = cache.NewLocal(func(ctx context.Context, uuid ChannelUUID) (*Channel, error) {
		return loadChannelByUUID(ctx, rt, uuid)
	}, time.Minute)
	channelsByUUID.Start()
	channelsByAddr = cache.NewLocal(func(ctx context.Context, addr ChannelAddress) (*Channel, error) {
		return loadChannelByAddress(ctx, rt, addr)
	}, time.Minute)
	channelsByAddr.Start()

	// create our spools and start their background flushing - their Start fails if a spool directory isn't
	// writable so a misconfigured spool volume can't silently drop items during database outages
	msgSpool = spools.New(path.Join(rt.Config.SpoolDir, "msgs"), 30*time.Second, spools.MarshalJSON, spools.UnmarshalJSON,
		func(ctx context.Context, batch []*MsgIn) ([]*MsgIn, error) { return flushMsgs(ctx, rt, batch) },
	)
	statusSpool = spools.New(path.Join(rt.Config.SpoolDir, "statuses"), 30*time.Second, spools.MarshalJSON, spools.UnmarshalJSON,
		func(ctx context.Context, batch []*StatusUpdate) ([]*StatusUpdate, error) {
			return flushStatuses(ctx, rt, batch)
		},
	)
	eventSpool = spools.New(path.Join(rt.Config.SpoolDir, "events"), 30*time.Second, spools.MarshalJSON, spools.UnmarshalJSON,
		func(ctx context.Context, batch []*ChannelEvent) ([]*ChannelEvent, error) {
			return flushEvents(ctx, rt, batch)
		},
	)
	if err := msgSpool.Start(); err != nil {
		return err
	}
	if err := statusSpool.Start(); err != nil {
		return err
	}
	if err := eventSpool.Start(); err != nil {
		return err
	}

	// create our batched status writer and start it
	writerWG = &sync.WaitGroup{}
	statusWriter = NewStatusWriter(rt)
	statusWriter.Start(writerWG)

	return nil
}

// Stop stops the channel caches, spools and batched writers
func Stop() {
	channelsByUUID.Stop()
	channelsByAddr.Stop()

	// stop our batched status writer and wait for it to flush fully
	statusWriter.Stop()
	writerWG.Wait()

	// stop our spools' background flushing (after the status writer since its failures are spooled)
	msgSpool.Stop()
	statusSpool.Stop()
	eventSpool.Stop()
}

// SpoolSizes returns the number of items in the msg, status and event spools
func SpoolSizes() (int, int, int) {
	return msgSpool.Size(), statusSpool.Size(), eventSpool.Size()
}
