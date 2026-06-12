package zoa

import (
	"context"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"

	"github.com/openshift/rosa-regional-platform-api/pkg/authz/client"
)

// AuditEntry represents a single API call logged in the zoa-audit-log table.
type AuditEntry struct {
	ID            string `dynamodbav:"id" json:"id"`
	AccountID     string `dynamodbav:"accountId" json:"account_id"`
	CallerARN     string `dynamodbav:"callerArn" json:"caller_arn"`
	Operator      string `dynamodbav:"operator" json:"operator"`
	Method        string `dynamodbav:"method" json:"method"`
	Path          string `dynamodbav:"path" json:"path"`
	Action        string `dynamodbav:"action" json:"action"`
	TargetCluster string `dynamodbav:"targetCluster" json:"target_cluster"`
	ExecutionID   string `dynamodbav:"executionId" json:"execution_id"`
	Jira          string `dynamodbav:"jira" json:"jira"`
	StatusCode    int    `dynamodbav:"statusCode" json:"status_code"`
	Timestamp     string `dynamodbav:"timestamp" json:"timestamp"`
	TTL           int64  `dynamodbav:"ttl,omitempty" json:"-"`
}

// AuditFilter defines optional filters for listing audit entries.
type AuditFilter struct {
	Action        string
	Operator      string
	TargetCluster string
	Method        string
	Since         string
}

// AuditStore provides operations for the ZOA audit log.
type AuditStore interface {
	Record(ctx context.Context, entry *AuditEntry) error
	List(ctx context.Context, accountID string, limit int, filter *AuditFilter) ([]*AuditEntry, error)
}

// DynamoAuditStore implements AuditStore backed by DynamoDB.
type DynamoAuditStore struct {
	tableName    string
	dynamoClient client.DynamoDBClient
	logger       *slog.Logger
}

// NewDynamoAuditStore creates a new DynamoDB-backed audit store.
func NewDynamoAuditStore(tableName string, dynamoClient client.DynamoDBClient, logger *slog.Logger) *DynamoAuditStore {
	return &DynamoAuditStore{
		tableName:    tableName,
		dynamoClient: dynamoClient,
		logger:       logger,
	}
}

func (s *DynamoAuditStore) Record(ctx context.Context, entry *AuditEntry) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	entry.TTL = time.Now().UTC().AddDate(0, 0, 365).Unix()

	item, err := attributevalue.MarshalMap(entry)
	if err != nil {
		return err
	}

	_, err = s.dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	})
	return err
}

func (s *DynamoAuditStore) List(ctx context.Context, accountID string, limit int, filter *AuditFilter) ([]*AuditEntry, error) {
	exprNames := map[string]string{}
	exprValues := map[string]types.AttributeValue{
		":aid": &types.AttributeValueMemberS{Value: accountID},
	}
	keyCondition := "#aid = :aid"
	exprNames["#aid"] = "accountId"

	filterParts := []string{}

	if filter != nil {
		if filter.Action != "" {
			filterParts = append(filterParts, "#act = :act")
			exprNames["#act"] = "action"
			exprValues[":act"] = &types.AttributeValueMemberS{Value: filter.Action}
		}
		if filter.Operator != "" {
			filterParts = append(filterParts, "#op = :op")
			exprNames["#op"] = "operator"
			exprValues[":op"] = &types.AttributeValueMemberS{Value: filter.Operator}
		}
		if filter.TargetCluster != "" {
			filterParts = append(filterParts, "#tc = :tc")
			exprNames["#tc"] = "targetCluster"
			exprValues[":tc"] = &types.AttributeValueMemberS{Value: filter.TargetCluster}
		}
		if filter.Method != "" {
			filterParts = append(filterParts, "#mth = :mth")
			exprNames["#mth"] = "method"
			exprValues[":mth"] = &types.AttributeValueMemberS{Value: filter.Method}
		}
		if filter.Since != "" {
			filterParts = append(filterParts, "#ts >= :since")
			exprNames["#ts"] = "timestamp"
			exprValues[":since"] = &types.AttributeValueMemberS{Value: filter.Since}
		}
	}

	input := &dynamodb.QueryInput{
		TableName:                 aws.String(s.tableName),
		KeyConditionExpression:    aws.String(keyCondition),
		ExpressionAttributeNames: exprNames,
		ExpressionAttributeValues: exprValues,
		ScanIndexForward:          aws.Bool(false),
		Limit:                     aws.Int32(int32(limit)),
	}

	if len(filterParts) > 0 {
		filterExpr := ""
		for i, part := range filterParts {
			if i > 0 {
				filterExpr += " AND "
			}
			filterExpr += part
		}
		input.FilterExpression = aws.String(filterExpr)
	}

	result, err := s.dynamoClient.Query(ctx, input)
	if err != nil {
		return nil, err
	}

	entries := make([]*AuditEntry, 0, len(result.Items))
	for _, item := range result.Items {
		var entry AuditEntry
		if err := attributevalue.UnmarshalMap(item, &entry); err != nil {
			s.logger.Warn("failed to unmarshal audit entry", "error", err)
			continue
		}
		entries = append(entries, &entry)
	}
	return entries, nil
}
