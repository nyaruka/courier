package rapidpro

import (
	"fmt"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/gocommon/jsonx"
)

func queueMsgHandling(rc redis.Conn, c *Contact, m *Msg) error {
	channel := m.Channel().(*Channel)

	// queue to mailroom
	body := map[string]any{
		"contact_id":      c.ID_,
		"org_id":          channel.OrgID_,
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

	return queueMailroomTask(rc, "msg_event", m.OrgID_, m.ContactID_, body)
}

func queueChannelEvent(rc redis.Conn, c *Contact, e *ChannelEvent) error {
	// queue to mailroom
	switch e.EventType() {
	case courier.EventTypeStopContact:
		body := map[string]any{
			"org_id":      e.OrgID_,
			"contact_id":  e.ContactID_,
			"occurred_on": e.OccurredOn_,
		}
		return queueMailroomTask(rc, "stop_contact", e.OrgID_, e.ContactID_, body)

	case courier.EventTypeWelcomeMessage:
		body := map[string]any{
			"org_id":      e.OrgID_,
			"contact_id":  e.ContactID_,
			"urn_id":      e.ContactURNID_,
			"channel_id":  e.ChannelID_,
			"new_contact": c.IsNew_,
			"occurred_on": e.OccurredOn_,
		}
		return queueMailroomTask(rc, "welcome_message", e.OrgID_, e.ContactID_, body)

	case courier.EventTypeReferral:
		body := map[string]any{
			"org_id":      e.OrgID_,
			"contact_id":  e.ContactID_,
			"urn_id":      e.ContactURNID_,
			"channel_id":  e.ChannelID_,
			"extra":       e.Extra(),
			"new_contact": c.IsNew_,
			"occurred_on": e.OccurredOn_,
		}
		return queueMailroomTask(rc, "referral", e.OrgID_, e.ContactID_, body)

	case courier.EventTypeNewConversation:
		body := map[string]any{
			"org_id":      e.OrgID_,
			"contact_id":  e.ContactID_,
			"urn_id":      e.ContactURNID_,
			"channel_id":  e.ChannelID_,
			"extra":       e.Extra(),
			"new_contact": c.IsNew_,
			"occurred_on": e.OccurredOn_,
		}
		return queueMailroomTask(rc, "new_conversation", e.OrgID_, e.ContactID_, body)

	case courier.EventTypeOptIn:
		body := map[string]any{
			"org_id":      e.OrgID_,
			"contact_id":  e.ContactID_,
			"urn_id":      e.ContactURNID_,
			"channel_id":  e.ChannelID_,
			"extra":       e.Extra(),
			"occurred_on": e.OccurredOn_,
		}
		return queueMailroomTask(rc, "optin", e.OrgID_, e.ContactID_, body)

	case courier.EventTypeOptOut:
		body := map[string]any{
			"org_id":      e.OrgID_,
			"contact_id":  e.ContactID_,
			"urn_id":      e.ContactURNID_,
			"channel_id":  e.ChannelID_,
			"extra":       e.Extra(),
			"occurred_on": e.OccurredOn_,
		}
		return queueMailroomTask(rc, "optout", e.OrgID_, e.ContactID_, body)

	default:
		return fmt.Errorf("unknown event type: %s", e.EventType())
	}
}

func queueMsgDeleted(rc redis.Conn, ch *Channel, msgID courier.MsgID, contactID ContactID) error {
	return queueMailroomTask(rc, "msg_deleted", ch.OrgID_, contactID, map[string]any{"org_id": ch.OrgID_, "msg_id": msgID})
}

// queueMailroomTask queues the passed in task to mailroom. Mailroom processes both messages and
// channel event tasks through the same ordered queue.
func queueMailroomTask(rc redis.Conn, taskType string, orgID OrgID, contactID ContactID, body map[string]any) (err error) {
	// create our event task
	eventJSON := jsonx.MustMarshal(mrTask{
		Type:     taskType,
		OrgID:    orgID,
		Task:     body,
		QueuedOn: time.Now(),
	})

	// create our org task
	contactJSON := jsonx.MustMarshal(mrTask{
		Type:     "handle_contact_event",
		OrgID:    orgID,
		Task:     mrContactTask{ContactID: contactID},
		QueuedOn: time.Now(),
	})

	now := time.Now().UTC()
	epochFloat := float64(now.UnixNano()) / float64(time.Second)

	// we do all our queueing in a transaction
	contactQueue := fmt.Sprintf("c:%d:%d", orgID, contactID)
	rc.Send("multi")
	rc.Send("rpush", contactQueue, eventJSON)
	rc.Send("zadd", fmt.Sprintf("handler:%d", orgID), fmt.Sprintf("%.5f", epochFloat-10000000), contactJSON)
	rc.Send("zincrby", "handler:active", 0, orgID)
	_, err = rc.Do("EXEC")

	return err
}

type mrContactTask struct {
	ContactID ContactID `json:"contact_id"`
}

type mrTask struct {
	Type     string    `json:"type"`
	OrgID    OrgID     `json:"org_id"`
	Task     any       `json:"task"`
	QueuedOn time.Time `json:"queued_on"`
}
