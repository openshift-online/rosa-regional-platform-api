package hyperfleet

import "time"

const (
	// ClusterKind is the kind identifier for cluster resources
	ClusterKind = "Cluster"
)

// HFCluster represents a cluster in Hyperfleet API
type HFCluster struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Labels     map[string]string      `json:"labels,omitempty"`
	Spec       map[string]interface{} `json:"spec"`
	Status     *HFClusterStatus       `json:"status,omitempty"`
	Generation int64                  `json:"generation"`
	CreatedBy  string                 `json:"created_by,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
}

// HFClusterStatus represents the status of a cluster in Hyperfleet
type HFClusterStatus struct {
	ObservedGeneration int64         `json:"observed_generation"`
	Conditions         []HFCondition `json:"conditions,omitempty"`
	Phase              string        `json:"phase"`
	Message            string        `json:"message,omitempty"`
	Reason             string        `json:"reason,omitempty"`
	LastUpdateTime     time.Time     `json:"last_update_time"`
}

// HFCondition represents a status condition in Hyperfleet
type HFCondition struct {
	Type               string    `json:"type"`
	Status             string    `json:"status"`
	LastTransitionTime time.Time `json:"last_transition_time"`
	Reason             string    `json:"reason,omitempty"`
	Message            string    `json:"message,omitempty"`
}

// HFClusterCreateRequest represents a request to create a cluster in Hyperfleet
type HFClusterCreateRequest struct {
	Kind      string                 `json:"kind"`
	Name      string                 `json:"name"`
	Labels    map[string]string      `json:"labels,omitempty"`
	Spec      map[string]interface{} `json:"spec"`
	CreatedBy string                 `json:"created_by,omitempty"`
}

// HFClusterUpdateRequest represents a request to update a cluster in Hyperfleet
type HFClusterUpdateRequest struct {
	Spec map[string]interface{} `json:"spec"`
}

// HFClusterList represents a list of clusters from Hyperfleet
type HFClusterList struct {
	Items      []HFCluster `json:"items"`
	TotalCount int         `json:"total_count"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
}

// HFAdapterStatus represents adapter-specific status from Hyperfleet
type HFAdapterStatus struct {
	ClusterID          string                 `json:"cluster_id"`
	AdapterName        string                 `json:"adapter_name"`
	ObservedGeneration int64                  `json:"observed_generation"`
	Conditions         []HFCondition          `json:"conditions,omitempty"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
	Data               map[string]interface{} `json:"data,omitempty"`
	LastUpdated        time.Time              `json:"last_updated"`
}

// HFAdapterStatusList represents a list of adapter statuses from Hyperfleet
type HFAdapterStatusList struct {
	Items []HFAdapterStatus `json:"items"`
}

// Error represents an error response from Hyperfleet API
type Error struct {
	Kind    string `json:"kind,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Reason  string `json:"reason,omitempty"`
}

// Error implements the error interface
func (e *Error) Error() string {
	if e.Reason != "" {
		return e.Reason
	}
	return e.Message
}

// IsNotFound checks if an error represents a 404 Not Found response
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	hfErr, ok := err.(*Error)
	return ok && hfErr.Code == "404"
}
