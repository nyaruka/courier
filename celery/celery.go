package celery

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/garyburd/redigo/redis"
	"github.com/satori/go.uuid"
)

// allows queuing a task to celery (with a redis backend)
//
// format to queue a new task to the queue named "handler" at normal priority is:
//  "SADD" "_kombu.binding.handler" "handler\x06\x16\x06\x16handler"
//  "LPUSH" "handler" "{\"body\": \"W1tdLCB7fSwgeyJjaG9yZCI6IG51bGwsICJjYWxsYmFja3MiOiBudWxsLCAiZXJyYmFja3MiOiBudWxsLCAiY2hhaW4iOiBudWxsfV0=\",
//	  \"headers\": {\"origin\": \"gen15039@lagom.local\", \"root_id\": \"adc1f782-c356-4aa1-acc8-238c2b348cac\", \"expires\": null,
//    \"id\": \"adc1f782-c356-4aa1-acc8-238c2b348cac\", \"kwargsrepr\": \"{}\", \"lang\": \"py\", \"retries\": 0, \"task\": \"handle_event_task\",
//    \"group\": null, \"timelimit\": [null, null], \"parent_id\": null, \"argsrepr\": \"()\", \"eta\": null}, \"content-type\": \"application/json\",
//    \"properties\": {\"priority\": 0, \"body_encoding\": \"base64\", \"correlation_id\": \"adc1f782-c356-4aa1-acc8-238c2b348cac\",
//	  \"reply_to\": \"ec9440ce-1983-3e62-958b-65241f83235b\", \"delivery_info\": {\"routing_key\": \"handler\", \"exchange\": \"\"},
//    \"delivery_mode\": 2, \"delivery_tag\": \"6e43def1-ed8e-4d06-93c5-9ec9a4695eb0\"}, \"content-encoding\": \"utf-8\"}"

const defaultBody = `[[], {}, {"chord": null, "callbacks": null, "errbacks": null, "chain": null}]`

// QueueEmptyTask queues a new empty task with the passed in task name for the passed in queue
func QueueEmptyTask(rc redis.Conn, queueName string, taskName string) error {
	body := base64.StdEncoding.EncodeToString([]byte(defaultBody))
	taskUUID := uuid.NewV4().String()

	task := Task{
		Body: body,
		Headers: map[string]interface{}{
			"root_id":    taskUUID,
			"id":         taskUUID,
			"lang":       "py",
			"kwargsrepr": "{}",
			"argsrepr":   "()",
			"task":       taskName,
			"expires":    nil,
		},
		ContentType: "application/json",
		Properties: TaskProperties{
			BodyEncoding:  "base64",
			CorrelationID: taskUUID,
			ReplyTo:       uuid.NewV4().String(),
			DeliveryMode:  2,
			DeliveryTag:   uuid.NewV4().String(),
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

	rc.Send("sadd", fmt.Sprintf("_kombu.binding.%s", queueName), fmt.Sprintf("%s\x06\x16\x06\x16%s", queueName, queueName))
	rc.Send("lpush", queueName, string(taskJSON))
	return nil
}

// Task is the outer struct for a celery task
type Task struct {
	Body            string                 `json:"body"`
	Headers         map[string]interface{} `json:"headers"`
	ContentType     string                 `json:"content-type"`
	Properties      TaskProperties         `json:"properties"`
	ContentEncoding string                 `json:"content-encoding"`
}

// TaskProperties is the struct for a task's properties
type TaskProperties struct {
	BodyEncoding  string           `json:"body_encoding"`
	CorrelationID string           `json:"correlation_id"`
	ReplyTo       string           `json:"replay_to"`
	DeliveryInfo  TaskDeliveryInfo `json:"delivery_info"`
	DeliveryMode  int              `json:"delivery_mode"`
	DeliveryTag   string           `json:"delivery_tag"`
}

// TaskDeliveryInfo is the struct for a task's delivery information
type TaskDeliveryInfo struct {
	Priority   int    `json:"priority"`
	RoutingKey string `json:"routing_key"`
	Exchange   string `json:"exchange"`
}
