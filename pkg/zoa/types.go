package zoa

// ExecutionStatus represents the state of a Trusted Action execution.
type ExecutionStatus string

const (
	StatusPending   ExecutionStatus = "pending"
	StatusRunning   ExecutionStatus = "running"
	StatusSucceeded ExecutionStatus = "succeeded"
	StatusFailed    ExecutionStatus = "failed"
)

// Execution represents a single Trusted Action execution stored in DynamoDB.
type Execution struct {
	ExecutionID      string          `dynamodbav:"executionId" json:"id"`
	AccountID        string          `dynamodbav:"accountId" json:"account_id"`
	CallerARN        string          `dynamodbav:"callerArn" json:"caller_arn"`
	Action           string          `dynamodbav:"action" json:"action"`
	TargetCluster    string          `dynamodbav:"targetCluster" json:"target_cluster"`
	Scope            string          `dynamodbav:"scope" json:"scope"`
	Status           ExecutionStatus `dynamodbav:"status" json:"status"`
	ManifestWorkName string          `dynamodbav:"manifestWorkName,omitempty" json:"manifest_work_name,omitempty"`
	OutputPath       string          `dynamodbav:"outputPath,omitempty" json:"output_path,omitempty"`
	OutputURL        string          `dynamodbav:"-" json:"output_url,omitempty"`
	CreatedAt        string          `dynamodbav:"createdAt" json:"created_at"`
	UpdatedAt        string          `dynamodbav:"updatedAt" json:"updated_at"`
	CompletedAt      string          `dynamodbav:"completedAt,omitempty" json:"completed_at,omitempty"`
	DurationSeconds  int             `dynamodbav:"duration,omitempty" json:"duration_seconds,omitempty"`
}

// CreateRequest is the JSON body for POST /api/v0/trusted_actions/{action}.
type CreateRequest struct {
	TargetCluster string            `json:"target_cluster"`
	Params        map[string]string `json:"params,omitempty"`
}

// ExecutionList wraps a list response for the API.
type ExecutionList struct {
	Kind  string       `json:"kind"`
	Items []*Execution `json:"items"`
	Total int          `json:"total"`
}
