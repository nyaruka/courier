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

type DynamoItem struct {
	PK         string         `dynamodbav:"PK"`
	SK         string         `dynamodbav:"SK"`
	OrgID      int            `dynamodbav:"OrgID"`
	TTL        time.Time      `dynamodbav:"TTL,unixtime,omitempty"`
	Attributes map[string]any `dynamodbav:"Attributes"`
	DataGZ     []byte         `dynamodbav:"DataGZ,omitempty"`
}

func (d *DynamoItem) EncodeData(v any) error {
	var err error
	d.DataGZ, err = dynamo.MarshalJSONGZ(v)
	return err
}

func (d *DynamoItem) MarshalDynamo() (map[string]types.AttributeValue, error) {
	return attributevalue.MarshalMap(d)
}

func (d *DynamoItem) UnmarshalDynamo(m map[string]types.AttributeValue) error {
	return attributevalue.UnmarshalMap(m, d)
}

type DynamoWriter struct {
	*syncx.Batcher[*DynamoItem]
}

func NewDynamoWriter(dy *dynamo.Service, wg *sync.WaitGroup) *DynamoWriter {
	return &DynamoWriter{
		Batcher: syncx.NewBatcher(func(batch []*DynamoItem) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			if err := writeDynamoBatch(ctx, dy, batch); err != nil {
				slog.Error("error writing logs to dynamo", "error", err)
			}
		}, 25, time.Millisecond*500, 1000, wg),
	}
}

func writeDynamoBatch(ctx context.Context, ds *dynamo.Service, batch []*DynamoItem) error {
	writeReqs := make([]types.WriteRequest, len(batch))

	for i, item := range batch {
		d, err := item.MarshalDynamo()
		if err != nil {
			return fmt.Errorf("error marshalling log for dynamo: %w", err)
		}
		writeReqs[i] = types.WriteRequest{PutRequest: &types.PutRequest{Item: d}}
	}

	resp, err := ds.Client.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]types.WriteRequest{ds.TableName("Main"): writeReqs},
	})
	if err != nil {
		return err
	}
	if len(resp.UnprocessedItems) > 0 {
		// TODO shouldn't happend.. but need to figure out how we would retry these
		slog.Error("unprocessed items writing logs to dynamo", "count", len(resp.UnprocessedItems))
	}
	return nil
}
