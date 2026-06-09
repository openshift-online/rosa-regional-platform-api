package zoa

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/openshift/rosa-regional-platform-api/pkg/authz/client"
)

// ExecutionStore provides CRUD operations for ZOA executions.
type ExecutionStore interface {
	Create(ctx context.Context, exec *Execution) error
	Get(ctx context.Context, executionID string) (*Execution, error)
	List(ctx context.Context, accountID string, limit int) ([]*Execution, error)
	UpdateStatus(ctx context.Context, executionID string, status ExecutionStatus, completedAt string, duration int) error
	UpdateCompletion(ctx context.Context, executionID string, status ExecutionStatus, completedAt string, duration int, artifactsAvailable bool) error
	UpdateManifestWorkName(ctx context.Context, executionID, mwName string) error
	ListPending(ctx context.Context) ([]*Execution, error)
}

// DynamoExecutionStore implements ExecutionStore backed by DynamoDB.
type DynamoExecutionStore struct {
	tableName    string
	dynamoClient client.DynamoDBClient
	logger       *slog.Logger
}

// NewDynamoExecutionStore creates a new DynamoDB-backed execution store.
func NewDynamoExecutionStore(tableName string, dynamoClient client.DynamoDBClient, logger *slog.Logger) *DynamoExecutionStore {
	return &DynamoExecutionStore{
		tableName:    tableName,
		dynamoClient: dynamoClient,
		logger:       logger,
	}
}

func (s *DynamoExecutionStore) Create(ctx context.Context, exec *Execution) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if exec.CreatedAt == "" {
		exec.CreatedAt = now
	}
	if exec.UpdatedAt == "" {
		exec.UpdatedAt = now
	}

	item, err := attributevalue.MarshalMap(exec)
	if err != nil {
		return fmt.Errorf("failed to marshal execution: %w", err)
	}

	_, err = s.dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.tableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(executionId)"),
	})
	if err != nil {
		return fmt.Errorf("failed to create execution: %w", err)
	}

	s.logger.Info("execution created", "execution_id", exec.ExecutionID, "action", exec.Action)
	return nil
}

func (s *DynamoExecutionStore) Get(ctx context.Context, executionID string) (*Execution, error) {
	result, err := s.dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"executionId": &types.AttributeValueMemberS{Value: executionID},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get execution: %w", err)
	}

	if result.Item == nil {
		return nil, nil
	}

	var exec Execution
	if err := attributevalue.UnmarshalMap(result.Item, &exec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal execution: %w", err)
	}

	return &exec, nil
}

func (s *DynamoExecutionStore) List(ctx context.Context, accountID string, limit int) ([]*Execution, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("account-index"),
		KeyConditionExpression: aws.String("accountId = :aid"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":aid": &types.AttributeValueMemberS{Value: accountID},
		},
		ScanIndexForward: aws.Bool(false),
	}
	if limit > 0 {
		input.Limit = aws.Int32(int32(limit))
	}

	result, err := s.dynamoClient.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to list executions: %w", err)
	}

	executions := make([]*Execution, 0, len(result.Items))
	for _, item := range result.Items {
		var exec Execution
		if err := attributevalue.UnmarshalMap(item, &exec); err != nil {
			s.logger.Error("failed to unmarshal execution item", "error", err)
			continue
		}
		executions = append(executions, &exec)
	}

	return executions, nil
}

func (s *DynamoExecutionStore) UpdateStatus(ctx context.Context, executionID string, status ExecutionStatus, completedAt string, duration int) error {
	updateExpr := "SET #status = :s, updatedAt = :u"
	exprNames := map[string]string{"#status": "status"}
	exprValues := map[string]types.AttributeValue{
		":s": &types.AttributeValueMemberS{Value: string(status)},
		":u": &types.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339)},
	}

	if completedAt != "" {
		updateExpr += ", completedAt = :c, #dur = :d"
		exprNames["#dur"] = "duration"
		exprValues[":c"] = &types.AttributeValueMemberS{Value: completedAt}
		exprValues[":d"] = &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", duration)}
	}

	_, err := s.dynamoClient.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"executionId": &types.AttributeValueMemberS{Value: executionID},
		},
		UpdateExpression:          aws.String(updateExpr),
		ExpressionAttributeNames:  exprNames,
		ExpressionAttributeValues: exprValues,
	})
	if err != nil {
		return fmt.Errorf("failed to update execution status: %w", err)
	}

	s.logger.Info("execution status updated", "execution_id", executionID, "status", status)
	return nil
}

func (s *DynamoExecutionStore) UpdateCompletion(ctx context.Context, executionID string, status ExecutionStatus, completedAt string, duration int, artifactsAvailable bool) error {
	_, err := s.dynamoClient.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"executionId": &types.AttributeValueMemberS{Value: executionID},
		},
		UpdateExpression: aws.String("SET #status = :s, updatedAt = :u, completedAt = :c, #dur = :d, artifactsAvailable = :a"),
		ExpressionAttributeNames: map[string]string{
			"#status": "status",
			"#dur":    "duration",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":s": &types.AttributeValueMemberS{Value: string(status)},
			":u": &types.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339)},
			":c": &types.AttributeValueMemberS{Value: completedAt},
			":d": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", duration)},
			":a": &types.AttributeValueMemberBOOL{Value: artifactsAvailable},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to update execution completion: %w", err)
	}

	s.logger.Info("execution completion updated",
		"execution_id", executionID,
		"status", status,
		"artifacts_available", artifactsAvailable,
	)
	return nil
}

func (s *DynamoExecutionStore) UpdateManifestWorkName(ctx context.Context, executionID, mwName string) error {
	_, err := s.dynamoClient.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"executionId": &types.AttributeValueMemberS{Value: executionID},
		},
		UpdateExpression: aws.String("SET manifestWorkName = :mw, updatedAt = :u"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":mw": &types.AttributeValueMemberS{Value: mwName},
			":u":  &types.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339)},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to update manifestwork name: %w", err)
	}
	return nil
}

func (s *DynamoExecutionStore) ListPending(ctx context.Context) ([]*Execution, error) {
	executions := make([]*Execution, 0)

	for _, status := range []ExecutionStatus{StatusPending, StatusRunning} {
		result, err := s.dynamoClient.Query(ctx, &dynamodb.QueryInput{
			TableName:              aws.String(s.tableName),
			IndexName:              aws.String("status-index"),
			KeyConditionExpression: aws.String("#status = :status"),
			ExpressionAttributeNames: map[string]string{
				"#status": "status",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":status": &types.AttributeValueMemberS{Value: string(status)},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to query %s executions: %w", status, err)
		}

		for _, item := range result.Items {
			var exec Execution
			if err := attributevalue.UnmarshalMap(item, &exec); err != nil {
				s.logger.Error("failed to unmarshal execution item", "error", err)
				continue
			}
			executions = append(executions, &exec)
		}
	}

	return executions, nil
}
