package rapidpro

import "time"

// Describes the common format for all items stored in DynamoDB.

// DynamoKey is the key type for all items, consisting of a partition key (PK) and a sort key (SK).
type DynamoKey struct {
	PK string `dynamodbav:"PK"`
	SK string `dynamodbav:"SK"`
}

// DynamoItem is the common structure for items stored in DynamoDB.
type DynamoItem struct {
	DynamoKey

	OrgID  int            `dynamodbav:"OrgID"`
	TTL    time.Time      `dynamodbav:"TTL,unixtime,omitempty"`
	Data   map[string]any `dynamodbav:"Data"`
	DataGZ []byte         `dynamodbav:"DataGZ,omitempty"`
}
