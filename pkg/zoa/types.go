package zoa

// ExecutionStatus represents the state of a Trusted Action execution.
type ExecutionStatus string

const (
	StatusPending   ExecutionStatus = "pending"
	StatusRunning   ExecutionStatus = "running"
	StatusSucceeded ExecutionStatus = "succeeded"
	StatusFailed    ExecutionStatus = "failed"
	StatusTimedOut  ExecutionStatus = "timed_out"
)

// OutputStatus represents the state of the S3 output upload for an execution.
type OutputStatus string

const (
	OutputStatusPending  OutputStatus = "pending"
	OutputStatusUploaded OutputStatus = "uploaded"
	OutputStatusFailed   OutputStatus = "failed"
)

// Execution represents a single Trusted Action execution stored in DynamoDB.
type Execution struct {
	ExecutionID      string          `dynamodbav:"executionId" json:"id"`
	AccountID        string          `dynamodbav:"accountId" json:"account_id,omitempty"`
	CallerARN        string          `dynamodbav:"callerArn" json:"caller_arn,omitempty"`
	Operator         string          `dynamodbav:"operator" json:"operator,omitempty"`
	Action           string          `dynamodbav:"action" json:"action"`
	ExecutedAction   string          `dynamodbav:"executedAction,omitempty" json:"executed_action,omitempty"`
	DryRun           bool            `dynamodbav:"dryRun" json:"dry_run"`
	Force            bool            `dynamodbav:"force" json:"force"`
	TargetCluster    string          `dynamodbav:"targetCluster" json:"target_cluster"`
	Scope            string          `dynamodbav:"scope" json:"scope"`
	Type             string          `dynamodbav:"type" json:"type,omitempty"`
	Params           map[string]string `dynamodbav:"params,omitempty" json:"params,omitempty"`
	Jira             string            `dynamodbav:"jira" json:"jira"`
	Revision         string            `dynamodbav:"revision,omitempty" json:"revision,omitempty"`
	Status           ExecutionStatus   `dynamodbav:"status" json:"status"`
	ManifestWorkName string          `dynamodbav:"manifestWorkName,omitempty" json:"manifest_work_name,omitempty"`
	OutputPath       string          `dynamodbav:"outputPath,omitempty" json:"output_path,omitempty"`
	OutputStatus     OutputStatus    `dynamodbav:"outputStatus,omitempty" json:"output_status,omitempty"`
	CreatedAt       string `dynamodbav:"createdAt" json:"created_at"`
	UpdatedAt       string `dynamodbav:"updatedAt,omitempty" json:"updated_at,omitempty"`
	CompletedAt     string `dynamodbav:"completedAt,omitempty" json:"completed_at,omitempty"`
	RunnerSeconds   int    `dynamodbav:"runnerSeconds,omitempty" json:"runner_seconds,omitempty"`
	UploadSeconds   int    `dynamodbav:"uploadSeconds,omitempty" json:"upload_seconds,omitempty"`
	DurationSeconds int    `dynamodbav:"durationSeconds,omitempty" json:"duration_seconds,omitempty"`
	TTL             int64  `dynamodbav:"ttl,omitempty" json:"-"`
}

// CreateRequest is the JSON body for POST /api/v0/trusted-actions/{action}/run.
type CreateRequest struct {
	TargetCluster string            `json:"target_cluster"`
	Params        map[string]string `json:"params,omitempty"`
	Jira          string            `json:"jira"`
	Force         bool              `json:"force,omitempty"`
	DryRun        bool              `json:"dry_run,omitempty"`
}

// ExecutionResponse is the full response format for GET /runs/{id}.
type ExecutionResponse struct {
	*Execution
	Output interface{} `json:"output,omitempty"`
	Logs   string      `json:"logs,omitempty"`
}

// ExecutionList wraps a paginated list response.
type ExecutionList struct {
	Items   []*Execution `json:"items"`
	Total   int          `json:"total"`
	Page    int          `json:"page"`
	Limit   int          `json:"limit"`
	HasMore bool         `json:"has_more"`
}

