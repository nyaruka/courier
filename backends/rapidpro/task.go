package rapidpro

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/celery"
)

func queueTask(rc redis.Conn, queueName string, taskName string, orgID OrgID, subQueue string, body map[string]interface{}) (err error) {
	// encode our body
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	epochFloat := float64(now.UnixNano()) / float64(time.Second)

	// we do all our queueing in a transaction
	rc.Send("multi")
	if subQueue != "" {
		rc.Send("zadd", subQueue, fmt.Sprintf("%.5f", epochFloat), bodyJSON)
	}
	rc.Send("zadd", fmt.Sprintf("%s:%d", taskName, orgID.Int64), fmt.Sprintf("%.5f", epochFloat-10000000), bodyJSON)
	rc.Send("zincrby", fmt.Sprintf("%s:active", taskName), 0, orgID.Int64)
	celery.QueueEmptyTask(rc, queueName, taskName)
	_, err = rc.Do("exec")

	return err
}

func queueMsgHandling(rc redis.Conn, c *DBContact, m *DBMsg) error {
	channel := m.Channel().(*DBChannel)

	// flow server enabled orgs go to mailroom
	if channel.OrgFlowServerEnabled() {
		body := map[string]interface{}{
			"contact_id":      c.ID_.Int64,
			"org_id":          channel.OrgID_.Int64,
			"channel_id":      channel.ID_.Int64,
			"msg_id":          m.ID_.Int64,
			"msg_uuid":        m.UUID_.String(),
			"msg_external_id": m.ExternalID(),
			"urn":             m.URN().String(),
			"urn_id":          m.ContactURNID_.Int64,
			"text":            m.Text(),
			"attachments":     m.Attachments(),
			"new_contact":     c.IsNew_,
		}

		return queueMailroomTask(rc, "msg_event", m.OrgID_, m.ContactID_, body)

	}

	body := map[string]interface{}{
		"type":        "msg",
		"id":          m.ID_.Int64,
		"contact_id":  c.ID_.Int64,
		"new_message": true,
		"new_contact": c.IsNew_,
	}

	return queueTask(rc, "handler", "handle_event_task", m.OrgID_, fmt.Sprintf("ch:%d", c.ID_.Int64), body)
}

func queueChannelEvent(rc redis.Conn, c *DBContact, e *DBChannelEvent) error {
	channel := e.Channel()

	// flow server enabled orgs go to mailroom
	if channel.OrgFlowServerEnabled() {
		switch e.EventType() {
		case courier.StopContact:
			body := map[string]interface{}{
				"org_id":     e.OrgID_.Int64,
				"contact_id": e.ContactID_.Int64,
			}
			return queueMailroomTask(rc, "stop_event", e.OrgID_, e.ContactID_, body)

		case courier.Referral:
			body := map[string]interface{}{
				"org_id":      e.OrgID_.Int64,
				"contact_id":  e.ContactID_.Int64,
				"channel_id":  e.ChannelID_.Int64,
				"extra":       e.Extra(),
				"new_contact": c.IsNew_,
			}
			return queueMailroomTask(rc, "referral", e.OrgID_, e.ContactID_, body)

		case courier.NewConversation:
			body := map[string]interface{}{
				"org_id":      e.OrgID_.Int64,
				"contact_id":  e.ContactID_.Int64,
				"channel_id":  e.ChannelID_.Int64,
				"extra":       e.Extra(),
				"new_contact": c.IsNew_,
			}
			return queueMailroomTask(rc, "new_conversation", e.OrgID_, e.ContactID_, body)

		default:
			return fmt.Errorf("unknown event type: %s", e.EventType())
		}
	}

	body := map[string]interface{}{
		"type":       "channel_event",
		"contact_id": e.ContactID_.Int64,
		"event_id":   e.ID_.Int64,
	}

	return queueTask(rc, "handler", "handle_event_task", e.OrgID_, "", body)
}

// queueMailroomTask queues the passed in task to mailroom. Mailroom processes both messages and
// channel event tasks through the same ordered queue.
func queueMailroomTask(rc redis.Conn, taskType string, orgID OrgID, contactID ContactID, body map[string]interface{}) (err error) {
	// create our event task
	eventTask := mrTask{
		Type:  taskType,
		OrgID: orgID.Int64,
		Task:  body,
	}

	eventJSON, err := json.Marshal(eventTask)
	if err != nil {
		return err
	}

	// create our org task
	contactTask := mrTask{
		Type:  "handle_event",
		OrgID: orgID.Int64,
		Task: mrContactTask{
			ContactID: contactID.Int64,
		},
	}

	contactJSON, err := json.Marshal(contactTask)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	epochFloat := float64(now.UnixNano()) / float64(time.Second)

	// we do all our queueing in a transaction
	contactQueue := fmt.Sprintf("c:%d:%d", orgID.Int64, contactID.Int64)
	rc.Send("multi")
	rc.Send("rpush", contactQueue, eventJSON)
	rc.Send("zadd", fmt.Sprintf("handler:%d", orgID.Int64), fmt.Sprintf("%.5f", epochFloat-10000000), contactJSON)
	rc.Send("zincrby", "handler:active", 0, orgID.Int64)
	_, err = rc.Do("exec")

	return err
}

type mrContactTask struct {
	ContactID int64 `json:"contact_id"`
}

type mrTask struct {
	Type  string      `json:"type"`
	OrgID int64       `json:"org_id"`
	Task  interface{} `json:"task"`
}
