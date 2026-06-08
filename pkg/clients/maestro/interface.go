package maestro

import (
	"context"

	workv1 "open-cluster-management.io/api/work/v1"
)

// ClientInterface defines the interface for Maestro API operations
type ClientInterface interface {
	CreateConsumer(ctx context.Context, req *ConsumerCreateRequest) (*Consumer, error)
	ListConsumers(ctx context.Context, page, size int) (*ConsumerList, error)
	GetConsumer(ctx context.Context, id string) (*Consumer, error)
	ListResourceBundles(ctx context.Context, page, size int, search, orderBy, fields string) (*ResourceBundleList, error)
	GetResourceBundle(ctx context.Context, id string) (*ResourceBundle, error)
	DeleteResourceBundle(ctx context.Context, id string) error
	CreateManifestWork(ctx context.Context, clusterName string, manifestWork *workv1.ManifestWork) (*workv1.ManifestWork, error)
	GetManifestWork(ctx context.Context, clusterName string, name string) (*workv1.ManifestWork, error)
	DeleteManifestWork(ctx context.Context, clusterName string, name string) error
}

// Ensure Client implements ClientInterface
var _ ClientInterface = (*Client)(nil)
