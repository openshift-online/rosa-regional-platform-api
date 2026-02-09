package client

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/verifiedpermissions"
)

// DynamoDBClient defines the interface for DynamoDB operations used by the authz package
type DynamoDBClient interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
}

// AVPClient defines the interface for Amazon Verified Permissions operations
type AVPClient interface {
	CreatePolicyStore(ctx context.Context, params *verifiedpermissions.CreatePolicyStoreInput, optFns ...func(*verifiedpermissions.Options)) (*verifiedpermissions.CreatePolicyStoreOutput, error)
	DeletePolicyStore(ctx context.Context, params *verifiedpermissions.DeletePolicyStoreInput, optFns ...func(*verifiedpermissions.Options)) (*verifiedpermissions.DeletePolicyStoreOutput, error)
	GetPolicyStore(ctx context.Context, params *verifiedpermissions.GetPolicyStoreInput, optFns ...func(*verifiedpermissions.Options)) (*verifiedpermissions.GetPolicyStoreOutput, error)
	CreatePolicy(ctx context.Context, params *verifiedpermissions.CreatePolicyInput, optFns ...func(*verifiedpermissions.Options)) (*verifiedpermissions.CreatePolicyOutput, error)
	DeletePolicy(ctx context.Context, params *verifiedpermissions.DeletePolicyInput, optFns ...func(*verifiedpermissions.Options)) (*verifiedpermissions.DeletePolicyOutput, error)
	GetPolicy(ctx context.Context, params *verifiedpermissions.GetPolicyInput, optFns ...func(*verifiedpermissions.Options)) (*verifiedpermissions.GetPolicyOutput, error)
	UpdatePolicy(ctx context.Context, params *verifiedpermissions.UpdatePolicyInput, optFns ...func(*verifiedpermissions.Options)) (*verifiedpermissions.UpdatePolicyOutput, error)
	IsAuthorized(ctx context.Context, params *verifiedpermissions.IsAuthorizedInput, optFns ...func(*verifiedpermissions.Options)) (*verifiedpermissions.IsAuthorizedOutput, error)
	PutSchema(ctx context.Context, params *verifiedpermissions.PutSchemaInput, optFns ...func(*verifiedpermissions.Options)) (*verifiedpermissions.PutSchemaOutput, error)
	CreatePolicyTemplate(ctx context.Context, params *verifiedpermissions.CreatePolicyTemplateInput, optFns ...func(*verifiedpermissions.Options)) (*verifiedpermissions.CreatePolicyTemplateOutput, error)
	DeletePolicyTemplate(ctx context.Context, params *verifiedpermissions.DeletePolicyTemplateInput, optFns ...func(*verifiedpermissions.Options)) (*verifiedpermissions.DeletePolicyTemplateOutput, error)
	GetPolicyTemplate(ctx context.Context, params *verifiedpermissions.GetPolicyTemplateInput, optFns ...func(*verifiedpermissions.Options)) (*verifiedpermissions.GetPolicyTemplateOutput, error)
	UpdatePolicyTemplate(ctx context.Context, params *verifiedpermissions.UpdatePolicyTemplateInput, optFns ...func(*verifiedpermissions.Options)) (*verifiedpermissions.UpdatePolicyTemplateOutput, error)
	ListPolicyTemplates(ctx context.Context, params *verifiedpermissions.ListPolicyTemplatesInput, optFns ...func(*verifiedpermissions.Options)) (*verifiedpermissions.ListPolicyTemplatesOutput, error)
	ListPolicies(ctx context.Context, params *verifiedpermissions.ListPoliciesInput, optFns ...func(*verifiedpermissions.Options)) (*verifiedpermissions.ListPoliciesOutput, error)
}
