package runtime

import (
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/nyaruka/gocommon/aws/dynamo"
)

type Writers struct {
	Main    *dynamo.Writer
	History *dynamo.Writer
}

func newWriters(cfg *Config, cl *dynamodb.Client, spool *dynamo.Spool) *Writers {
	// status tags written to history might be same message so need to be de-duped
	historyKeyFn := func(item map[string]types.AttributeValue) string {
		pk := item["PK"].(*types.AttributeValueMemberS).Value
		sk := item["SK"].(*types.AttributeValueMemberS).Value
		return pk + "|" + sk
	}

	return &Writers{
		Main:    dynamo.NewWriter(cl, cfg.DynamoTablePrefix+"Main", 250*time.Millisecond, 1000, spool, nil),
		History: dynamo.NewWriter(cl, cfg.DynamoTablePrefix+"History", 250*time.Millisecond, 1000, spool, historyKeyFn),
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
