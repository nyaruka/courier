package courier

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/gocommon/aws/cwatch"
)

func (s *Server) startMetricsReporter(interval time.Duration) {
	s.waitGroup.Add(1)

	// both sqlx and redis provide wait stats which are cummulative that we need to convert into increments by
	// tracking their previous values
	var dbWaitDuration, redisWaitDuration time.Duration

	report := func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		count, err := s.reportMetrics(ctx, &dbWaitDuration, &redisWaitDuration)
		cancel()
		if err != nil {
			slog.Error("error reporting metrics", "error", err)
		} else {
			slog.Info("sent metrics to cloudwatch", "count", count)
		}
	}

	go func() {
		defer func() {
			slog.Info("metrics reporter exiting")
			s.waitGroup.Done()
		}()

		for {
			select {
			case <-s.stopChan:
				report()
				return
			case <-time.After(interval):
				report()
			}
		}
	}()
}

func (s *Server) reportMetrics(ctx context.Context, dbWaitDuration, redisWaitDuration *time.Duration) (int, error) {
	if s.rt.Config.MetricsReporting == "off" {
		return 0, nil
	}

	metrics := s.rt.Stats.Extract().ToMetrics(s.rt.Config.MetricsReporting == "advanced")

	// get queue sizes
	rc := s.rt.VK.Get()
	defer rc.Close()
	active, err := redis.Strings(rc.Do("ZRANGE", fmt.Sprintf("%s:active", models.MsgQueueName), "0", "-1"))
	if err != nil {
		return 0, fmt.Errorf("error getting active queues: %w", err)
	}
	throttled, err := redis.Strings(rc.Do("ZRANGE", fmt.Sprintf("%s:throttled", models.MsgQueueName), "0", "-1"))
	if err != nil {
		return 0, fmt.Errorf("error getting throttled queues: %w", err)
	}
	queues := append(active, throttled...)

	prioritySize := 0
	bulkSize := 0
	for _, queue := range queues {
		q := fmt.Sprintf("%s/1", queue)
		count, err := redis.Int(rc.Do("ZCARD", q))
		if err != nil {
			return 0, fmt.Errorf("error getting size of priority queue: %s: %w", q, err)
		}
		prioritySize += count

		q = fmt.Sprintf("%s/0", queue)
		count, err = redis.Int(rc.Do("ZCARD", q))
		if err != nil {
			return 0, fmt.Errorf("error getting size of bulk queue: %s: %w", q, err)
		}
		bulkSize += count
	}

	// calculate DB and redis pool metrics
	dbStats := s.rt.DB.Stats()
	redisStats := s.rt.VK.Stats()
	dbWaitDurationInPeriod := dbStats.WaitDuration - *dbWaitDuration
	redisWaitDurationInPeriod := redisStats.WaitDuration - *redisWaitDuration
	*dbWaitDuration = dbStats.WaitDuration
	*redisWaitDuration = redisStats.WaitDuration

	msgSpoolSize, statusSpoolSize, eventSpoolSize := models.SpoolSizes()

	// instance level metrics are published without an instance dimension so that instances (which come and go with
	// deploys) are just samples of the same metric, and can be aggregated with statistics like Max and Sum
	metrics = append(metrics,
		cwatch.Datum("DBConnectionsInUse", float64(dbStats.InUse), cwtypes.StandardUnitCount),
		cwatch.Datum("DBConnectionWaitDuration", float64(dbWaitDurationInPeriod)/float64(time.Second), cwtypes.StandardUnitSeconds),
		cwatch.Datum("ValkeyConnectionsInUse", float64(redisStats.ActiveCount), cwtypes.StandardUnitCount),
		cwatch.Datum("ValkeyConnectionsWaitDuration", float64(redisWaitDurationInPeriod)/float64(time.Second), cwtypes.StandardUnitSeconds),
		cwatch.Datum("QueuedMsgs", float64(bulkSize), cwtypes.StandardUnitCount, cwatch.Dimension("QueueName", "bulk")),
		cwatch.Datum("QueuedMsgs", float64(prioritySize), cwtypes.StandardUnitCount, cwatch.Dimension("QueueName", "priority")),
		cwatch.Datum("DynamoSpooledItems", float64(s.rt.Spool.Size()), cwtypes.StandardUnitCount),
		cwatch.Datum("PostgresSpooledItems", float64(msgSpoolSize), cwtypes.StandardUnitCount, cwatch.Dimension("SpoolName", "msgs")),
		cwatch.Datum("PostgresSpooledItems", float64(statusSpoolSize), cwtypes.StandardUnitCount, cwatch.Dimension("SpoolName", "statuses")),
		cwatch.Datum("PostgresSpooledItems", float64(eventSpoolSize), cwtypes.StandardUnitCount, cwatch.Dimension("SpoolName", "events")),
	)

	if err := s.rt.CW.Send(ctx, metrics...); err != nil {
		return 0, fmt.Errorf("error sending metrics: %w", err)
	}

	return len(metrics), nil
}
