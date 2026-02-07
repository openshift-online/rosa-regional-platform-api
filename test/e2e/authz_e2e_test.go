package e2e_test

import (
	"fmt"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Test configuration
const (
	// PrivilegedAccountID is used as the caller for privileged operations
	PrivilegedAccountID = "000000000000"

	// DefaultTimeout for API operations
	DefaultTimeout = 30 * time.Second
)

var _ = Describe("Authz E2E Tests", Ordered, func() {
	var (
		client       *APIClient
		testPolicies []PolicyTestFile
	)

	BeforeAll(func() {
		// Get base URL from environment or use default
		baseURL := os.Getenv("E2E_BASE_URL")
		if baseURL == "" {
			baseURL = "http://localhost:8000"
		}
		client = NewAPIClient(baseURL)

		// Wait for service to be ready
		Eventually(func() error {
			return client.CheckReady()
		}, DefaultTimeout, 1*time.Second).Should(Succeed(), "Service should be ready")

		// Load all test policies
		var err error
		testPolicies, err = LoadAllTestPolicies()
		Expect(err).NotTo(HaveOccurred(), "Should load test policies")
		Expect(testPolicies).NotTo(BeEmpty(), "Should have test policies")

		GinkgoWriter.Printf("Loaded %d test policy files\n", len(testPolicies))
	})

	// Test each policy file using DescribeTable pattern
	Context("Policy Authorization Tests", func() {
		It("should evaluate all policy test cases", func() {
			totalTests := 0
			passedTests := 0
			failedTests := 0

			for _, policyFile := range testPolicies {
				GinkgoWriter.Printf("\n=== Testing Policy: %s ===\n", policyFile.Name)
				GinkgoWriter.Printf("Description: %s\n", policyFile.Description)
				GinkgoWriter.Printf("Test cases: %d\n", len(policyFile.TestCases))

				// Create unique test account for this policy
				testAccountID := fmt.Sprintf("test-%d", time.Now().UnixNano())
				testAdminARN := fmt.Sprintf("arn:aws:iam::%s:user/e2e-admin", testAccountID)

				// Setup: Create account — failure must abort this policy's tests
				_, err := client.CreateAccount(PrivilegedAccountID, testAccountID, false)
				Expect(err).NotTo(HaveOccurred(), "Failed to create account for policy %s", policyFile.Name)

				// Setup: Seed admin directly in DynamoDB (bootstrap: can't use the API
				// to add the first admin since RequireAdmin blocks unauthenticated calls)
				err = SeedAdminDirect(testAccountID, testAdminARN)
				Expect(err).NotTo(HaveOccurred(), "Failed to seed admin for policy %s", policyFile.Name)

				// Set caller ARN so subsequent authz management calls pass RequireAdmin
				client.CallerARN = testAdminARN

				// Setup: Create policy
				policyID, err := client.CreatePolicy(
					testAccountID,
					policyFile.Name,
					policyFile.Description,
					policyFile.Policy,
				)
				Expect(err).NotTo(HaveOccurred(), "Failed to create policy %s", policyFile.Name)

				// Setup: Create group
				groupID, err := client.CreateGroup(testAccountID, "test-group", "Test group for e2e")
				Expect(err).NotTo(HaveOccurred(), "Failed to create group for policy %s", policyFile.Name)

				// Setup: Attach policy to group
				attachmentID, err := client.CreateAttachment(testAccountID, policyID, "group", groupID)
				Expect(err).NotTo(HaveOccurred(), "Failed to attach policy %s to group", policyFile.Name)

				// Run each test case
				for i, tc := range policyFile.TestCases {
					totalTests++
					testName := fmt.Sprintf("[%s/%d] %s", policyFile.ID, i+1, tc.Description)

					// Determine principal
					principal := fmt.Sprintf("arn:aws:iam::%s:user/testuser", testAccountID)
					if tc.Principal != nil && tc.Principal.Username != "" {
						principal = fmt.Sprintf("arn:aws:iam::%s:user/%s", testAccountID, tc.Principal.Username)
					}

					// Add user to group
					err := client.AddGroupMembers(testAccountID, groupID, []string{principal})
					Expect(err).NotTo(HaveOccurred(), "Failed to add member for test case %s", testName)

					// Handle additional policies for this test case
					var additionalAttachmentIDs []string
					for j, additionalCedar := range tc.AdditionalPolicies {
						addPolicyID, err := client.CreatePolicy(
							testAccountID,
							fmt.Sprintf("%s-additional-%d", policyFile.Name, j),
							"Additional policy for test case",
							additionalCedar,
						)
						Expect(err).NotTo(HaveOccurred(), "Failed to create additional policy %d for %s", j, testName)

						addAttachID, err := client.CreateAttachment(testAccountID, addPolicyID, "group", groupID)
						Expect(err).NotTo(HaveOccurred(), "Failed to attach additional policy %d for %s", j, testName)
						additionalAttachmentIDs = append(additionalAttachmentIDs, addAttachID)
					}

					// Build resource tags as string map, handling non-string values
					resourceTags := make(map[string]string)
					for k, v := range tc.Request.ResourceTags {
						resourceTags[k] = fmt.Sprintf("%v", v)
					}

					// Call the authorization check endpoint
					authzReq := CheckAuthorizationRequest{
						Principal:    principal,
						Action:       tc.Request.Action,
						Resource:     tc.Request.Resource,
						Context:      tc.Request.Context,
						ResourceTags: resourceTags,
					}

					decision, err := client.CheckAuthorization(testAccountID, authzReq)
					if err != nil {
						GinkgoWriter.Printf("  %s: ERROR (%v)\n", testName, err)
						failedTests++
					} else if decision == tc.ExpectedResult {
						GinkgoWriter.Printf("  %s: PASS (got %s)\n", testName, decision)
						passedTests++
					} else {
						GinkgoWriter.Printf("  %s: FAIL (got %s, expected %s) action=%s resource=%s\n",
							testName, decision, tc.ExpectedResult, tc.Request.Action, tc.Request.Resource)
						failedTests++
					}

					// Cleanup additional policies
					for _, attID := range additionalAttachmentIDs {
						_ = client.DeleteAttachment(testAccountID, attID)
					}
				}

				// Cleanup
				_ = client.DeleteAttachment(testAccountID, attachmentID)
				_ = client.DeleteGroup(testAccountID, groupID)
				_ = client.DeletePolicy(testAccountID, policyID)
			}

			GinkgoWriter.Printf("\n=== Test Summary ===\n")
			GinkgoWriter.Printf("Total: %d, Passed: %d, Failed: %d\n",
				totalTests, passedTests, failedTests)
			Expect(failedTests).To(Equal(0), "All tests should pass")
		})
	})

	// Category validation tests — verify test data loads and has expected structure
	Context("Category Validation", func() {
		It("should load 01-basic-access policies with valid Cedar", func() {
			policies, err := LoadTestPoliciesByCategory("01-basic-access")
			Expect(err).NotTo(HaveOccurred())
			Expect(policies).NotTo(BeEmpty())

			for _, p := range policies {
				Expect(p.TestCases).NotTo(BeEmpty(), "Policy %s should have test cases", p.Name)
				Expect(p.Policy).NotTo(BeEmpty(), "Policy %s should have Cedar policy text", p.Name)
			}
		})

		It("should load 02-cluster-management policies with valid Cedar", func() {
			policies, err := LoadTestPoliciesByCategory("02-cluster-management")
			Expect(err).NotTo(HaveOccurred())
			Expect(policies).NotTo(BeEmpty())

			for _, p := range policies {
				Expect(p.TestCases).NotTo(BeEmpty(), "Policy %s should have test cases", p.Name)
				Expect(p.Policy).NotTo(BeEmpty(), "Policy %s should have Cedar policy text", p.Name)
			}
		})

		It("should load 05-tag-based-access policies with resource tag conditions", func() {
			policies, err := LoadTestPoliciesByCategory("05-tag-based-access")
			Expect(err).NotTo(HaveOccurred())
			Expect(policies).NotTo(BeEmpty())

			for _, p := range policies {
				Expect(p.TestCases).NotTo(BeEmpty(), "Policy %s should have test cases", p.Name)
				Expect(p.Policy).To(ContainSubstring("tags"), "Policy %s should reference tags", p.Name)
			}
		})

		It("should load 06-deny-policies with forbid statements", func() {
			policies, err := LoadTestPoliciesByCategory("06-deny-policies")
			Expect(err).NotTo(HaveOccurred())
			Expect(policies).NotTo(BeEmpty())

			for _, p := range policies {
				Expect(p.TestCases).NotTo(BeEmpty(), "Policy %s should have test cases", p.Name)
				Expect(strings.Contains(p.Policy, "forbid(")).To(BeTrue(),
					"Policy %s should contain at least one forbid() statement", p.Name)
			}
		})

		It("should load 08-complex-scenarios policies", func() {
			policies, err := LoadTestPoliciesByCategory("08-complex-scenarios")
			Expect(err).NotTo(HaveOccurred())
			Expect(policies).NotTo(BeEmpty())

			for _, p := range policies {
				Expect(p.TestCases).NotTo(BeEmpty(), "Policy %s should have test cases", p.Name)
				Expect(p.Policy).NotTo(BeEmpty(), "Policy %s should have Cedar policy text", p.Name)
			}
		})
	})
})
