package rapidpro

import (
	"context"
	"fmt"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/vkutil/queues"
)

var mrQueue = queues.NewFair("tasks:realtime", 100)

func queueMsgHandling(ctx context.Context, rc redis.Conn, c *models.Contact, m *MsgIn) error {
	channel := m.Channel().(*models.Channel)

	body := map[string]any{
		"channel_id":      channel.ID_,
		"msg_uuid":        m.UUID(),
		"msg_external_id": m.ExternalID(),
		"urn":             m.URN().String(),
		"urn_id":          m.ContactURNID_,
		"text":            m.Text(),
		"attachments":     m.Attachments(),
		"new_contact":     c.IsNew_,
	}

	return queueMailroomTask(ctx, rc, "msg_received", m.OrgID_, m.ContactID_, body)
}

func queueEventHandling(ctx context.Context, rc redis.Conn, c *models.Contact, e *ChannelEvent) error {
	body := map[string]any{
		"event_uuid":  e.UUID(),
		"event_type":  e.EventType_,
		"urn_id":      e.ContactURNID_,
		"channel_id":  e.ChannelID_,
		"extra":       e.Extra(),
		"new_contact": c.IsNew_,
		"occurred_on": e.OccurredOn_,
		"created_on":  e.CreatedOn_,
	}
	if e.OptInID_ != 0 {
		body["optin_id"] = e.OptInID_
	}

	return queueMailroomTask(ctx, rc, "event_received", e.OrgID_, e.ContactID_, body)
}

func queueMsgDeleted(ctx context.Context, rc redis.Conn, ch *models.Channel, msgUUID models.MsgUUID, contactID models.ContactID) error {
	return queueMailroomTask(ctx, rc, "msg_deleted", ch.OrgID_, contactID, map[string]any{"msg_uuid": msgUUID})
}

// queueMailroomTask queues the passed in task to mailroom. Mailroom processes both messages and
// channel event tasks through the same ordered queue.
func queueMailroomTask(ctx context.Context, rc redis.Conn, taskType string, orgID models.OrgID, contactID models.ContactID, body map[string]any) (err error) {
	eventJSON := jsonx.MustMarshal(mrTask{
		Type:     taskType,
		Task:     body,
		QueuedOn: time.Now(),
	})

	// push task onto the contact queue
	contactQueue := fmt.Sprintf("c:%d:%d", orgID, contactID)
	if _, err := redis.DoContext(rc, ctx, "RPUSH", contactQueue, eventJSON); err != nil {
		return fmt.Errorf("error pushing task onto contact queue: %w", err)
	}

	// create our org task
	contactJSON := jsonx.MustMarshal(mrTask{
		Type:     "handle_contact_event",
		Task:     mrContactTask{ContactID: contactID},
		QueuedOn: time.Now(),
	})

	if _, err := mrQueue.Push(ctx, rc, queues.OwnerID(fmt.Sprint(orgID)), true, contactJSON); err != nil {
		return fmt.Errorf("error pushing task onto org queue: %w", err)
	}

	return nil
}

type mrContactTask struct {
	ContactID models.ContactID `json:"contact_id"`
}

type mrTask struct {
	Type     string    `json:"type"`
	Task     any       `json:"task"`
	QueuedOn time.Time `json:"queued_on"`
}
