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

func queueMsgHandling(rc redis.Conn, orgID OrgID, contactID ContactID, msgID courier.MsgID, newContact bool) error {
	body := map[string]interface{}{
		"type":        "msg",
		"id":          msgID.Int64,
		"contact_id":  contactID.Int64,
		"new_message": true,
		"new_contact": newContact,
	}

	return queueTask(rc, "handler", "handle_event_task", orgID, fmt.Sprintf("ch:%d", contactID.Int64), body)
}

func queueChannelEvent(rc redis.Conn, orgID OrgID, contactID ContactID, eventID ChannelEventID) error {
	body := map[string]interface{}{
		"type":       "channel_event",
		"contact_id": contactID.Int64,
		"event_id":   eventID.Int64,
	}

	return queueTask(rc, "handler", "handle_event_task", orgID, "", body)
}
