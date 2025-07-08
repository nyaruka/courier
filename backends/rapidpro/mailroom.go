package rapidpro

import (
	"context"
	"fmt"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/vkutil/queues"
)

var mrQueue = queues.NewFair("tasks-realtime", 100)

func queueMsgHandling(ctx context.Context, rc redis.Conn, c *Contact, m *Msg) error {
	channel := m.Channel().(*Channel)

	body := map[string]any{
		"channel_id":      channel.ID_,
		"msg_id":          m.ID_,
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

func queueEventHandling(ctx context.Context, rc redis.Conn, c *Contact, e *ChannelEvent) error {
	body := map[string]any{
		"event_id":    e.ID_,
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

func queueMsgDeleted(ctx context.Context, rc redis.Conn, ch *Channel, msgID courier.MsgID, contactID ContactID) error {
	return queueMailroomTask(ctx, rc, "msg_deleted", ch.OrgID_, contactID, map[string]any{"msg_id": msgID})
}

// queueMailroomTask queues the passed in task to mailroom. Mailroom processes both messages and
// channel event tasks through the same ordered queue.
func queueMailroomTask(ctx context.Context, rc redis.Conn, taskType string, orgID OrgID, contactID ContactID, body map[string]any) (err error) {
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

	if err := mrQueue.Push(ctx, rc, fmt.Sprint(orgID), false, contactJSON); err != nil {
		return fmt.Errorf("error pushing task onto org queue: %w", err)
	}

	return nil
}

type mrContactTask struct {
	ContactID ContactID `json:"contact_id"`
}

type mrTask struct {
	Type     string    `json:"type"`
	Task     any       `json:"task"`
	QueuedOn time.Time `json:"queued_on"`
}
