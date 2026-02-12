package types

import "time"

// Cluster represents a cluster resource
type Cluster struct {
	ID              string                 `json:"id"`
	Name            string                 `json:"name"`
	TargetProjectID string                 `json:"target_project_id"`
	CreatedBy       string                 `json:"created_by"`
	Generation      int64                  `json:"generation"`
	ResourceVersion string                 `json:"resource_version"`
	Spec            map[string]interface{} `json:"spec"`
	Status          *ClusterStatusInfo     `json:"status,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

// ClusterCreateRequest represents a request to create a cluster
type ClusterCreateRequest struct {
	Name            string                 `json:"name"`
	TargetProjectID string                 `json:"target_project_id,omitempty"`
	Spec            map[string]interface{} `json:"spec"`
}

// ClusterUpdateRequest represents a request to update a cluster
type ClusterUpdateRequest struct {
	Spec map[string]interface{} `json:"spec"`
}

// ClusterStatusInfo represents the status of a cluster
type ClusterStatusInfo struct {
	ObservedGeneration int64       `json:"observedGeneration"`
	Conditions         []Condition `json:"conditions,omitempty"`
	Phase              string      `json:"phase"`
	Message            string      `json:"message,omitempty"`
	Reason             string      `json:"reason,omitempty"`
	LastUpdateTime     time.Time   `json:"lastUpdateTime"`
}

// Condition represents a status condition
type Condition struct {
	Type               string    `json:"type"`
	Status             string    `json:"status"`
	LastTransitionTime time.Time `json:"lastTransitionTime"`
	Reason             string    `json:"reason,omitempty"`
	Message            string    `json:"message,omitempty"`
}

// ClusterControllerStatus represents controller-specific status for a cluster
type ClusterControllerStatus struct {
	ClusterID          string                 `json:"cluster_id"`
	ControllerName     string                 `json:"controller_name"`
	ObservedGeneration int64                  `json:"observed_generation"`
	Conditions         []Condition            `json:"conditions,omitempty"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
	LastUpdated        time.Time              `json:"last_updated"`
}

// ClusterStatusResponse represents the response for cluster status endpoint
type ClusterStatusResponse struct {
	ClusterID          string                     `json:"cluster_id"`
	Status             *ClusterStatusInfo         `json:"status"`
	ControllerStatuses []*ClusterControllerStatus `json:"controller_statuses,omitempty"`
}
