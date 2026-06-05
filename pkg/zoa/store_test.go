package zoa

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"log/slog"
	"os"
)

type mockDynamoClient struct {
	putItemFunc    func(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	getItemFunc    func(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	queryFunc      func(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	scanFunc       func(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	updateItemFunc func(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	deleteItemFunc func(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
}

func (m *mockDynamoClient) PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	if m.putItemFunc != nil {
		return m.putItemFunc(ctx, params, optFns...)
	}
	return &dynamodb.PutItemOutput{}, nil
}

func (m *mockDynamoClient) GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	if m.getItemFunc != nil {
		return m.getItemFunc(ctx, params, optFns...)
	}
	return &dynamodb.GetItemOutput{}, nil
}

func (m *mockDynamoClient) Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, params, optFns...)
	}
	return &dynamodb.QueryOutput{}, nil
}

func (m *mockDynamoClient) Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	if m.scanFunc != nil {
		return m.scanFunc(ctx, params, optFns...)
	}
	return &dynamodb.ScanOutput{}, nil
}

func (m *mockDynamoClient) UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	if m.updateItemFunc != nil {
		return m.updateItemFunc(ctx, params, optFns...)
	}
	return &dynamodb.UpdateItemOutput{}, nil
}

func (m *mockDynamoClient) DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	if m.deleteItemFunc != nil {
		return m.deleteItemFunc(ctx, params, optFns...)
	}
	return &dynamodb.DeleteItemOutput{}, nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestDynamoExecutionStore_Create(t *testing.T) {
	var capturedInput *dynamodb.PutItemInput
	client := &mockDynamoClient{
		putItemFunc: func(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
			capturedInput = params
			return &dynamodb.PutItemOutput{}, nil
		},
	}

	store := NewDynamoExecutionStore("test-table", client, testLogger())

	exec := &Execution{
		ExecutionID:   "exec-123",
		AccountID:     "111222333444",
		Action:        "get_nodes",
		TargetCluster: "mc01",
		Scope:         "kube-api",
		Status:        StatusPending,
	}

	err := store.Create(context.Background(), exec)
	require.NoError(t, err)
	assert.Equal(t, "test-table", *capturedInput.TableName)
	assert.NotEmpty(t, exec.CreatedAt)
	assert.NotEmpty(t, exec.UpdatedAt)
}

func TestDynamoExecutionStore_Get(t *testing.T) {
	client := &mockDynamoClient{
		getItemFunc: func(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
			return &dynamodb.GetItemOutput{
				Item: map[string]types.AttributeValue{
					"executionId":   &types.AttributeValueMemberS{Value: "exec-123"},
					"accountId":     &types.AttributeValueMemberS{Value: "111222333444"},
					"action":        &types.AttributeValueMemberS{Value: "get_nodes"},
					"targetCluster": &types.AttributeValueMemberS{Value: "mc01"},
					"status":        &types.AttributeValueMemberS{Value: "pending"},
				},
			}, nil
		},
	}

	store := NewDynamoExecutionStore("test-table", client, testLogger())

	exec, err := store.Get(context.Background(), "exec-123")
	require.NoError(t, err)
	require.NotNil(t, exec)
	assert.Equal(t, "exec-123", exec.ExecutionID)
	assert.Equal(t, "get_nodes", exec.Action)
	assert.Equal(t, StatusPending, exec.Status)
}

func TestDynamoExecutionStore_Get_NotFound(t *testing.T) {
	client := &mockDynamoClient{
		getItemFunc: func(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
			return &dynamodb.GetItemOutput{Item: nil}, nil
		},
	}

	store := NewDynamoExecutionStore("test-table", client, testLogger())

	exec, err := store.Get(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, exec)
}

func TestDynamoExecutionStore_UpdateStatus(t *testing.T) {
	var capturedInput *dynamodb.UpdateItemInput
	client := &mockDynamoClient{
		updateItemFunc: func(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
			capturedInput = params
			return &dynamodb.UpdateItemOutput{}, nil
		},
	}

	store := NewDynamoExecutionStore("test-table", client, testLogger())

	err := store.UpdateStatus(context.Background(), "exec-123", StatusSucceeded, "2026-01-01T00:00:00Z", 42)
	require.NoError(t, err)
	assert.Equal(t, "test-table", *capturedInput.TableName)
	assert.Contains(t, *capturedInput.UpdateExpression, "completedAt")
}
