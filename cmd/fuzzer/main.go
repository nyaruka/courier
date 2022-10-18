package main

import (
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier"
)

func main() {
	config := courier.LoadConfig("courier.toml")

	// parse and test our redis config
	redisURL, err := url.Parse(config.Redis)
	if err != nil {
		log.Fatalf("unable to parse Redis URL '%s': %s", config.Redis, err)
	}

	// create our pool
	redisPool := &redis.Pool{
		Wait:        true,              // makes callers wait for a connection
		MaxActive:   5,                 // only open this many concurrent connections at once
		MaxIdle:     2,                 // only keep up to 2 idle
		IdleTimeout: 240 * time.Second, // how long to wait before reaping a connection
		Dial: func() (redis.Conn, error) {
			conn, err := redis.Dial("tcp", redisURL.Host)
			if err != nil {
				return nil, err
			}

			// switch to the right DB
			_, err = conn.Do("SELECT", strings.TrimLeft(redisURL.Path, "/"))
			return conn, err
		},
	}

	// grab our connection
	conn := redisPool.Get()
	defer conn.Close()

	channelUUID := "dbc126ed-66bc-4e28-b67b-81dc3327c97d"

	msgJSON := `{
	  "channel_uuid": "dbc126ed-66bc-4e28-b67b-81dc3327c97d",
	  "text": "This is msg: %d",
	  "urn": "tel:+250788123123"
	}`

	// insert our messages
	for i := 0; i < 1000; i++ {
		json := fmt.Sprintf(msgJSON, i)
		_, err := conn.Do("ZADD", "msgs:"+channelUUID, 0.0, json)
		if err != nil {
			log.Fatalf("err inserting msg: %s", err)
		}
		_, err = conn.Do("ZINCRBY", "msgs:active", 0.0, channelUUID)
		if err != nil {
			log.Fatalf("err incrementing active: %s", err)
		}
	}
}
