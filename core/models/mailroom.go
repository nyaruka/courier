package models

import (
	"context"
	"fmt"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/queues"
)

var mrQueue = queues.NewFairV2("tasks:realtime", 100)

func queueMsgHandling(ctx context.Context, rc redis.Conn, c *Contact, m *MsgIn) error {
	channel := m.Channel()

	body := map[string]any{
		"channel_id":      channel.ID_,
		"msg_uuid":        m.UUID(),
		"msg_external_id": m.ExternalID(),
		"urn":             m.URN().String(),
		"urn_id":          c.URNID_,
		"text":            m.Text(),
		"attachments":     m.Attachments(),
		"new_contact":     c.IsNew_,
	}
	if m.NewURN_ != nil {
		body["new_urn"] = m.NewURN_
	}
	if len(m.Payload_) > 0 {
		body["payload"] = m.Payload_
	}

	return queueMailroomTask(ctx, rc, "msg_received", channel.OrgID_, c.ID_, body)
}

func queueEventHandling(ctx context.Context, rc redis.Conn, c *Contact, e *ChannelEvent) error {
	channel := e.Channel()

	body := map[string]any{
		"event_uuid":  e.UUID(),
		"event_type":  e.EventType_,
		"urn_id":      c.URNID_,
		"channel_id":  channel.ID_,
		"extra":       e.Extra(),
		"new_contact": c.IsNew_,
		"occurred_on": e.OccurredOn_,
	}

	return queueMailroomTask(ctx, rc, "event_received", channel.OrgID_, c.ID_, body)
}

func queueMsgDeleted(ctx context.Context, rc redis.Conn, ch *Channel, msgUUID MsgUUID, contactID ContactID) error {
	return queueMailroomTask(ctx, rc, "msg_deleted", ch.OrgID_, contactID, map[string]any{"msg_uuid": msgUUID})
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

	if _, err := mrQueue.Push(ctx, rc, queues.OwnerID(fmt.Sprint(orgID)), true, contactJSON); err != nil {
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