// TAParameter defines a parameter accepted by a Trusted Action.
type TAParameter struct {
	Name        string `yaml:"name" json:"name"`
	Required    bool   `yaml:"required" json:"required"`
	Default     string `yaml:"default,omitempty" json:"default,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// TARBAC defines RBAC rules for a Trusted Action.
type TARBAC struct {
	ClusterScoped  bool       `yaml:"cluster_scoped" json:"cluster_scoped"`
	NamespaceParam string     `yaml:"namespace_param,omitempty" json:"namespace_param,omitempty"`
	Rules          []RBACRule `yaml:"rules" json:"rules"`
}

// RBACRule mirrors a Kubernetes RBAC PolicyRule.
type RBACRule struct {
	APIGroups []string `yaml:"apiGroups" json:"apiGroups"`
	Resources []string `yaml:"resources" json:"resources"`
	Verbs     []string `yaml:"verbs" json:"verbs"`
}

// TATemplate defines a Trusted Action loaded from a simplified YAML file.
type TATemplate struct {
	Name                 string        `yaml:"name" json:"name"`
	Scope                string        `yaml:"scope" json:"scope"`
	Type                 string        `yaml:"type" json:"type"`
	Description          string        `yaml:"description" json:"description"`
	TimeoutSeconds       int           `yaml:"timeout_seconds,omitempty" json:"timeout_seconds,omitempty"`
	ApprovalRequired     bool          `yaml:"approval_required,omitempty" json:"approval_required,omitempty"`
	WriteCooldownSeconds int           `yaml:"write_cooldown_seconds,omitempty" json:"write_cooldown_seconds,omitempty"`
	DryRunAction         string        `yaml:"dry_run_action,omitempty" json:"dry_run_action,omitempty"`
	Params               []TAParameter `yaml:"params,omitempty" json:"params,omitempty"`
	RBAC                 *TARBAC       `yaml:"rbac" json:"-"`
	Script               string        `yaml:"script" json:"-"`
}

// TAListItem is the lean response for GET /trusted-actions (catalog listing).
type TAListItem struct {
	Name        string `json:"name"`
	Scope       string `json:"scope"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

// TADescribeResponse is returned by GET /trusted-actions/{action}.
type TADescribeResponse struct {
	Name                 string        `json:"name"`
	Scope                string        `json:"scope"`
	Type                 string        `json:"type"`
	Description          string        `json:"description"`
	ApprovalRequired     bool          `json:"approval_required,omitempty"`
	WriteCooldownSeconds int           `json:"write_cooldown_seconds,omitempty"`
	DryRunAction         string        `json:"dry_run_action,omitempty"`
	Params               []TAParameter `json:"params,omitempty"`
	RequiredFields       []string      `json:"required_fields"`
}

// JobConfig holds boilerplate configuration for Job generation,
// loaded from the zoa-job-config ConfigMap.
type JobConfig struct {
	Image                   string `json:"image"`
	Revision                string `json:"revision"`
	CPURequest              string `json:"cpu_request"`
	MemoryRequest           string `json:"memory_request"`
	CPULimit                string `json:"cpu_limit"`
	MemoryLimit             string `json:"memory_limit"`
	TTLSeconds              int32  `json:"ttl_seconds"`
	ExecutionTimeoutSeconds int    `json:"execution_timeout_seconds"`
	EntrypointScript        string `json:"entrypoint_script"`
	UploadTimeoutSeconds    int    `json:"upload_timeout_seconds"`
	UploadEntrypointScript  string `json:"upload_entrypoint_script"`
	WriteCooldownSeconds    int    `json:"write_cooldown_seconds"`
	MaxConcurrentPerTarget  int    `json:"max_concurrent_per_target"`
}

// RenderContext holds all the data needed to generate a ManifestWork for a TA execution.
type RenderContext struct {
	ExecID        string
	ActionName    string
	TargetCluster string
	Namespace     string
	OutputBucket  string
	Operator      string
	Revision      string
	Type          string
	Scope         string
	Params        map[string]string
	Config        JobConfig
}
