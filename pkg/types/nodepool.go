package types

import "time"

// NodePool represents a nodepool resource
type NodePool struct {
	ID              string              `json:"id"`
	ClusterID       string              `json:"cluster_id"`
	Name            string              `json:"name"`
	CreatedBy       string              `json:"created_by"`
	Generation      int64               `json:"generation"`
	ResourceVersion string              `json:"resource_version"`
	Spec            *NodePoolSpec       `json:"spec"`
	Status          *NodePoolStatusInfo `json:"status,omitempty"`
	CreatedAt       time.Time           `json:"created_at"`
	UpdatedAt       time.Time           `json:"updated_at"`
}

// NodePoolCreateRequest represents a request to create a nodepool
type NodePoolCreateRequest struct {
	ClusterID string        `json:"cluster_id"`
	Name      string        `json:"name"`
	Spec      *NodePoolSpec `json:"spec"`
}

// NodePoolUpdateRequest represents a request to update a nodepool
type NodePoolUpdateRequest struct {
	Spec *NodePoolSpec `json:"spec"`
}

// NodePoolSpec represents the specification for a nodepool
type NodePoolSpec struct {
	Replicas         int32                  `json:"replicas,omitempty"`
	Management       map[string]interface{} `json:"management,omitempty"`
	Platform         map[string]interface{} `json:"platform,omitempty"`
	Release          map[string]interface{} `json:"release,omitempty"`
	NodeDrainTimeout string                 `json:"nodeDrainTimeout,omitempty"`
}

// NodePoolStatusInfo represents the status of a nodepool
type NodePoolStatusInfo struct {
	ObservedGeneration int64       `json:"observedGeneration"`
	Conditions         []Condition `json:"conditions,omitempty"`
	Phase              string      `json:"phase"`
	Message            string      `json:"message,omitempty"`
	Reason             string      `json:"reason,omitempty"`
	LastUpdateTime     time.Time   `json:"lastUpdateTime"`
}

// NodePoolControllerStatus represents controller-specific status for a nodepool
type NodePoolControllerStatus struct {
	NodePoolID         string                 `json:"nodepool_id"`
	ControllerName     string                 `json:"controller_name"`
	ObservedGeneration int64                  `json:"observed_generation"`
	Conditions         []Condition            `json:"conditions,omitempty"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
	LastUpdated        time.Time              `json:"last_updated"`
}

// NodePoolStatusResponse represents the response for nodepool status endpoint
type NodePoolStatusResponse struct {
	NodePoolID         string                      `json:"nodepool_id"`
	Status             *NodePoolStatusInfo         `json:"status"`
	ControllerStatuses []*NodePoolControllerStatus `json:"controller_statuses,omitempty"`
}
