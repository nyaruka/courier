package rapidpro

import (
	"context"
	"fmt"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/gocommon/jsonx"
)

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
	// create our event task
	eventJSON := jsonx.MustMarshal(mrTask{
		Type:     taskType,
		Task:     body,
		QueuedOn: time.Now(),
	})

	// create our org task
	contactJSON := jsonx.MustMarshal(mrTask{
		Type:     "handle_contact_event",
		Task:     mrContactTask{ContactID: contactID},
		QueuedOn: time.Now(),
	})

	now := time.Now().UTC()
	epochFloat := float64(now.UnixNano()) / float64(time.Second)

	// we do all our queueing in a transaction
	contactQueue := fmt.Sprintf("c:%d:%d", orgID, contactID)
	rc.Send("MULTI")
	rc.Send("RPUSH", contactQueue, eventJSON)
	rc.Send("ZADD", fmt.Sprintf("tasks:handler:%d", orgID), fmt.Sprintf("%.5f", epochFloat-10000000), contactJSON)
	rc.Send("ZINCRBY", "tasks:handler:active", 0, orgID)
	_, err = redis.DoContext(rc, ctx, "EXEC")

	return err
}

type mrContactTask struct {
	ContactID ContactID `json:"contact_id"`
}

type mrTask struct {
	Type     string    `json:"type"`
	Task     any       `json:"task"`
	QueuedOn time.Time `json:"queued_on"`
}
