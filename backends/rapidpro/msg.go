package rapidpro

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gomodule/redigo/redis"
	filetype "github.com/h2non/filetype"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/gocommon/dbutil"
)

func msgHash(m *models.MsgIn) string {
	hash := sha1.Sum([]byte(m.Text_ + "|" + strings.Join(m.Attachments_, "|")))
	return hex.EncodeToString(hash[:])
}

// WriteMsg creates a message given the passed in arguments
func writeMsg(ctx context.Context, b *backend, m *models.MsgIn, clog *models.ChannelLog) error {
	channel := m.Channel()

	// check for data: attachment URLs which need to be fetched now - fetching of other URLs can be deferred until
	// message handling and performed by calling the /ci/attachment/fetch endpoint
	for i, attURL := range m.Attachments_ {
		if strings.HasPrefix(attURL, "data:") {
			attData, err := base64.StdEncoding.DecodeString(attURL[5:])
			if err != nil {
				clog.Error(models.ErrorAttachmentNotDecodable())
				return fmt.Errorf("unable to decode attachment data: %w", err)
			}

			var contentType, extension string
			fileType, _ := filetype.Match(attData[:300])
			if fileType != filetype.Unknown {
				contentType = fileType.MIME.Value
				extension = fileType.Extension
			} else {
				contentType = "application/octet-stream"
				extension = "bin"
			}

			newURL, err := b.SaveAttachment(ctx, channel, contentType, attData, extension)
			if err != nil {
				return err
			}
			m.Attachments_[i] = fmt.Sprintf("%s:%s", contentType, newURL)
		}
	}

	// try to write it our db
	contact, err := writeMsgToDB(ctx, b, m, clog)
	if err != nil {
		if dbutil.IsUniqueViolation(err) {
			slog.Warn("duplicate incoming message detected, ignoring", "msg", m.UUID())
			return nil
		}

		// if we failed, log and write to spool
		slog.Error("error writing to db", "error", err, "msg", m.UUID())

		if err := b.msgSpool.Add([]*models.MsgIn{m}); err != nil {
			return fmt.Errorf("error writing msg to spool: %w", err)
		}
		return nil
	}

	rc := b.rt.VK.Get()
	defer rc.Close()

	// queue to mailroom for handling
	if err := queueMsgHandling(ctx, rc, contact, m); err != nil {
		slog.Error("error queueing msg handling", "error", err, "msg", m.UUID_, "contact", contact.ID_)
	}

	return err
}

func writeMsgToDB(ctx context.Context, b *backend, m *models.MsgIn, clog *models.ChannelLog) (*models.Contact, error) {
	contact, err := contactForMsg(ctx, b, m, clog)

	if err != nil {
		// our db is down, write to the spool, we will write/queue this later
		return nil, fmt.Errorf("error getting contact for message: %w", err)
	}

	// set our contact and urn id
	m.ContactID_ = contact.ID_
	m.ContactURNID_ = contact.URNID_

	if err := models.InsertIncomingMsg(ctx, b.rt.DB, m); err != nil {
		return nil, fmt.Errorf("error inserting message: %w", err)
	}

	return contact, nil
}

//-----------------------------------------------------------------------------
// Msg flusher for flushing failed writes
//-----------------------------------------------------------------------------

// flushMsgs is the flush function for the msg spool - it retries writing spooled msgs to the database, returning
// those that fail again so they're respooled
func (b *backend) flushMsgs(ctx context.Context, batch []*models.MsgIn) ([]*models.MsgIn, error) {
	var failed []*models.MsgIn

	for _, m := range batch {
		ctx, cancel := context.WithTimeout(ctx, time.Second*30)
		err := b.flushMsg(ctx, m)
		cancel()

		if err != nil {
			slog.Error("error flushing spooled msg", "error", err, "msg", m.UUID())
			failed = append(failed, m)
		}
	}

	return failed, nil
}

func (b *backend) flushMsg(ctx context.Context, m *models.MsgIn) error {
	// look up our channel
	channel, err := b.GetChannel(ctx, models.AnyChannelType, m.ChannelUUID_)
	if err != nil {
		return err
	}
	m.Channel_ = channel

	// create log tho it won't be written
	clog := models.NewChannelLog(models.ChannelLogTypeMsgReceive, channel, nil, nil)

	// try to write it our db
	contact, err := writeMsgToDB(ctx, b, m, clog)
	if err != nil {
		if dbutil.IsUniqueViolation(err) {
			slog.Warn("duplicate incoming message detected, ignoring", "msg", m.UUID())
			return nil
		}
		return err // fail? oh well, we'll try again later
	}

	rc := b.rt.VK.Get()
	defer rc.Close()

	// queue to mailroom for handling
	if err := queueMsgHandling(ctx, rc, contact, m); err != nil {
		slog.Error("error queueing handling for de-spooled message", "error", err, "msg", m.UUID_, "contact", contact.ID_)
	}

	return nil
}

//-----------------------------------------------------------------------------
// Deduping utility methods
//-----------------------------------------------------------------------------

// checks to see if this message has already been received and if so returns its UUID
func (b *backend) checkMsgAlreadyReceived(ctx context.Context, m *models.MsgIn) models.MsgUUID {
	rc := b.rt.VK.Get()
	defer rc.Close()

	// if we have an external id use that
	if m.ExternalIdentifier_ != "" {
		fingerprint := fmt.Sprintf("%s|%s|%s", m.Channel().UUID(), m.URN().Identity(), m.ExternalID())

		if uuid, _ := b.receivedExternalIDs.Get(ctx, rc, fingerprint); uuid != "" {
			return models.MsgUUID(uuid)
		}
	} else {
		// otherwise de-dup based on text received from that channel+urn since last send
		fingerprint := fmt.Sprintf("%s|%s", m.Channel().UUID(), m.URN().Identity())

		if uuidAndHash, _ := b.receivedMsgs.Get(ctx, rc, fingerprint); uuidAndHash != "" {
			prevUUID := uuidAndHash[:36]
			prevHash := uuidAndHash[37:]

			// if it is the same hash, return the UUID
			if prevHash == msgHash(m) {
				return models.MsgUUID(prevUUID)
			}
		}
	}

	return ""
}

// records that the given message has been received and written to the database
func (b *backend) recordMsgReceived(ctx context.Context, m *models.MsgIn) {
	rc := b.rt.VK.Get()
	defer rc.Close()

	if m.ExternalIdentifier_ != "" {
		fingerprint := fmt.Sprintf("%s|%s|%s", m.Channel().UUID(), m.URN().Identity(), m.ExternalID())

		if err := b.receivedExternalIDs.Set(ctx, rc, fingerprint, string(m.UUID())); err != nil {
			slog.Error("error recording received external id", "msg", m.UUID(), "error", err)
		}
	} else {
		fingerprint := fmt.Sprintf("%s|%s", m.Channel().UUID(), m.URN().Identity())

		if err := b.receivedMsgs.Set(ctx, rc, fingerprint, fmt.Sprintf("%s|%s", m.UUID(), msgHash(m))); err != nil {
			slog.Error("error recording received msg", "msg", m.UUID(), "error", err)
		}
	}
}

// clearMsgSeen clears our seen incoming messages for the passed in channel and URN
func (b *backend) clearMsgSeen(ctx context.Context, vc redis.Conn, m *models.MsgOut) {
	fingerprint := fmt.Sprintf("%s|%s", m.Channel().UUID(), m.URN().Identity())

	if err := b.receivedMsgs.Del(ctx, vc, fingerprint); err != nil {
		slog.Error("error clearing received msgs", "urn", m.URN().Identity(), "error", err)
	}
}
