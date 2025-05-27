package rapidpro

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/nyaruka/gocommon/aws/dynamo"
	"github.com/nyaruka/gocommon/syncx"
)

type DynamoItem struct {
	PK     string         `dynamodbav:"PK"`
	SK     string         `dynamodbav:"SK"`
	OrgID  int            `dynamodbav:"OrgID"`
	TTL    time.Time      `dynamodbav:"TTL,unixtime,omitempty"`
	Data   map[string]any `dynamodbav:"Data"`
	DataGZ []byte         `dynamodbav:"DataGZ,omitempty"`
}

func getDynamoItem(ctx context.Context, dyn *dynamo.Service, table, pk, sk string) (*DynamoItem, error) {
	resp, err := dyn.Client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(dyn.TableName(table)),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("error getting item from dynamo: %w", err)
	}

	if resp.Item == nil {
		return nil, nil // item not found
	}

	item := &DynamoItem{}
	if err := attributevalue.UnmarshalMap(resp.Item, &item); err != nil {
		return nil, fmt.Errorf("error unmarshalling dynamo item: %w", err)
	}

	return item, nil
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
		d, err := attributevalue.MarshalMap(item)
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
		slog.Error("unprocessed items writing to dynamo", "count", len(resp.UnprocessedItems))
	}
	return nil
}
