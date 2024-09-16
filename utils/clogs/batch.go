package clogs

import (
	"context"
	"fmt"
	"slices"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/nyaruka/gocommon/aws/dynamo"
)

// BatchPut writes multiple logs to DynamoDB in batches of 25. This should probably be a generic function in the
// gocommon/dynamo package but need to think more about errors.
func BatchPut(ctx context.Context, ds *dynamo.Service, table string, logs []*Log) error {
	for batch := range slices.Chunk(logs, 25) {
		writeReqs := make([]types.WriteRequest, len(batch))

		for i, l := range batch {
			d, err := l.MarshalDynamo()
			if err != nil {
				return fmt.Errorf("error marshalling log: %w", err)
			}
			writeReqs[i] = types.WriteRequest{PutRequest: &types.PutRequest{Item: d}}
		}

		_, err := ds.Client.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{ds.TableName(table): writeReqs},
		})
		if err != nil {
			return fmt.Errorf("error writing logs to db: %w", err)
		}
	}

	return nil
}
