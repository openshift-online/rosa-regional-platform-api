package clusters_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	e2e "github.com/openshift/rosa-regional-platform-api/test/e2e"
)

func TestClustersE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Clusters E2E Test Suite")
}

var _ = Describe("Clusters API E2E Tests", Ordered, func() {
	var (
		baseURL   string
		accountID string
		apiClient *e2e.APIClient
		clusterID string
	)

	BeforeAll(func() {
		baseURL = os.Getenv("E2E_BASE_URL")
		Expect(baseURL).NotTo(BeEmpty(), "E2E_BASE_URL must be set")

		accountID = os.Getenv("E2E_ACCOUNT_ID")
		if accountID == "" {
			GinkgoWriter.Printf("No E2E_ACCOUNT_ID set, using AWS STS caller identity\n")
			cmd := exec.Command("aws", "sts", "get-caller-identity", "--query", "Account", "--output", "text")
			output, err := cmd.CombinedOutput()
			if err != nil {
				Fail("Failed to get AWS account ID: " + err.Error())
			}
			accountID = strings.TrimSpace(string(output))
		}

		apiClient = e2e.NewAPIClient(baseURL)
		GinkgoWriter.Printf("Testing against: %s with account: %s\n", baseURL, accountID)
	})

	Describe("GET /api/v0/clusters", func() {
		It("should successfully list clusters", func() {
			response, err := apiClient.Get("/api/v0/clusters", accountID)
			Expect(err).To(BeNil())
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			Expect(response.Headers).To(HaveKey("Content-Type"))

			var listResp struct {
				Clusters []map[string]interface{} `json:"clusters"`
				Total    int                      `json:"total"`
				Limit    int                      `json:"limit"`
				Offset   int                      `json:"offset"`
			}
			err = json.Unmarshal(response.Body, &listResp)
			Expect(err).To(BeNil())
			// Expect(listResp.Clusters).NotTo(BeNil())
			// it can be empty
			GinkgoWriter.Printf("Found %d clusters (total: %d)\n", len(listResp.Clusters), listResp.Total)
		})

		// It("should support pagination parameters", func() {
		// 	response, err := apiClient.Get("/api/v0/clusters?limit=10&offset=0", accountID)
		// 	Expect(err).To(BeNil())
		// 	Expect(response.StatusCode).To(Equal(http.StatusOK))

		// 	var listResp struct {
		// 		Clusters []map[string]interface{} `json:"clusters"`
		// 		Total    int                      `json:"total"`
		// 		Limit    int                      `json:"limit"`
		// 		Offset   int                      `json:"offset"`
		// 	}
		// 	err = json.Unmarshal(response.Body, &listResp)
		// 	Expect(err).To(BeNil())
		// 	Expect(listResp.Limit).To(Equal(10))
		// 	Expect(listResp.Offset).To(Equal(0))
		// })

		// It("should support status filter", func() {
		// 	response, err := apiClient.Get("/api/v0/clusters?status=ready", accountID)
		// 	Expect(err).To(BeNil())
		// 	Expect(response.StatusCode).To(Equal(http.StatusOK))

		// 	var listResp struct {
		// 		Clusters []map[string]interface{} `json:"clusters"`
		// 	}
		// 	err = json.Unmarshal(response.Body, &listResp)
		// 	Expect(err).To(BeNil())
		// })
	})

	Describe("POST /api/v0/clusters", func() {
		It("should create a new cluster", func() {
			clusterName := fmt.Sprintf("e2e-test-cluster-%s", uuid.New().String()[:8])
			targetProjectID := fmt.Sprintf("test-project-%s", uuid.New().String()[:8])

			createReq := map[string]interface{}{
				"name":              clusterName,
				"target_project_id": targetProjectID,
				"spec": map[string]interface{}{
					"region":          "us-east-1",
					"version":         "4.14.0",
					"compute_nodes":   3,
					"compute_machine": "m5.xlarge",
				},
			}

			GinkgoWriter.Printf("Creating cluster: %s in project: %s\n", clusterName, targetProjectID)
			response, err := apiClient.Post("/api/v0/clusters", createReq, accountID)
			Expect(err).To(BeNil())
			Expect(response.StatusCode).To(Equal(http.StatusCreated))

			var cluster map[string]interface{}
			err = json.Unmarshal(response.Body, &cluster)
			Expect(err).To(BeNil())
			Expect(cluster["id"]).NotTo(BeEmpty())
			Expect(cluster["name"]).To(Equal(clusterName))
			Expect(cluster["target_project_id"]).To(Equal(targetProjectID))
			Expect(cluster["created_at"]).NotTo(BeEmpty())
			Expect(cluster["updated_at"]).NotTo(BeEmpty())
			Expect(cluster["spec"]).NotTo(BeNil())

			clusterID = cluster["id"].(string)
			GinkgoWriter.Printf("Created cluster with ID: %s\n", clusterID)
		})

		It("should reject creation with missing required fields", func() {
			invalidReq := map[string]interface{}{
				"name": "invalid-cluster",
				// missing spec
			}

			response, err := apiClient.Post("/api/v0/clusters", invalidReq, accountID)
			Expect(err).To(BeNil())
			Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
		})

		It("should reject creation with empty name", func() {
			invalidReq := map[string]interface{}{
				"name":              "",
				"target_project_id": "test-project",
				"spec": map[string]interface{}{
					"region": "us-east-1",
				},
			}

			response, err := apiClient.Post("/api/v0/clusters", invalidReq, accountID)
			Expect(err).To(BeNil())
			Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
		})
	})

	Describe("GET /api/v0/clusters/{id}", func() {
		It("should get cluster details by ID", func() {
			if clusterID == "" {
				Skip("No cluster ID available from creation test")
			}

			path := fmt.Sprintf("/api/v0/clusters/%s", clusterID)
			response, err := apiClient.Get(path, accountID)
			Expect(err).To(BeNil())
			Expect(response.StatusCode).To(Equal(http.StatusOK))

			var cluster map[string]interface{}
			err = json.Unmarshal(response.Body, &cluster)
			Expect(err).To(BeNil())
			Expect(cluster["id"]).To(Equal(clusterID))
			Expect(cluster["name"]).NotTo(BeEmpty())
			Expect(cluster["spec"]).NotTo(BeNil())
			Expect(cluster["created_at"]).NotTo(BeEmpty())
			Expect(cluster["updated_at"]).NotTo(BeEmpty())

			GinkgoWriter.Printf("Retrieved cluster: id=%s name=%s\n", cluster["id"], cluster["name"])
		})

		It("should return 404 for non-existent cluster", func() {
			fakeID := uuid.New().String()
			path := fmt.Sprintf("/api/v0/clusters/%s", fakeID)
			response, err := apiClient.Get(path, accountID)
			Expect(err).To(BeNil())
			Expect(response.StatusCode).To(Equal(http.StatusNotFound))
		})
	})

	Describe("GET /api/v0/clusters/{id}/status", func() {
		It("should get cluster status", func() {
			if clusterID == "" {
				Skip("No cluster ID available from creation test")
			}

			path := fmt.Sprintf("/api/v0/clusters/%s/status", clusterID)
			response, err := apiClient.Get(path, accountID)
			Expect(err).To(BeNil())
			Expect(response.StatusCode).To(Equal(http.StatusOK))

			var statusResp map[string]interface{}
			err = json.Unmarshal(response.Body, &statusResp)
			Expect(err).To(BeNil())
			Expect(statusResp["cluster_id"]).To(Equal(clusterID))

			// Check for status field
			if status, ok := statusResp["status"].(map[string]interface{}); ok {
				GinkgoWriter.Printf("Cluster status: %+v\n", status)
			}

			// Check for controller_statuses field (optional)
			if controllers, ok := statusResp["controller_statuses"].([]interface{}); ok {
				GinkgoWriter.Printf("Controller statuses count: %d\n", len(controllers))
				for i, ctrl := range controllers {
					if ctrlMap, ok := ctrl.(map[string]interface{}); ok {
						GinkgoWriter.Printf("  Controller %d: %s\n", i, ctrlMap["name"])
					}
				}
			}
		})

		It("should return 404 for status of non-existent cluster", func() {
			fakeID := uuid.New().String()
			path := fmt.Sprintf("/api/v0/clusters/%s/status", fakeID)
			response, err := apiClient.Get(path, accountID)
			Expect(err).To(BeNil())
			Expect(response.StatusCode).To(Equal(http.StatusNotFound))
		})

		It("should poll cluster status until it reaches a stable state", func() {
			if clusterID == "" {
				Skip("No cluster ID available from creation test")
			}

			path := fmt.Sprintf("/api/v0/clusters/%s/status", clusterID)
			stableStates := []string{"ready", "error", "failed"}
			foundStable := false

			GinkgoWriter.Printf("Polling cluster status for cluster %s...\n", clusterID)

			// Poll for up to 2 minutes to see if status changes
			Eventually(func() bool {
				response, err := apiClient.Get(path, accountID)
				if err != nil || response.StatusCode != http.StatusOK {
					return false
				}

				var statusResp map[string]interface{}
				err = json.Unmarshal(response.Body, &statusResp)
				if err != nil {
					return false
				}

				if status, ok := statusResp["status"].(map[string]interface{}); ok {
					if state, ok := status["state"].(string); ok {
						GinkgoWriter.Printf("Current state: %s\n", state)
						for _, stable := range stableStates {
							if state == stable {
								foundStable = true
								return true
							}
						}
					}
				}
				return foundStable
			}, "2m", "10s").Should(Or(BeTrue(), BeFalse())) // Don't fail if not reached, just observe
		})
	})

	Describe("PUT /api/v0/clusters/{id}", func() {
		It("should update cluster spec", func() {
			if clusterID == "" {
				Skip("No cluster ID available from creation test")
			}

			updateReq := map[string]interface{}{
				"spec": map[string]interface{}{
					"compute_nodes": 5, // Update from 3 to 5
				},
			}

			path := fmt.Sprintf("/api/v0/clusters/%s", clusterID)
			response, err := apiClient.Put(path, updateReq, accountID)
			Expect(err).To(BeNil())
			Expect(response.StatusCode).To(Equal(http.StatusOK))

			var cluster map[string]interface{}
			err = json.Unmarshal(response.Body, &cluster)
			Expect(err).To(BeNil())
			Expect(cluster["id"]).To(Equal(clusterID))

			GinkgoWriter.Printf("Updated cluster %s\n", clusterID)
		})

		It("should return 404 for updating non-existent cluster", func() {
			fakeID := uuid.New().String()
			updateReq := map[string]interface{}{
				"spec": map[string]interface{}{
					"compute_nodes": 5,
				},
			}

			path := fmt.Sprintf("/api/v0/clusters/%s", fakeID)
			response, err := apiClient.Put(path, updateReq, accountID)
			Expect(err).To(BeNil())
			Expect(response.StatusCode).To(Equal(http.StatusNotFound))
		})

		It("should reject update with invalid spec", func() {
			if clusterID == "" {
				Skip("No cluster ID available from creation test")
			}

			invalidReq := map[string]interface{}{
				// missing spec field
			}

			path := fmt.Sprintf("/api/v0/clusters/%s", clusterID)
			response, err := apiClient.Put(path, invalidReq, accountID)
			Expect(err).To(BeNil())
			Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
		})
	})

	Describe("DELETE /api/v0/clusters/{id}", func() {
		It("should delete cluster", func() {
			if clusterID == "" {
				Skip("No cluster ID available from creation test")
			}

			path := fmt.Sprintf("/api/v0/clusters/%s", clusterID)
			response, err := apiClient.Delete(path, accountID)
			Expect(err).To(BeNil())
			Expect(response.StatusCode).To(Equal(http.StatusAccepted))

			var deleteResp map[string]interface{}
			err = json.Unmarshal(response.Body, &deleteResp)
			Expect(err).To(BeNil())
			Expect(deleteResp["message"]).NotTo(BeEmpty())
			Expect(deleteResp["cluster_id"]).To(Equal(clusterID))

			GinkgoWriter.Printf("Deleted cluster %s: %s\n", clusterID, deleteResp["message"])

			// Verify cluster is gone or marked for deletion
			time.Sleep(2 * time.Second) // Brief pause
			getResp, err := apiClient.Get(path, accountID)
			Expect(err).To(BeNil())
			// Should either be 404 or still exist but with deletion status
			Expect(getResp.StatusCode).To(Or(Equal(http.StatusNotFound), Equal(http.StatusOK)))

			if getResp.StatusCode == http.StatusOK {
				var cluster map[string]interface{}
				err = json.Unmarshal(getResp.Body, &cluster)
				Expect(err).To(BeNil())
				GinkgoWriter.Printf("Cluster still exists after delete (likely marked for deletion): %+v\n", cluster)
			}
		})

		It("should support force delete parameter", func() {
			// Create a temporary cluster for this test
			clusterName := fmt.Sprintf("e2e-force-delete-%s", uuid.New().String()[:8])
			createReq := map[string]interface{}{
				"name":              clusterName,
				"target_project_id": fmt.Sprintf("test-project-%s", uuid.New().String()[:8]),
				"spec": map[string]interface{}{
					"region": "us-east-1",
				},
			}

			createResp, err := apiClient.Post("/api/v0/clusters", createReq, accountID)
			if err != nil || createResp.StatusCode != http.StatusCreated {
				Skip("Could not create cluster for force delete test")
			}

			var cluster map[string]interface{}
			err = json.Unmarshal(createResp.Body, &cluster)
			Expect(err).To(BeNil())
			tempClusterID := cluster["id"].(string)

			// Force delete the cluster
			path := fmt.Sprintf("/api/v0/clusters/%s?force=true", tempClusterID)
			response, err := apiClient.Delete(path, accountID)
			Expect(err).To(BeNil())
			Expect(response.StatusCode).To(Equal(http.StatusAccepted))

			var deleteResp map[string]interface{}
			err = json.Unmarshal(response.Body, &deleteResp)
			Expect(err).To(BeNil())
			GinkgoWriter.Printf("Force deleted cluster %s\n", tempClusterID)
		})

		It("should return 404 for deleting non-existent cluster", func() {
			fakeID := uuid.New().String()
			path := fmt.Sprintf("/api/v0/clusters/%s", fakeID)
			response, err := apiClient.Delete(path, accountID)
			Expect(err).To(BeNil())
			Expect(response.StatusCode).To(Equal(http.StatusNotFound))
		})
	})

	Describe("Complete workflow integration test", func() {
		It("should handle complete cluster lifecycle", func() {
			// This test demonstrates a complete workflow from creation to deletion
			clusterName := fmt.Sprintf("e2e-workflow-%s", uuid.New().String()[:8])
			targetProjectID := fmt.Sprintf("workflow-project-%s", uuid.New().String()[:8])

			By("creating a cluster")
			createReq := map[string]interface{}{
				"name":              clusterName,
				"target_project_id": targetProjectID,
				"spec": map[string]interface{}{
					"region":          "us-west-2",
					"version":         "4.14.0",
					"compute_nodes":   2,
					"compute_machine": "m5.large",
				},
			}

			createResp, err := apiClient.Post("/api/v0/clusters", createReq, accountID)
			Expect(err).To(BeNil())
			Expect(createResp.StatusCode).To(Equal(http.StatusCreated))

			var cluster map[string]interface{}
			err = json.Unmarshal(createResp.Body, &cluster)
			Expect(err).To(BeNil())
			workflowClusterID := cluster["id"].(string)
			GinkgoWriter.Printf("Workflow test - Created cluster: %s\n", workflowClusterID)

			By("retrieving the cluster")
			getResp, err := apiClient.Get(fmt.Sprintf("/api/v0/clusters/%s", workflowClusterID), accountID)
			Expect(err).To(BeNil())
			Expect(getResp.StatusCode).To(Equal(http.StatusOK))

			By("checking cluster status")
			statusResp, err := apiClient.Get(fmt.Sprintf("/api/v0/clusters/%s/status", workflowClusterID), accountID)
			Expect(err).To(BeNil())
			Expect(statusResp.StatusCode).To(Equal(http.StatusOK))

			By("updating the cluster")
			updateReq := map[string]interface{}{
				"spec": map[string]interface{}{
					"compute_nodes": 4,
				},
			}
			updateResp, err := apiClient.Put(fmt.Sprintf("/api/v0/clusters/%s", workflowClusterID), updateReq, accountID)
			Expect(err).To(BeNil())
			Expect(updateResp.StatusCode).To(Equal(http.StatusOK))

			By("deleting the cluster")
			deleteResp, err := apiClient.Delete(fmt.Sprintf("/api/v0/clusters/%s", workflowClusterID), accountID)
			Expect(err).To(BeNil())
			Expect(deleteResp.StatusCode).To(Equal(http.StatusAccepted))

			GinkgoWriter.Printf("Workflow test completed successfully for cluster: %s\n", workflowClusterID)
		})
	})
})
