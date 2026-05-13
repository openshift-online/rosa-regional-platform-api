package client

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// NewDynamoDBClient creates a new DynamoDB client using the default AWS config
// If endpoint is provided, it overrides the default AWS endpoint (for local development)
func NewDynamoDBClient(ctx context.Context, region, endpoint string) (DynamoDBClient, error) {
	// FedRAMP SC-13 / IA-7: enable FIPS 140-3 validated endpoints.
	// config.WithUseFIPSEndpoint routes all DynamoDB API calls through
	// FIPS endpoints (e.g. dynamodb-fips.us-east-1.amazonaws.com) when
	// operating in a FedRAMP-authorized environment.
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithUseFIPSEndpoint(aws.FIPSEndpointStateEnabled),
	)
	if err != nil {
		return nil, err
	}

	var opts []func(*dynamodb.Options)
	if endpoint != "" {
		// For local DynamoDB, use dummy credentials and custom endpoint.
		// The local endpoint override disables FIPS routing for development only.
		opts = append(opts, func(o *dynamodb.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.Credentials = credentials.NewStaticCredentialsProvider("dummy", "dummy", "")
		})
	}

	return dynamodb.NewFromConfig(cfg, opts...), nil
}

// NewDynamoDBClientFromConfig creates a new DynamoDB client from an existing AWS config
func NewDynamoDBClientFromConfig(cfg aws.Config) DynamoDBClient {
	return dynamodb.NewFromConfig(cfg)
}
