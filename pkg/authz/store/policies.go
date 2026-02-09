package store

// Policy represents a policy template for API responses
type Policy struct {
	AccountID   string `json:"accountId"`
	PolicyID    string `json:"policyId"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	CedarPolicy string `json:"cedarPolicy"`
	CreatedAt   string `json:"createdAt"`
}
