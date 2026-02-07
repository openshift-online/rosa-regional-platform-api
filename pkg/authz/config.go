package authz

// Config holds the configuration for the authorization service
type Config struct {
	// AWSRegion is the AWS region for AVP and DynamoDB
	AWSRegion string

	// Table names for DynamoDB
	AccountsTableName string
	AdminsTableName   string
	GroupsTableName   string
	MembersTableName  string

	// Enabled determines if Cedar/AVP authorization is enabled
	// When false, falls back to legacy allowlist behavior
	Enabled bool

	// DynamoDBEndpoint overrides the DynamoDB endpoint for local development
	// Leave empty to use AWS default
	DynamoDBEndpoint string

	// CedarAgentEndpoint is the URL for cedar-agent (local testing only)
	// When set, MockAVPClient is used instead of real AVP
	CedarAgentEndpoint string
}

// DefaultConfig returns the default authorization configuration
func DefaultConfig() *Config {
	return &Config{
		AWSRegion:         "us-east-1",
		AccountsTableName: "rosa-authz-accounts",
		AdminsTableName:   "rosa-authz-admins",
		GroupsTableName:   "rosa-authz-groups",
		MembersTableName:  "rosa-authz-group-members",
		Enabled:           true,
	}
}
