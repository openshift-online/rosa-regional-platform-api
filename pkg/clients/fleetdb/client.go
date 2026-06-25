package fleetdb

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hyperfleetv1alpha1 "github.com/typeid/hyperfleet-operator/api/v1alpha1"
)

// Client wraps a controller-runtime client authenticated to fleet-db.
type Client struct {
	client.Client
	logger *slog.Logger
}

// NewClientFrom wraps an existing controller-runtime client (useful for testing
// with fake.NewClientBuilder).
func NewClientFrom(c client.Client, logger *slog.Logger) *Client {
	return &Client{Client: c, logger: logger}
}

// NewClient creates a fleet-db client by building a REST config from IAM
// credentials and connecting to the fleet-db EKS cluster.
func NewClient(ctx context.Context, awsCfg aws.Config, clusterName string, logger *slog.Logger) (*Client, error) {
	restCfg, err := newRESTConfig(ctx, awsCfg, clusterName)
	if err != nil {
		return nil, fmt.Errorf("build fleet-db REST config: %w", err)
	}

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("register core scheme: %w", err)
	}
	if err := hyperfleetv1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("register hyperfleet scheme: %w", err)
	}

	c, err := client.New(restCfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("create fleet-db kube client: %w", err)
	}

	fc := &Client{Client: c, logger: logger}
	if err := fc.ensureNamespace(ctx, "zoa-jobs"); err != nil {
		return nil, fmt.Errorf("ensure zoa-jobs namespace on fleet-db: %w", err)
	}

	return fc, nil
}

// --- Cluster operations ---

// CreateCluster creates a Cluster CR on fleet-db. The accountID becomes the
// namespace; the cluster ID is metadata.name.
func (c *Client) CreateCluster(ctx context.Context, accountID string, cluster *hyperfleetv1alpha1.Cluster) error {
	if err := c.ensureNamespace(ctx, accountID); err != nil {
		return fmt.Errorf("ensure namespace %s: %w", accountID, err)
	}
	cluster.Namespace = accountID
	if err := c.Client.Create(ctx, cluster); err != nil {
		return fmt.Errorf("create cluster %s/%s: %w", accountID, cluster.Name, err)
	}
	return nil
}

