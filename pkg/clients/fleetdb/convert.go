package fleetdb

import (
	"encoding/json"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hyperfleetv1alpha1 "github.com/typeid/hyperfleet-operator/api/v1alpha1"

	"github.com/openshift/rosa-regional-platform-api/pkg/types"
)

// --- Cluster conversions ---

// ClusterCRToPlatform converts a v1alpha1.Cluster CR to the platform API type.
func ClusterCRToPlatform(cr *hyperfleetv1alpha1.Cluster) *types.Cluster {
	spec := specToMap(cr.Spec)

	cluster := &types.Cluster{
		ID:              cr.Name,
		Name:            cr.Spec.Name,
		Generation:      cr.Generation,
		ResourceVersion: cr.ResourceVersion,
		Spec:            spec,
		CreatedAt:       cr.CreationTimestamp.Time,
		UpdatedAt:       metaTime(cr),
	}

	if cr.Spec.CreatorARN != "" {
		cluster.CreatedBy = cr.Spec.CreatorARN
	}

	if cr.Spec.AccountID != "" {
		cluster.TargetProjectID = cr.Spec.AccountID
	}

	if phase := cr.Status.Phase; phase != "" {
		cluster.Status = &types.ClusterStatusInfo{
			ObservedGeneration: cr.Status.ObservedGeneration,
			Phase:              phase,
			LastUpdateTime:     metaTime(cr),
		}

		if len(cr.Status.Conditions) > 0 {
			cluster.Status.Conditions = make([]types.Condition, 0, len(cr.Status.Conditions))
			for _, c := range cr.Status.Conditions {
				cluster.Status.Conditions = append(cluster.Status.Conditions, types.Condition{
					Type:               c.Type,
					Status:             string(c.Status),
					LastTransitionTime: c.LastTransitionTime.Time,
					Reason:             c.Reason,
					Message:            c.Message,
				})
			}
		}
	}

	return cluster
}

// PlatformCreateToClusterCR converts a platform ClusterCreateRequest into a
// v1alpha1.Cluster CR. The caller sets metadata.Namespace (accountID) and
// metadata.Name (clusterID).
func PlatformCreateToClusterCR(clusterID, accountID string, req *types.ClusterCreateRequest) (*hyperfleetv1alpha1.Cluster, error) {
	var spec hyperfleetv1alpha1.ClusterSpec
	if err := mapToSpec(req.Spec, &spec); err != nil {
		return nil, fmt.Errorf("convert cluster spec: %w", err)
	}

	if spec.Name == "" {
		spec.Name = req.Name
	}
	if spec.AccountID == "" {
		spec.AccountID = accountID
	}

	return &hyperfleetv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterID,
			Namespace: accountID,
		},
		Spec: spec,
	}, nil
}

// ApplyPlatformUpdateToClusterCR merges an update request into an existing CR.
func ApplyPlatformUpdateToClusterCR(cr *hyperfleetv1alpha1.Cluster, req *types.ClusterUpdateRequest) error {
	if req.Spec == nil {
		return nil
	}

	existing := specToMap(cr.Spec)
	for k, v := range req.Spec {
		existing[k] = v
	}

	var merged hyperfleetv1alpha1.ClusterSpec
	if err := mapToSpec(existing, &merged); err != nil {
		return fmt.Errorf("merge cluster spec: %w", err)
	}
	cr.Spec = merged
	return nil
}

// ClusterStatusFromCR builds the status response from a Cluster CR.
func ClusterStatusFromCR(cr *hyperfleetv1alpha1.Cluster) *types.ClusterStatusResponse {
	platform := ClusterCRToPlatform(cr)
	return &types.ClusterStatusResponse{
		ClusterID: cr.Name,
		Status:    platform.Status,
	}
}

// --- NodePool conversions ---

