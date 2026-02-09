package client

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/verifiedpermissions"
)

// NewAVPClient creates a new Amazon Verified Permissions client using the default AWS config
func NewAVPClient(ctx context.Context, region string) (AVPClient, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, err
	}
	return verifiedpermissions.NewFromConfig(cfg), nil
}

// NewAVPClientFromConfig creates a new AVP client from an existing AWS config
func NewAVPClientFromConfig(cfg aws.Config) AVPClient {
	return verifiedpermissions.NewFromConfig(cfg)
}