func (c *Client) ensureNamespace(ctx context.Context, name string) error {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if err := c.Client.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

// GetCluster retrieves a Cluster CR by account ID and cluster ID.
func (c *Client) GetCluster(ctx context.Context, accountID, clusterID string) (*hyperfleetv1alpha1.Cluster, error) {
	var cluster hyperfleetv1alpha1.Cluster
	key := client.ObjectKey{Namespace: accountID, Name: clusterID}
	if err := c.Client.Get(ctx, key, &cluster); err != nil {
		return nil, err
	}
	return &cluster, nil
}

// ListClusters lists Cluster CRs in the given account namespace.
func (c *Client) ListClusters(ctx context.Context, accountID string) (*hyperfleetv1alpha1.ClusterList, error) {
	var list hyperfleetv1alpha1.ClusterList
	if err := c.Client.List(ctx, &list, client.InNamespace(accountID)); err != nil {
		return nil, fmt.Errorf("list clusters in %s: %w", accountID, err)
	}
	return &list, nil
}

// UpdateCluster updates the spec of an existing Cluster CR.
func (c *Client) UpdateCluster(ctx context.Context, cluster *hyperfleetv1alpha1.Cluster) error {
	if err := c.Client.Update(ctx, cluster); err != nil {
		return fmt.Errorf("update cluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}
	return nil
}

// DeleteCluster deletes a Cluster CR.
func (c *Client) DeleteCluster(ctx context.Context, accountID, clusterID string) error {
	cluster := &hyperfleetv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: accountID,
			Name:      clusterID,
		},
	}
	if err := c.Client.Delete(ctx, cluster); err != nil {
		return fmt.Errorf("delete cluster %s/%s: %w", accountID, clusterID, err)
	}
	return nil
}

// --- NodePool operations ---

// CreateNodePool creates a NodePool CR on fleet-db.
func (c *Client) CreateNodePool(ctx context.Context, accountID string, np *hyperfleetv1alpha1.NodePool) error {
	np.Namespace = accountID
	if err := c.Client.Create(ctx, np); err != nil {
		return fmt.Errorf("create nodepool %s/%s: %w", accountID, np.Name, err)
	}
	return nil
}

// GetNodePool retrieves a NodePool CR by account ID and nodepool ID.
func (c *Client) GetNodePool(ctx context.Context, accountID, nodepoolID string) (*hyperfleetv1alpha1.NodePool, error) {
	var np hyperfleetv1alpha1.NodePool
	key := client.ObjectKey{Namespace: accountID, Name: nodepoolID}
	if err := c.Client.Get(ctx, key, &np); err != nil {
		return nil, err
	}
	return &np, nil
}

// ListNodePools lists NodePool CRs in the given account namespace, optionally
// filtered by clusterRef.
func (c *Client) ListNodePools(ctx context.Context, accountID, clusterRef string) (*hyperfleetv1alpha1.NodePoolList, error) {
	var list hyperfleetv1alpha1.NodePoolList
	if err := c.Client.List(ctx, &list, client.InNamespace(accountID)); err != nil {
		return nil, fmt.Errorf("list nodepools in %s: %w", accountID, err)
	}
	if clusterRef != "" {
		filtered := list.Items[:0]
		for i := range list.Items {
			if list.Items[i].Spec.ClusterRef == clusterRef {
				filtered = append(filtered, list.Items[i])
			}
		}
		list.Items = filtered
	}
	return &list, nil
}

// UpdateNodePool updates the spec of an existing NodePool CR.
func (c *Client) UpdateNodePool(ctx context.Context, np *hyperfleetv1alpha1.NodePool) error {
	if err := c.Client.Update(ctx, np); err != nil {
		return fmt.Errorf("update nodepool %s/%s: %w", np.Namespace, np.Name, err)
	}
	return nil
}

// DeleteNodePool deletes a NodePool CR.
func (c *Client) DeleteNodePool(ctx context.Context, accountID, nodepoolID string) error {
	np := &hyperfleetv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: accountID,
			Name:      nodepoolID,
		},
	}
	if err := c.Client.Delete(ctx, np); err != nil {
		return fmt.Errorf("delete nodepool %s/%s: %w", accountID, nodepoolID, err)
	}
	return nil
}

// --- HyperFleetManifest operations ---

// CreateManifest creates a HyperFleetManifest CR on fleet-db.
func (c *Client) CreateManifest(ctx context.Context, accountID string, hfm *hyperfleetv1alpha1.HyperFleetManifest) error {
	hfm.Namespace = accountID
	if err := c.Client.Create(ctx, hfm); err != nil {
		return fmt.Errorf("create manifest %s/%s: %w", accountID, hfm.Name, err)
	}
	return nil
}

// GetManifest retrieves a HyperFleetManifest CR by account ID and name.
func (c *Client) GetManifest(ctx context.Context, accountID, name string) (*hyperfleetv1alpha1.HyperFleetManifest, error) {
	var hfm hyperfleetv1alpha1.HyperFleetManifest
	key := client.ObjectKey{Namespace: accountID, Name: name}
	if err := c.Client.Get(ctx, key, &hfm); err != nil {
		return nil, err
	}
	return &hfm, nil
}

// DeleteManifest deletes a HyperFleetManifest CR.
func (c *Client) DeleteManifest(ctx context.Context, accountID, name string) error {
	hfm := &hyperfleetv1alpha1.HyperFleetManifest{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: accountID,
			Name:      name,
		},
	}
	if err := c.Client.Delete(ctx, hfm); err != nil {
		return fmt.Errorf("delete manifest %s/%s: %w", accountID, name, err)
	}
	return nil
}

// --- Error helpers ---

// IsNotFound returns true if the error is a Kubernetes 404.
func IsNotFound(err error) bool {
	return apierrors.IsNotFound(err)
}

// IsAlreadyExists returns true if the error is a Kubernetes 409 (already exists).
func IsAlreadyExists(err error) bool {
	return apierrors.IsAlreadyExists(err)
}