// NodePoolCRToPlatform converts a v1alpha1.NodePool CR to the platform API type.
func NodePoolCRToPlatform(cr *hyperfleetv1alpha1.NodePool) *types.NodePool {
	np := &types.NodePool{
		ID:              cr.Name,
		ClusterID:       cr.Spec.ClusterRef,
		Name:            cr.Name,
		Generation:      cr.Generation,
		ResourceVersion: cr.ResourceVersion,
		Spec: &types.NodePoolSpec{
			Replicas: cr.Spec.Replicas,
			Management: map[string]interface{}{
				"autoRepair":  cr.Spec.Management.AutoRepair,
				"upgradeType": cr.Spec.Management.UpgradeType,
			},
			Platform: map[string]interface{}{
				"aws": map[string]interface{}{
					"instanceType":    cr.Spec.Platform.AWS.InstanceType,
					"rootVolume":      map[string]interface{}{"size": cr.Spec.Platform.AWS.RootVolume.Size, "type": cr.Spec.Platform.AWS.RootVolume.Type},
					"subnetId":        cr.Spec.Platform.AWS.SubnetId,
					"instanceProfile": cr.Spec.Platform.AWS.InstanceProfile,
					"securityGroups":  cr.Spec.Platform.AWS.SecurityGroups,
				},
			},
			Release: map[string]interface{}{
				"image": cr.Spec.Release.Image,
			},
		},
		CreatedAt: cr.CreationTimestamp.Time,
		UpdatedAt: metaTime(cr),
	}

	if phase := cr.Status.Phase; phase != "" {
		np.Status = &types.NodePoolStatusInfo{
			ObservedGeneration: cr.Status.ObservedGeneration,
			Phase:              phase,
			LastUpdateTime:     metaTime(cr),
		}
		if len(cr.Status.Conditions) > 0 {
			np.Status.Conditions = make([]types.Condition, 0, len(cr.Status.Conditions))
			for _, c := range cr.Status.Conditions {
				np.Status.Conditions = append(np.Status.Conditions, types.Condition{
					Type:               c.Type,
					Status:             string(c.Status),
					LastTransitionTime: c.LastTransitionTime.Time,
					Reason:             c.Reason,
					Message:            c.Message,
				})
			}
		}
	}

	return np
}

// PlatformCreateToNodePoolCR converts a platform NodePoolCreateRequest into a
// v1alpha1.NodePool CR.
func PlatformCreateToNodePoolCR(nodepoolID, accountID string, req *types.NodePoolCreateRequest) (*hyperfleetv1alpha1.NodePool, error) {
	var spec hyperfleetv1alpha1.NodePoolSpec
	spec.ClusterRef = req.ClusterID

	if req.Spec != nil {
		spec.Replicas = req.Spec.Replicas
		if err := mapToSpec(req.Spec.Release, &spec.Release); err != nil {
			return nil, fmt.Errorf("convert nodepool release: %w", err)
		}
		if err := mapToSpec(req.Spec.Management, &spec.Management); err != nil {
			return nil, fmt.Errorf("convert nodepool management: %w", err)
		}
		if err := mapToSpec(req.Spec.Platform, &spec.Platform); err != nil {
			return nil, fmt.Errorf("convert nodepool platform: %w", err)
		}
	}

	return &hyperfleetv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nodepoolID,
			Namespace: accountID,
		},
		Spec: spec,
	}, nil
}

// ApplyPlatformUpdateToNodePoolCR merges an update request into an existing CR.
func ApplyPlatformUpdateToNodePoolCR(cr *hyperfleetv1alpha1.NodePool, req *types.NodePoolUpdateRequest) error {
	if req.Spec == nil {
		return nil
	}
	if req.Spec.Replicas != 0 {
		cr.Spec.Replicas = req.Spec.Replicas
	}
	if req.Spec.Release != nil {
		if err := mapToSpec(req.Spec.Release, &cr.Spec.Release); err != nil {
			return fmt.Errorf("merge nodepool release: %w", err)
		}
	}
	if req.Spec.Management != nil {
		if err := mapToSpec(req.Spec.Management, &cr.Spec.Management); err != nil {
			return fmt.Errorf("merge nodepool management: %w", err)
		}
	}
	if req.Spec.Platform != nil {
		if err := mapToSpec(req.Spec.Platform, &cr.Spec.Platform); err != nil {
			return fmt.Errorf("merge nodepool platform: %w", err)
		}
	}
	return nil
}

// NodePoolStatusFromCR builds the status response from a NodePool CR.
func NodePoolStatusFromCR(cr *hyperfleetv1alpha1.NodePool) *types.NodePoolStatusResponse {
	platform := NodePoolCRToPlatform(cr)
	return &types.NodePoolStatusResponse{
		NodePoolID: cr.Name,
		Status:     platform.Status,
	}
}

// --- helpers ---

// specToMap converts a typed struct to map[string]interface{} via JSON round-trip.
func specToMap(v interface{}) map[string]interface{} {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var m map[string]interface{}
	_ = json.Unmarshal(data, &m)
	return m
}

// mapToSpec converts a map (or any JSON-serializable value) into a typed struct
// via JSON round-trip.
func mapToSpec(src interface{}, dst interface{}) error {
	if src == nil {
		return nil
	}
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

func metaTime(obj metav1.Object) time.Time {
	if t := obj.GetDeletionTimestamp(); t != nil {
		return t.Time
	}
	return obj.GetCreationTimestamp().Time
}
