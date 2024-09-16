package clogs

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/nyaruka/gocommon/aws/dynamo"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
)

const (
	dynamoTableBase = "ChannelLogs"
	dynamoTTL       = 14 * 24 * time.Hour
)

// log struct to be written to DynamoDB
type dynamoLog struct {
	UUID      LogUUID   `dynamodbav:"UUID"`
	Type      LogType   `dynamodbav:"Type"`
	DataGZ    []byte    `dynamodbav:"DataGZ,omitempty"`
	ElapsedMS int       `dynamodbav:"ElapsedMS"`
	CreatedOn time.Time `dynamodbav:"CreatedOn,unixtime"`
	ExpiresOn time.Time `dynamodbav:"ExpiresOn,unixtime"`
}

type dynamoLogData struct {
	HttpLogs []*httpx.Log `json:"http_logs"`
	Errors   []*LogError  `json:"errors"`
}

func newDynamoLog(l *Log) *dynamoLog {
	data := &dynamoLogData{HttpLogs: l.HttpLogs, Errors: l.Errors}
	buf := &bytes.Buffer{}
	w := gzip.NewWriter(buf)
	w.Write(jsonx.MustMarshal(data))
	w.Close()

	return &dynamoLog{
		UUID:      l.UUID,
		Type:      l.Type,
		DataGZ:    buf.Bytes(),
		ElapsedMS: int(l.Elapsed / time.Millisecond),
		CreatedOn: l.CreatedOn,
		ExpiresOn: l.CreatedOn.Add(dynamoTTL),
	}
}

func (d *dynamoLog) unpack() (*Log, error) {
	r, err := gzip.NewReader(bytes.NewReader(d.DataGZ))
	if err != nil {
		return nil, err
	}
	j, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var data dynamoLogData
	if err := json.Unmarshal(j, &data); err != nil {
		return nil, err
	}

	return &Log{
		UUID:      d.UUID,
		Type:      d.Type,
		HttpLogs:  data.HttpLogs,
		Errors:    data.Errors,
		CreatedOn: d.CreatedOn,
		Elapsed:   time.Duration(d.ElapsedMS) * time.Millisecond,
	}, nil
}

// Get retrieves a log from DynamoDB by its UUID
func Get(ctx context.Context, ds *dynamo.Service, uuid LogUUID) (*Log, error) {
	resp, err := ds.Client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(ds.TableName(dynamoTableBase)),
		Key: map[string]types.AttributeValue{
			"UUID": &types.AttributeValueMemberS{Value: string(uuid)},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("error getting log from db: %w", err)
	}

	var d dynamoLog
	if err := attributevalue.UnmarshalMap(resp.Item, &d); err != nil {
		return nil, fmt.Errorf("error unmarshaling log: %w", err)
	}

	return d.unpack()
}

// BulkPut writes multiple logs to DynamoDB in batches of 25
func BulkPut(ctx context.Context, ds *dynamo.Service, logs []*Log) error {
	for batch := range slices.Chunk(logs, 25) {
		writeReqs := make([]types.WriteRequest, len(batch))

		for i, l := range batch {
			dl := newDynamoLog(l)

			item, err := attributevalue.MarshalMap(dl)
			if err != nil {
				return fmt.Errorf("error marshalling log: %w", err)
			}
			writeReqs[i] = types.WriteRequest{PutRequest: &types.PutRequest{Item: item}}
		}

		_, err := ds.Client.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{ds.TableName(dynamoTableBase): writeReqs},
		})
		if err != nil {
			return fmt.Errorf("error writing logs to db: %w", err)
		}
	}

	return nil
}
