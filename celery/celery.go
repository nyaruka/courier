package celery

import (
	"encoding/base64"
	"encoding/json"

	"github.com/nyaruka/gocommon/uuids"

	"github.com/gomodule/redigo/redis"
)

// allows queuing a task to celery (with a redis backend)
//
// format to queue a new task to the queue named "handler" at normal priority is:
// "lpush" "handler" "{\"body\": \"W1tdLCB7fSwgeyJjYWxsYmFja3MiOiBudWxsLCAiZXJyYmFja3MiOiBudWxsLCAiY2hhaW4iOiBudWxsLCAiY2hvcmQiOiBudWxsfV0=\",
//	\"content-encoding\": \"utf-8\", \"content-type\": \"application/json\", \"headers\": {\"lang\": \"py\", \"task\": \"handle_event_task\",
//  \"id\": \"efca7c4e-952e-430f-87f7-c01c4652ed54\", \"eta\": null, \"expires\": null, \"group\": null, \"retries\": 0, \"timelimit\": [180, 120],
// \"root_id\": \"efca7c4e-952e-430f-87f7-c01c4652ed54\", \"parent_id\": null, \"argsrepr\": \"()\", \"kwargsrepr\": \"{}\",
// \"origin\": \"gen12382@ip-172-31-43-31\"}, \"properties\": {\"correlation_id\": \"efca7c4e-952e-430f-87f7-c01c4652ed54\",
// \"reply_to\": \"59ad710c-7d28-37c2-a730-89048c13f030\", \"delivery_mode\": 2, \"delivery_info\": {\"exchange\": \"\",
// \"routing_key\": \"handler\"}, \"priority\": 0, \"body_encoding\": \"base64\", \"delivery_tag\": \"bf838430-d01c-4550-b0a1-a6a309a28017\"}}"
//
// multi
// "zadd" "unacked_index" "1526928218.953298" "bf838430-d01c-4550-b0a1-a6a309a28017"
// "hset" "unacked" "bf838430-d01c-4550-b0a1-a6a309a28017" "[{\"body\":
//	\"W1tdLCB7fSwgeyJjYWxsYmFja3MiOiBudWxsLCAiZXJyYmFja3MiOiBudWxsLCAiY2hhaW4iOiBudWxsLCAiY2hvcmQiOiBudWxsfV0=\",
// \"content-encoding\": \"utf-8\", \"content-type\": \"application/json\", \"headers\": {\"lang\": \"py\", \"task\": \"handle_event_task\",
// \"id\": \"efca7c4e-952e-430f-87f7-c01c4652ed54\", \"eta\": null, \"expires\": null, \"group\": null, \"retries\": 0, \"timelimit\":
// [180, 120], \"root_id\": \"efca7c4e-952e-430f-87f7-c01c4652ed54\", \"parent_id\": null, \"argsrepr\": \"()\", \"kwargsrepr\": \"{}\",
// \"origin\": \"gen12382@ip-172-31-43-31\"}, \"properties\": {\"correlation_id\": \"efca7c4e-952e-430f-87f7-c01c4652ed54\",
// \"reply_to\": \"59ad710c-7d28-37c2-a730-89048c13f030\", \"delivery_mode\": 2, \"delivery_info\": {\"exchange\": \"\", \"routing_key\": \"handler\"},
// \"priority\": 0, \"body_encoding\": \"base64\", \"delivery_tag\": \"bf838430-d01c-4550-b0a1-a6a309a28017\"}}, \"\", \"handler\"]"
// exec
//
//

const defaultBody = `[[], {}, {"chord": null, "callbacks": null, "errbacks": null, "chain": null}]`

// QueueEmptyTask queues a new empty task with the passed in task name for the passed in queue
func QueueEmptyTask(rc redis.Conn, queueName string, taskName string) error {
	body := base64.StdEncoding.EncodeToString([]byte(defaultBody))
	taskUUID := string(uuids.New())
	deliveryTag := string(uuids.New())

	task := Task{
		Body: body,
		Headers: map[string]any{
			"root_id":    taskUUID,
			"id":         taskUUID,
			"lang":       "py",
			"kwargsrepr": "{}",
			"argsrepr":   "()",
			"task":       taskName,
			"expires":    nil,
			"eta":        nil,
			"group":      nil,
			"origin":     "courier@localhost",
			"parent_id":  nil,
			"retries":    0,
			"timelimit":  []int{180, 120},
		},
		ContentType: "application/json",
		Properties: TaskProperties{
			BodyEncoding:  "base64",
			CorrelationID: taskUUID,
			ReplyTo:       string(uuids.New()),
			DeliveryMode:  2,
			DeliveryTag:   deliveryTag,
			DeliveryInfo: TaskDeliveryInfo{
				RoutingKey: queueName,
			},
		},
		ContentEncoding: "utf-8",
	}

	taskJSON, err := json.Marshal(task)
	if err != nil {
		return err
	}

	rc.Send("lpush", queueName, string(taskJSON))
	return nil
}

// Task is the outer struct for a celery task
type Task struct {
	Body            string         `json:"body"`
	Headers         map[string]any `json:"headers"`
	ContentType     string         `json:"content-type"`
	Properties      TaskProperties `json:"properties"`
	ContentEncoding string         `json:"content-encoding"`
}

// TaskProperties is the struct for a task's properties
type TaskProperties struct {
	BodyEncoding  string           `json:"body_encoding"`
	CorrelationID string           `json:"correlation_id"`
	ReplyTo       string           `json:"reply_to"`
	DeliveryInfo  TaskDeliveryInfo `json:"delivery_info"`
	DeliveryMode  int              `json:"delivery_mode"`
	DeliveryTag   string           `json:"delivery_tag"`
	Priority      int              `json:"priority"`
}

// TaskDeliveryInfo is the struct for a task's delivery information
type TaskDeliveryInfo struct {
	RoutingKey string `json:"routing_key"`
	Exchange   string `json:"exchange"`
}
