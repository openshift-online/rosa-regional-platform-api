package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// APIClient provides methods for making requests to the ROSA API
type APIClient struct {
	baseURL    string
	httpClient *http.Client
	CallerARN  string
}

// APIResponse wraps an HTTP response with convenience methods
type APIResponse struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
}

// NewAPIClient creates a new API client
func NewAPIClient(baseURL string) *APIClient {
	return &APIClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Do performs an HTTP request
func (c *APIClient) Do(method, path string, body interface{}, accountID string) (*APIResponse, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if accountID != "" {
		req.Header.Set("X-Amz-Account-Id", accountID)
	}
	if c.CallerARN != "" {
		req.Header.Set("X-Amz-Caller-Arn", c.CallerARN)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return &APIResponse{
		StatusCode: resp.StatusCode,
		Body:       respBody,
		Headers:    resp.Header,
	}, nil
}

// Get performs a GET request
func (c *APIClient) Get(path, accountID string) (*APIResponse, error) {
	return c.Do(http.MethodGet, path, nil, accountID)
}

// Post performs a POST request
func (c *APIClient) Post(path string, body interface{}, accountID string) (*APIResponse, error) {
	return c.Do(http.MethodPost, path, body, accountID)
}

// Put performs a PUT request
func (c *APIClient) Put(path string, body interface{}, accountID string) (*APIResponse, error) {
	return c.Do(http.MethodPut, path, body, accountID)
}

// Delete performs a DELETE request
func (c *APIClient) Delete(path, accountID string) (*APIResponse, error) {
	return c.Do(http.MethodDelete, path, nil, accountID)
}

// JSON parses the response body as JSON
func (r *APIResponse) JSON() (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := json.Unmarshal(r.Body, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// JSONArray parses the response body as a JSON array
func (r *APIResponse) JSONArray() ([]map[string]interface{}, error) {
	var result []map[string]interface{}
	if err := json.Unmarshal(r.Body, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ----- Convenience methods for authz operations -----

// CreateAccount enables an account (requires privileged caller)
func (c *APIClient) CreateAccount(privilegedAccountID, newAccountID string, privileged bool) (string, error) {
	body := map[string]interface{}{
		"accountId":  newAccountID,
		"privileged": privileged,
	}

	resp, err := c.Post("/api/v0/accounts", body, privilegedAccountID)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to create account: status %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	data, err := resp.JSON()
	if err != nil {
		return "", err
	}

	if id, ok := data["id"].(string); ok {
		return id, nil
	}
	return newAccountID, nil
}

// CreateAdmin adds an admin to an account
func (c *APIClient) CreateAdmin(accountID, principalArn string) error {
	body := map[string]interface{}{
		"principalArn": principalArn,
	}

	resp, err := c.Post("/api/v0/authz/admins", body, accountID)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to create admin: status %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	return nil
}

// CreatePolicy creates a policy and returns its ID
func (c *APIClient) CreatePolicy(accountID, name, description string, policy string) (string, error) {
	body := map[string]interface{}{
		"name":        name,
		"description": description,
		"policy":      policy,
	}

	resp, err := c.Post("/api/v0/authz/policies", body, accountID)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to create policy: status %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	data, err := resp.JSON()
	if err != nil {
		return "", err
	}

	if id, ok := data["policyId"].(string); ok {
		return id, nil
	}
	if id, ok := data["id"].(string); ok {
		return id, nil
	}
	return "", fmt.Errorf("policy ID not found in response: %s", string(resp.Body))
}

// CreateGroup creates a group and returns its ID
func (c *APIClient) CreateGroup(accountID, name, description string) (string, error) {
	body := map[string]interface{}{
		"name":        name,
		"description": description,
	}

	resp, err := c.Post("/api/v0/authz/groups", body, accountID)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to create group: status %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	data, err := resp.JSON()
	if err != nil {
		return "", err
	}

	if id, ok := data["groupId"].(string); ok {
		return id, nil
	}
	if id, ok := data["id"].(string); ok {
		return id, nil
	}
	return "", fmt.Errorf("group ID not found in response: %s", string(resp.Body))
}

// AddGroupMembers adds members to a group
func (c *APIClient) AddGroupMembers(accountID, groupID string, members []string) error {
	body := map[string]interface{}{
		"add": members,
	}

	path := fmt.Sprintf("/api/v0/authz/groups/%s/members", groupID)
	resp, err := c.Put(path, body, accountID)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to add group members: status %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	return nil
}

// CreateAttachment attaches a policy to a target and returns the attachment ID
func (c *APIClient) CreateAttachment(accountID, policyID, targetType, targetID string) (string, error) {
	body := map[string]interface{}{
		"policyId":   policyID,
		"targetType": targetType,
		"targetId":   targetID,
	}

	resp, err := c.Post("/api/v0/authz/attachments", body, accountID)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to create attachment: status %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	data, err := resp.JSON()
	if err != nil {
		return "", err
	}

	if id, ok := data["attachmentId"].(string); ok {
		return id, nil
	}
	if id, ok := data["id"].(string); ok {
		return id, nil
	}
	return "", fmt.Errorf("attachment ID not found in response: %s", string(resp.Body))
}

// DeleteAttachment removes a policy attachment
func (c *APIClient) DeleteAttachment(accountID, attachmentID string) error {
	path := fmt.Sprintf("/api/v0/authz/attachments/%s", attachmentID)
	resp, err := c.Delete(path, accountID)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to delete attachment: status %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	return nil
}

// DeleteGroup removes a group
func (c *APIClient) DeleteGroup(accountID, groupID string) error {
	path := fmt.Sprintf("/api/v0/authz/groups/%s", groupID)
	resp, err := c.Delete(path, accountID)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to delete group: status %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	return nil
}

// DeletePolicy removes a policy
func (c *APIClient) DeletePolicy(accountID, policyID string) error {
	path := fmt.Sprintf("/api/v0/authz/policies/%s", policyID)
	resp, err := c.Delete(path, accountID)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to delete policy: status %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	return nil
}

// CheckReady checks if the service is ready
func (c *APIClient) CheckReady() error {
	resp, err := c.Get("/api/v0/ready", "")
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("service not ready: status %d", resp.StatusCode)
	}

	return nil
}

// CheckAuthorizationRequest represents a request to the check authorization endpoint
type CheckAuthorizationRequest struct {
	Principal    string            `json:"principal"`
	Action       string            `json:"action"`
	Resource     string            `json:"resource"`
	Context      map[string]any    `json:"context,omitempty"`
	ResourceTags map[string]string `json:"resourceTags,omitempty"`
}

// SeedAdminDirect inserts an admin record directly into DynamoDB Local,
// bypassing the API. This is needed to bootstrap the first admin for an
// account since the admin API itself requires admin privileges.
func SeedAdminDirect(accountID, principalArn string) error {
	endpoint := os.Getenv("DYNAMODB_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:8180"
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}

	item := fmt.Sprintf(`{
		"accountId": {"S": "%s"},
		"principalArn": {"S": "%s"},
		"createdAt": {"S": "%s"},
		"createdBy": {"S": "e2e-test-seed"}
	}`, accountID, principalArn, time.Now().UTC().Format(time.RFC3339))

	cmd := exec.Command("aws", "dynamodb", "put-item",
		"--endpoint-url", endpoint,
		"--region", region,
		"--table-name", "rosa-authz-admins",
		"--item", item,
	)
	cmd.Env = append(os.Environ(),
		"AWS_ACCESS_KEY_ID=dummy",
		"AWS_SECRET_ACCESS_KEY=dummy",
		"AWS_PAGER=",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to seed admin in DynamoDB: %w, output: %s", err, string(output))
	}
	return nil
}

// CheckAuthorization checks if a principal is authorized to perform an action on a resource
func (c *APIClient) CheckAuthorization(accountID string, req CheckAuthorizationRequest) (string, error) {
	resp, err := c.Post("/api/v0/authz/check", req, accountID)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("authorization check failed: status %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	data, err := resp.JSON()
	if err != nil {
		return "", err
	}

	if decision, ok := data["decision"].(string); ok {
		return decision, nil
	}

	return "", fmt.Errorf("decision not found in response")
}
