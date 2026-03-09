package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// apiGatewayPublicURL stores the public URL of the API Gateway found in the AWS account
var apiGatewayPublicURL string

// createPayloadJSON creates a dynamic payload.json file similar to the bash script
// It generates a ManifestWork with a timestamp and wraps it in a payload structure
func createPayloadJSON(clusterID string, outputPath string) (string, error) {
	timestamp := time.Now().Unix()
	timestampStr := fmt.Sprintf("%d", timestamp)

	// Create the ManifestWork structure
	manifestWork := map[string]interface{}{
		"apiVersion": "work.open-cluster-management.io/v1",
		"kind":       "ManifestWork",
		"metadata": map[string]interface{}{
			"name": fmt.Sprintf("maestro-payload-%s", timestampStr),
		},
		"spec": map[string]interface{}{
			"workload": map[string]interface{}{
				"manifests": []map[string]interface{}{
					{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name":      fmt.Sprintf("maestro-payload-%s", timestampStr),
							"namespace": "default",
							"labels": map[string]string{
								"test":      "maestro-distribution",
								"timestamp": timestampStr,
							},
						},
						"data": map[string]string{
							"message":             fmt.Sprintf("Hello from Regional Cluster via Maestro MQTT %s", timestampStr),
							"cluster_source":      "regional-cluster",
							"cluster_destination": clusterID,
							"transport":           "aws-iot-core-mqtt",
							"test_id":             timestampStr,
							"payload_size":        "This tests MQTT payload distribution through AWS IoT Core",
						},
					},
				},
			},
			"deleteOption": map[string]string{
				"propagationPolicy": "Foreground",
			},
			"manifestConfigs": []map[string]interface{}{
				{
					"resourceIdentifier": map[string]string{
						"group":     "",
						"resource":  "configmaps",
						"namespace": "default",
						"name":      fmt.Sprintf("maestro-payload-%s", timestampStr),
					},
					"feedbackRules": []map[string]interface{}{
						{
							"type": "JSONPaths",
							"jsonPaths": []map[string]string{
								{
									"name": "status",
									"path": ".metadata",
								},
							},
						},
					},
					"updateStrategy": map[string]string{
						"type": "ServerSideApply",
					},
				},
			},
		},
	}

	// Create the payload structure
	payload := map[string]interface{}{
		"cluster_id": clusterID,
		"data":       manifestWork,
	}

	// Marshal to JSON with indentation
	jsonData, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload JSON: %w", err)
	}

	// Write to file
	err = os.WriteFile(outputPath, jsonData, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write payload.json: %w", err)
	}

	return timestampStr, nil
}

// getAndExpectOK performs a GET request and asserts no error. If bodyContains is non-empty,
// it asserts the response body contains that substring. Returns the response for further assertions or logging.
func getAndExpectOK(client *APIClient, path, accountID, bodyContains string) *APIResponse {
	response, err := client.Get(path, accountID)
	Expect(err).To(BeNil())
	if bodyContains != "" {
		Expect(response.Body).To(ContainSubstring(bodyContains))
	}
	return response
}

// runCommandWithTimeout executes a command with a timeout and returns the output and error
// The context is automatically cancelled after the command completes or times out
func runCommandWithTimeout(timeout time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	return output, err
}

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ROSA Regional Platform API E2E Suite")
}

