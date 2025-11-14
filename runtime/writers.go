package runtime

import (
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/nyaruka/gocommon/aws/dynamo"
)

type Writers struct {
	Main    *dynamo.Writer
	History *dynamo.Writer
}

func newWriters(cfg *Config, cl *dynamodb.Client, spool *dynamo.Spool) *Writers {
	return &Writers{
		Main:    dynamo.NewWriter(cl, cfg.DynamoTablePrefix+"Main", 250*time.Millisecond, 1000, spool),
		History: dynamo.NewWriter(cl, cfg.DynamoTablePrefix+"History", 250*time.Millisecond, 1000, spool),
	}
}

func (w *Writers) start() {
	w.Main.Start()
	w.History.Start()
}

func (w *Writers) stop() {
	w.Main.Stop()
	w.History.Stop()
}
