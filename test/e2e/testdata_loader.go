package e2e_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// PolicyTestFile represents a test policy file from testdata
type PolicyTestFile struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Policy      string     `json:"policy,omitempty"`
	PolicyFile  string     `json:"policyFile,omitempty"`
	TestCases   []TestCase `json:"testCases"`
	Notes       string     `json:"notes,omitempty"`
}

// TestCase represents a single authorization test case
type TestCase struct {
	Description        string         `json:"description"`
	Principal          *TestPrincipal `json:"principal,omitempty"`
	Request            TestRequest    `json:"request"`
	ExpectedResult     string         `json:"expectedResult"` // "ALLOW", "DENY", "NOT_EVALUATED"
	AdditionalPolicies []string       `json:"additionalPolicies,omitempty"`
}

// TestPrincipal represents the principal for a test case
type TestPrincipal struct {
	Username string            `json:"username,omitempty"`
	Tags     map[string]string `json:"tags,omitempty"`
}

// TestRequest represents the authorization request for a test case
type TestRequest struct {
	Action       string         `json:"action"`
	Resource     string         `json:"resource"`
	Context      map[string]any `json:"context,omitempty"`
	ResourceTags map[string]any `json:"resourceTags,omitempty"`
}

// getTestDataDir returns the path to the testdata directory
func getTestDataDir() string {
	_, filename, _, _ := runtime.Caller(0)
	testDir := filepath.Dir(filename)
	return filepath.Join(testDir, "..", "..", "pkg", "authz", "testdata", "policies")
}

// LoadAllTestPolicies loads all test policy files from testdata
func LoadAllTestPolicies() ([]PolicyTestFile, error) {
	baseDir := getTestDataDir()
	var policies []PolicyTestFile

	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".json" {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		var policy PolicyTestFile
		if err := json.Unmarshal(data, &policy); err != nil {
			return err
		}

		if err := policy.loadPolicyFile(filepath.Dir(path)); err != nil {
			return err
		}

		// Add relative path for debugging
		relPath, _ := filepath.Rel(baseDir, path)
		if policy.Name == "" {
			policy.Name = relPath
		}

		policies = append(policies, policy)
		return nil
	})

	return policies, err
}

// loadPolicyFile reads the Cedar policy from the companion .cedar file if policyFile is set.
func (p *PolicyTestFile) loadPolicyFile(dir string) error {
	if p.PolicyFile == "" || p.Policy != "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(dir, p.PolicyFile))
	if err != nil {
		return fmt.Errorf("failed to read policy file %s: %w", p.PolicyFile, err)
	}
	p.Policy = string(data)
	return nil
}

// LoadTestPoliciesByCategory loads test policies from a specific category
func LoadTestPoliciesByCategory(category string) ([]PolicyTestFile, error) {
	baseDir := filepath.Join(getTestDataDir(), category)
	var policies []PolicyTestFile

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(baseDir, entry.Name()))
		if err != nil {
			return nil, err
		}

		var policy PolicyTestFile
		if err := json.Unmarshal(data, &policy); err != nil {
			return nil, err
		}

		if err := policy.loadPolicyFile(baseDir); err != nil {
			return nil, err
		}

		policies = append(policies, policy)
	}

	return policies, nil
}
