package rapidpro

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/nyaruka/gocommon/aws/dynamo"
	"github.com/nyaruka/gocommon/syncx"
)

type DynamoKey struct {
	PK string `dynamodbav:"PK"`
	SK string `dynamodbav:"SK"`
}

type DynamoItem struct {
	DynamoKey

	OrgID  int            `dynamodbav:"OrgID"`
	TTL    time.Time      `dynamodbav:"TTL,unixtime,omitempty"`
	Data   map[string]any `dynamodbav:"Data"`
	DataGZ []byte         `dynamodbav:"DataGZ,omitempty"`
}

type DynamoWriter struct {
	*syncx.Batcher[*DynamoItem]
}

func NewDynamoWriter(tbl *dynamo.Table[DynamoKey, DynamoItem], wg *sync.WaitGroup) *DynamoWriter {
	return &DynamoWriter{
		Batcher: syncx.NewBatcher(func(batch []*DynamoItem) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			if err := writeDynamoBatch(ctx, tbl, batch); err != nil {
				slog.Error("error writing logs to dynamo", "error", err)
			}
		}, 25, time.Millisecond*500, 1000, wg),
	}
}

func writeDynamoBatch(ctx context.Context, tbl *dynamo.Table[DynamoKey, DynamoItem], batch []*DynamoItem) error {
	writeReqs := make([]types.WriteRequest, len(batch))

	for i, item := range batch {
		d, err := attributevalue.MarshalMap(item)
		if err != nil {
			return fmt.Errorf("error marshalling log for dynamo: %w", err)
		}
		writeReqs[i] = types.WriteRequest{PutRequest: &types.PutRequest{Item: d}}
	}

	resp, err := tbl.Client.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]types.WriteRequest{tbl.Name(): writeReqs},
	})
	if err != nil {
		return err
	}
	if len(resp.UnprocessedItems) > 0 {
		// TODO shouldn't happend.. but need to figure out how we would retry these
		slog.Error("unprocessed items writing to dynamo", "count", len(resp.UnprocessedItems))
	}
	return nil
}
