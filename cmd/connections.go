package cmd

import (
	"context"
	"log/slog"
	"time"

	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/gocommon/aws/dynamo"
)

// tests our connections to backing services, logging any failures but always moving forward
func testConnections(rt *runtime.Runtime) {
	log := slog.With("comp", "server")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// test Postgres
	if err := rt.DB.PingContext(ctx); err != nil {
		log.Error("db not reachable", "error", err)
	} else {
		log.Info("db ok")
	}

	// test DynamoDB
	if err := dynamo.Test(ctx, rt.Dynamo, rt.Config.DynamoTablePrefix+"Main"); err != nil {
		log.Error("dynamodb not reachable", "error", err)
	} else {
		log.Info("dynamodb ok")
	}

	// test Valkey
	vc := rt.VK.Get()
	defer vc.Close()
	if _, err := vc.Do("PING"); err != nil {
		log.Error("valkey not reachable", "error", err)
	} else {
		log.Info("valkey ok")
	}

	// test S3 bucket access
	if err := rt.S3.Test(ctx, rt.Config.S3AttachmentsBucket); err != nil {
		log.Error("attachments bucket not accessible", "error", err)
	} else {
		log.Info("attachments bucket ok")
	}

	// test that the Centrifugo server is reachable and accepts our key
	if err := rt.Centrifugo.Client.Info(ctx); err != nil {
		log.Error("centrifugo not reachable", "error", err)
	} else {
		log.Info("centrifugo ok")
	}
}