// Ordered for now, as our test size is small
var _ = Describe("Platform API", Ordered, func() {
	var (
		baseURL   string
		accountID string
		apiClient *APIClient
	)

	BeforeAll(func() {
		baseURL = os.Getenv("E2E_BASE_URL")
		accountID = os.Getenv("E2E_ACCOUNT_ID")
		apiClient = NewAPIClient(baseURL)
	})

	It("should basic passing test", func() {
		// Placeholder for future e2e tests
		Expect(true).To(BeTrue())
	})

	It("should have BASE_URL set with valid URL: "+baseURL, func() {

		Expect(baseURL).NotTo(BeEmpty())
		Expect(baseURL).To(MatchRegexp("^https?://.*$"))
		// Validate API Gateway URL format: https://<api-id>.execute-api.<region>.amazonaws.com[/<stage>]
		// Accepts any AWS region (e.g., us-east-1, eu-west-2, ap-southeast-1) and optional stage/path
		Expect(baseURL).To(MatchRegexp("^https://[a-zA-Z0-9]+\\.execute-api\\.[a-z]+-[a-z]+-[0-9]+\\.amazonaws\\.com(/.*)?$"))
	})

	// it should successfully call the API GET /live endpoint
	// it should access endpoint using the sigv4 authentication protocol
	It("should successfully call the API GET /v0/live endpoint", func() {
		response := getAndExpectOK(apiClient, "/v0/live", accountID, "ok")
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		Expect(string(response.Body)).To(ContainSubstring("ok"))
		Expect(response.Headers).To(HaveKey("Content-Type"))
		Expect(response.Headers).To(HaveKey("X-Amz-Apigw-Id"))
	})

	It("should successfully call the API GET /v0/ready endpoint", func() {
		response := getAndExpectOK(apiClient, "/v0/ready", accountID, "ok")
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		Expect(string(response.Body)).To(ContainSubstring("ok"))
		Expect(response.Headers).To(HaveKey("Content-Type"))
		Expect(response.Headers).To(HaveKey("X-Amz-Apigw-Id"))
	})

	It("should successfully call the API GET /api/v0/ready endpoint", func() {
		response := getAndExpectOK(apiClient, "/api/v0/ready", accountID, "ok")
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		Expect(string(response.Body)).To(ContainSubstring("ok"))
		Expect(response.Headers).To(HaveKey("Content-Type"))
		Expect(response.Headers).To(HaveKey("X-Amz-Apigw-Id"))
	})

	It("should be able to list all the registered management clusters", func() {
		response := getAndExpectOK(apiClient, "/api/v0/management_clusters", accountID, "")
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		// response is ConsumerList: { "kind", "page", "size", "total", "items": [...] }
		var list struct {
			Items []map[string]interface{} `json:"items"`
		}
		err := json.Unmarshal(response.Body, &list)
		Expect(err).To(BeNil())
		Expect(list.Items).ToNot(BeEmpty())
		// display list.Items as a table using standard go library
		for _, item := range list.Items {
			labels, _ := item["labels"].(map[string]interface{})
			clusterType := ""
			if labels != nil {
				clusterType, _ = labels["cluster_type"].(string)
			}
			GinkgoWriter.Printf("management cluster id=%v name=%v cluster_type=%s\n", item["id"], item["name"], clusterType)
		}
		// fmt.Println(tabulate.Fprint(os.Stdout, list.Items))
		Expect(response.Headers).To(HaveKey("Content-Type"))
		Expect(response.Headers).To(HaveKey("X-Amz-Apigw-Id"))
	})

	It("should be able to register a new management cluster", func() {
		// Generate a unique cluster name for this test run
		clusterName := fmt.Sprintf("test-mgmt-%s", time.Now().Format("20060102150405"))
		GinkgoWriter.Printf("Creating management cluster: %s\n", clusterName)

		// create a management cluster with unique name
		managementCluster := map[string]interface{}{
			"name": clusterName,
			"labels": map[string]string{
				"cluster_type": "management",
			},
		}

		response, err := apiClient.Post("/api/v0/management_clusters", managementCluster, accountID)
		Expect(err).To(BeNil())
		Expect(response.StatusCode).To(Equal(http.StatusCreated))
		Expect(response.Headers).To(HaveKey("Content-Type"))
		Expect(response.Headers).To(HaveKey("X-Amz-Apigw-Id"))
		// expect the response body to be a valid json object

		err = json.Unmarshal(response.Body, &managementCluster)
		Expect(err).To(BeNil())
		Expect(managementCluster["kind"]).To(Equal("Consumer"))
		Expect(managementCluster["href"]).ToNot(BeEmpty())
		Expect(managementCluster["name"]).To(Equal(clusterName))
		Expect(managementCluster["labels"].(map[string]interface{})["cluster_type"]).To(Equal("management"))
		Expect(managementCluster["created_at"]).ToNot(BeEmpty())
		Expect(managementCluster["updated_at"]).ToNot(BeEmpty())

		// it should be able to get the management cluster by ID
		response, err = apiClient.Get("/api/v0/management_clusters/"+managementCluster["id"].(string), accountID)
		Expect(err).To(BeNil())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		Expect(response.Headers).To(HaveKey("Content-Type"))
		Expect(response.Headers).To(HaveKey("X-Amz-Apigw-Id"))
		// expect the response body to be a valid json object
		var managementCluster2 map[string]interface{}
		err = json.Unmarshal(response.Body, &managementCluster2)
		Expect(err).To(BeNil())
		Expect(managementCluster2["kind"]).To(Equal("Consumer"))
		Expect(managementCluster2["href"]).ToNot(BeEmpty())
		Expect(managementCluster2["name"]).To(Equal(clusterName))
		Expect(managementCluster2["labels"].(map[string]interface{})["cluster_type"]).To(Equal("management"))
		Expect(managementCluster2["created_at"]).ToNot(BeEmpty())
		Expect(managementCluster2["updated_at"]).ToNot(BeEmpty())

		// DELETE is not defined for management_clusters in the API (OpenAPI has no delete operation for /management_clusters/{id})
	})

	// it should be able to post to the work endpoint
	It("should be able to create new manifestwork on each of the registered management clusters", func() {

		// iterate through the list of management_clusters, get back the management cluster id
		listResp := getAndExpectOK(apiClient, "/api/v0/management_clusters", accountID, "")
		Expect(listResp.StatusCode).To(Equal(http.StatusOK))
		var list struct {
			Items []map[string]interface{} `json:"items"`
		}
		err := json.Unmarshal(listResp.Body, &list)
		Expect(err).To(BeNil())
		Expect(list.Items).ToNot(BeEmpty(), "No management clusters registered - cannot test work creation")
		for _, item := range list.Items {
			managementClusterID := item["id"].(string)
			managementClusterName := item["name"].(string)
			GinkgoWriter.Printf("management cluster id=%s name=%s\n", managementClusterID, managementClusterName)
			work := map[string]interface{}{
				"cluster_id": managementClusterName,
				"data": map[string]interface{}{
					"apiVersion": "work.open-cluster-management.io/v1",
					"kind":       "ManifestWork",
					"metadata": map[string]interface{}{
						"name": "test-work-" + time.Now().Format("20060102150405"),
					},
					"spec": map[string]interface{}{
						"workload": map[string]interface{}{
							"manifests": []map[string]interface{}{
								{
									"apiVersion": "v1",
									"kind":       "ConfigMap",
									"metadata": map[string]interface{}{
										"name":      "test-config-" + time.Now().Format("20060102150405"),
										"namespace": "default",
									},
									"data": map[string]string{
										"key": "value",
									},
								},
							},
						},
					},
				},
			}

			response, err := apiClient.Post("/api/v0/work", work, accountID)
			Expect(err).To(BeNil())
			Expect(response.StatusCode).To(Equal(http.StatusCreated))
			// expect the response body to be a valid json object
			var responseBody map[string]interface{}
			err = json.Unmarshal(response.Body, &responseBody)
			Expect(err).To(BeNil())
			Expect(responseBody["kind"]).To(Equal("ManifestWork"))
			Expect(responseBody["href"]).ToNot(BeEmpty())
			Expect(responseBody["cluster_id"]).To(Equal(managementClusterName))
			Expect(responseBody["name"]).To(ContainSubstring("test-work"))
			Expect(responseBody["status"]).ToNot(BeEmpty())

		}

	})

	// it should be able to get the resource bundles
	It("should be able to list all the resource bundles", func() {
		response := getAndExpectOK(apiClient, "/api/v0/resource_bundles", accountID, "")
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		Expect(response.Headers).To(HaveKey("Content-Type"))
		Expect(response.Headers).To(HaveKey("X-Amz-Apigw-Id"))
		// expect the response body to be a valid json object
		var list struct {
			Items []map[string]interface{} `json:"items"`
		}
		err := json.Unmarshal(response.Body, &list)
		Expect(err).To(BeNil())
		Expect(list.Items).ToNot(BeEmpty())
		// display list.Items as a table using standard go library
		for _, item := range list.Items {
			GinkgoWriter.Printf("%v %v %v %v %v %v %v %v\n", item["id"], item["kind"], item["href"], item["name"], item["consumer_name"], item["version"], item["created_at"], item["updated_at"])
		}
		// GET /api/v0/resource_bundles/{id} is not implemented; only list is available
		// TODO: add support for GET /api/v0/resource_bundles/{id}
	})

	// get the /api/v0/resource_bundles, iterate and find items with status.resourceStatus (and optional statusFeedback / StatusFeedbackSynced)
	// if there are statusfeedback then maestro-server is connected to maestro-client
	It("should be have maestro-server connected to maestro-agent", func() {
		By("having resource bundles records with statusfeedback and statusfeedbackSynced true", func() {
			response := getAndExpectOK(apiClient, "/api/v0/resource_bundles", accountID, "")
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			Expect(response.Headers).To(HaveKey("Content-Type"))
			Expect(response.Headers).To(HaveKey("X-Amz-Apigw-Id"))
			var list struct {
				Items []map[string]interface{} `json:"items"`
			}
			err := json.Unmarshal(response.Body, &list)
			Expect(err).To(BeNil())
			Expect(list.Items).ToNot(BeEmpty())

			// Track whether we found at least one bundle showing connectivity
			foundOrSynced := false

			// Structure: status.resourceStatus[] has statusFeedback and conditions (e.g. type StatusFeedbackSynced, status True)
			for _, item := range list.Items {
				status, _ := item["status"].(map[string]interface{})
				if status == nil {
					continue
				}
				resourceStatusList, _ := status["resourceStatus"].([]interface{})
				for _, rs := range resourceStatusList {
					rsMap, _ := rs.(map[string]interface{})
					if rsMap == nil {
						continue
					}
					statusFeedback, _ := rsMap["statusFeedback"].(map[string]interface{})
					conditions, _ := rsMap["conditions"].([]interface{})
					hasFeedbackSynced := false
					for _, c := range conditions {
						cond, _ := c.(map[string]interface{})
						if cond != nil && cond["type"] == "StatusFeedbackSynced" && cond["status"] == "True" {
							hasFeedbackSynced = true
							break
						}
					}
					if hasFeedbackSynced || (statusFeedback != nil && len(statusFeedback) > 0) {
						foundOrSynced = true
						GinkgoWriter.Printf("resource_bundle id=%v name=%v resourceStatus with statusFeedback / StatusFeedbackSynced: %v\n", item["id"], item["name"], statusFeedback)
					}
				}
			}

			// Fail if no resource bundle shows maestro-server to maestro-agent connectivity
			Expect(foundOrSynced).To(BeTrue(), "No resource bundles found with statusFeedback or StatusFeedbackSynced - maestro-server may not be connected to maestro-agent")
		})
	})

	// it should be able to GET /clusters endpoint, the list should be empty now as this is not yet
	// connected to CLM/hyperfleet
	It("should be have the clusters endpoint defined", func() {
		response := getAndExpectOK(apiClient, "/api/v0/clusters", accountID, "")
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		Expect(response.Headers).To(HaveKey("Content-Type"))
		Expect(response.Headers).To(HaveKey("X-Amz-Apigw-Id"))
		var list struct {
			Items []map[string]interface{} `json:"items"`
		}
		err := json.Unmarshal(response.Body, &list)
		Expect(err).To(BeNil())
		// Items is empty because clusters is not implemented, only defined
		Expect(list.Items).To(BeEmpty(), "clusters list should be empty")
	})
})
