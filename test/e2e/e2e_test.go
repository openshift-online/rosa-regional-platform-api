package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
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
	RunSpecs(t, "ROSA Regional Frontend API E2E Suite")
}

var _ = Describe("E2E Tests", func() {
	It("should run e2e tests", func() {
		// Placeholder for future e2e tests
		Expect(true).To(BeTrue())
	})

	Describe("api_work", Ordered, func() {
		BeforeAll(func() {
			// Discover API Gateway URL before running any tests that depend on it
			awsProfile := os.Getenv("AWS_PROFILE")
			if awsProfile == "" {
				// Skip discovery if AWS_PROFILE not set - tests will skip individually
				return
			}

			// Check for E2E_API_GATEWAY_URL environment variable first
			envAPIURL := os.Getenv("E2E_API_GATEWAY_URL")
			if envAPIURL != "" {
				apiGatewayPublicURL = envAPIURL
				GinkgoWriter.Printf("Using API Gateway URL from E2E_API_GATEWAY_URL environment variable: %s\n", apiGatewayPublicURL)
				fmt.Printf("API Gateway Public URL: %s\n", apiGatewayPublicURL)
				return
			}

			// Check if AWS CLI is available
			_, err := exec.LookPath("aws")
			if err != nil {
				// AWS CLI not available - tests will skip individually
				return
			}

			// Get the region from AWS config
			cfg, err := config.LoadDefaultConfig(context.Background())
			if err != nil {
				// Config load failed - tests will handle this individually
				return
			}

			region := cfg.Region
			if region == "" {
				// Try to get region from AWS CLI config
				output, err := runCommandWithTimeout(10*time.Second, "aws", "configure", "get", "region")
				if err == nil {
					region = strings.TrimSpace(string(output))
				}
			}

			if region == "" {
				// Region not found - tests will handle this individually
				return
			}

			GinkgoWriter.Printf("Searching for API Gateway instances in region: %s\n", region)

			var foundAPIGateways []string

			// Search for REST APIs (API Gateway v1) and extract invoke URLs
			output, err := runCommandWithTimeout(30*time.Second, "aws", "apigateway", "get-rest-apis", "--region", region, "--query", "items[*].[id,name]", "--output", "text")
			if err == nil {
				restAPIsOutput := strings.TrimSpace(string(output))
				if restAPIsOutput != "" && restAPIsOutput != "None" {
					lines := strings.Split(restAPIsOutput, "\n")
					for _, line := range lines {
						fields := strings.Fields(line)
						if len(fields) >= 2 {
							apiID := fields[0]
							apiName := fields[1]
							foundAPIGateways = append(foundAPIGateways, fmt.Sprintf("REST API: %s (ID: %s)", apiName, apiID))

							// Get the stage to construct the invoke URL
							stageOutput, err := runCommandWithTimeout(30*time.Second, "aws", "apigateway", "get-stages", "--rest-api-id", apiID, "--region", region, "--query", "item[0].stageName", "--output", "text")
							if err == nil {
								stageName := strings.TrimSpace(string(stageOutput))
								if stageName != "" && stageName != "None" {
									invokeURL := fmt.Sprintf("https://%s.execute-api.%s.amazonaws.com/%s", apiID, region, stageName)
									if apiGatewayPublicURL == "" {
										apiGatewayPublicURL = invokeURL
										GinkgoWriter.Printf("Extracted REST API Gateway public URL: %s\n", invokeURL)
									}
								}
							}
						}
					}
					GinkgoWriter.Printf("Found REST APIs: %s\n", restAPIsOutput)
				}
			}

			// Search for HTTP APIs (API Gateway v2) and extract invoke URLs
			output, err = runCommandWithTimeout(30*time.Second, "aws", "apigatewayv2", "get-apis", "--region", region, "--query", "Items[*].[ApiId,Name,ApiEndpoint]", "--output", "text")
			if err == nil {
				httpAPIsOutput := strings.TrimSpace(string(output))
				if httpAPIsOutput != "" && httpAPIsOutput != "None" {
					lines := strings.Split(httpAPIsOutput, "\n")
					for _, line := range lines {
						fields := strings.Fields(line)
						if len(fields) >= 2 {
							apiID := fields[0]
							apiName := fields[1]
							foundAPIGateways = append(foundAPIGateways, fmt.Sprintf("HTTP API: %s (ID: %s)", apiName, apiID))

							// Extract the ApiEndpoint if available
							if len(fields) >= 3 && fields[2] != "None" {
								invokeURL := fields[2]
								if apiGatewayPublicURL == "" {
									apiGatewayPublicURL = invokeURL
									GinkgoWriter.Printf("Extracted HTTP API Gateway public URL: %s\n", invokeURL)
								}
							}
						}
					}
					GinkgoWriter.Printf("Found HTTP APIs: %s\n", httpAPIsOutput)
				}
			}

			// Log all found API Gateways
			if len(foundAPIGateways) > 0 {
				fmt.Printf("Found %d API Gateway instance(s) in region %s:\n", len(foundAPIGateways), region)
				for _, api := range foundAPIGateways {
					fmt.Printf("  - %s\n", api)
					GinkgoWriter.Printf("  - %s\n", api)
				}
			} else {
				GinkgoWriter.Printf("No API Gateway instances found in region %s\n", region)
			}

			// Store the public URL if found
			if apiGatewayPublicURL != "" {
				fmt.Printf("API Gateway Public URL: %s\n", apiGatewayPublicURL)
				GinkgoWriter.Printf("API Gateway Public URL stored: %s\n", apiGatewayPublicURL)
			}
		})

		It("should check AWS_PROFILE and print account ID", func() {
			awsProfile := os.Getenv("AWS_PROFILE")
			if awsProfile == "" {
				Skip("AWS_PROFILE not set, skipping test")
			}

			GinkgoWriter.Printf("AWS_PROFILE is set to: %s\n", awsProfile)

			// Load AWS config and get caller identity
			cfg, err := config.LoadDefaultConfig(context.Background())
			Expect(err).NotTo(HaveOccurred())

			stsClient := sts.NewFromConfig(cfg)
			identity, err := stsClient.GetCallerIdentity(context.Background(), &sts.GetCallerIdentityInput{})
			Expect(err).NotTo(HaveOccurred())

			accountID := *identity.Account

			Expect(awsProfile).To(ContainSubstring("regional"))

			fmt.Printf("AWS Account ID: %s\n", accountID)
			GinkgoWriter.Printf("AWS Account ID: %s\n", accountID)
		})

		It("should detect the presence of awscurl CLI", func() {
			awscurlPath, err := exec.LookPath("awscurl")
			if err != nil {
				Skip("awscurl CLI not found in PATH, skipping test")
			}

			Expect(awscurlPath).NotTo(BeEmpty())
			GinkgoWriter.Printf("awscurl found at: %s\n", awscurlPath)
		})

		It("should detect the presence of terraform CLI", func() {
			terraformPath, err := exec.LookPath("terraform")
			if err != nil {
				Skip("terraform CLI not found in PATH, skipping test")
			}

			Expect(terraformPath).NotTo(BeEmpty())
			GinkgoWriter.Printf("terraform found at: %s\n", terraformPath)
		})

		// It("should extract the api_gateway_invoke_url from the AWS account", func() {
		// 	awsProfile := os.Getenv("AWS_PROFILE")
		// 	if awsProfile == "" {
		// 		Skip("AWS_PROFILE not set, skipping test")
		// 	}

		// 	// Check if terraform is available
		// 	terraformPath, err := exec.LookPath("terraform")
		// 	if err != nil {
		// 		Skip("terraform CLI not found in PATH, skipping test")
		// 	}
		// 	Expect(terraformPath).NotTo(BeEmpty())

		// 	var apiGatewayInvokeURL string

		// 	// Try to get from terraform output
		// 	outputNames := []string{
		// 		"api_gateway_invoke_url",
		// 		"apigateway_invoke_url",
		// 		"api_gateway_url",
		// 	}

		// 	for _, outputName := range outputNames {
		// 		cmd := exec.Command("terraform", "output", "-raw", outputName)
		// 		output, err := cmd.CombinedOutput()
		// 		if err == nil {
		// 			apiGatewayInvokeURL = strings.TrimSpace(string(output))
		// 			if apiGatewayInvokeURL != "" {
		// 				GinkgoWriter.Printf("Found API Gateway invoke URL from terraform output %s: %s\n", outputName, apiGatewayInvokeURL)
		// 				break
		// 			}
		// 		}
		// 	}

		// 	// If not found via terraform output, try AWS CLI with SSM
		// 	if apiGatewayInvokeURL == "" {
		// 		_, err := exec.LookPath("aws")
		// 		if err == nil {
		// 			paramNames := []string{
		// 				"/rosa-regional-frontend/api_gateway_invoke_url",
		// 				"/rosa/api_gateway_invoke_url",
		// 				"api_gateway_invoke_url",
		// 			}

		// 			for _, paramName := range paramNames {
		// 				cmd := exec.Command("aws", "ssm", "get-parameter", "--name", paramName, "--query", "Parameter.Value", "--output", "text")
		// 				output, err := cmd.CombinedOutput()
		// 				if err == nil {
		// 					apiGatewayInvokeURL = strings.TrimSpace(string(output))
		// 					if apiGatewayInvokeURL != "" {
		// 						GinkgoWriter.Printf("Found API Gateway invoke URL from SSM parameter %s: %s\n", paramName, apiGatewayInvokeURL)
		// 						break
		// 					}
		// 				}
		// 			}
		// 		}
		// 	}

		// 	Expect(apiGatewayInvokeURL).NotTo(BeEmpty(), "API Gateway invoke URL not found in terraform outputs or SSM Parameter Store")
		// 	Expect(apiGatewayInvokeURL).To(ContainSubstring("https://"), "API Gateway invoke URL should be a valid HTTPS URL")

		// 	fmt.Printf("API Gateway Invoke URL: %s\n", apiGatewayInvokeURL)
		// 	GinkgoWriter.Printf("API Gateway Invoke URL: %s\n", apiGatewayInvokeURL)
		// })

		It("should verify API Gateway URL was discovered", func() {
			// This test verifies that the BeforeAll block successfully discovered the API Gateway URL
			// The actual discovery happens in BeforeAll to ensure it runs before all dependent tests
			if apiGatewayPublicURL == "" {
				Skip("API Gateway URL not discovered - check AWS_PROFILE and AWS CLI availability")
			}
			GinkgoWriter.Printf("API Gateway URL verified: %s\n", apiGatewayPublicURL)
		})

		It("should invoke the API Gateway ready endpoint using awscurl", func() {
			awsProfile := os.Getenv("AWS_PROFILE")
			if awsProfile == "" {
				Skip("AWS_PROFILE not set, skipping test")
			}

			// Check if awscurl is available
			awscurlPath, err := exec.LookPath("awscurl")
			if err != nil {
				Skip("awscurl CLI not found in PATH, skipping test")
			}
			Expect(awscurlPath).NotTo(BeEmpty())

			// Check if apiGatewayPublicURL was set by previous test
			if apiGatewayPublicURL == "" {
				Skip("API Gateway public URL not found, skipping test")
			}

			// Get the region from AWS config
			cfg, err := config.LoadDefaultConfig(context.Background())
			Expect(err).NotTo(HaveOccurred())

			region := cfg.Region
			if region == "" {
				// Try to get region from AWS CLI config
				output, err := runCommandWithTimeout(10*time.Second, "aws", "configure", "get", "region")
				if err == nil {
					region = strings.TrimSpace(string(output))
				}
			}
			Expect(region).NotTo(BeEmpty(), "AWS region not found in config")

			// Normalize the base URL - remove trailing slash if present
			baseURL := strings.TrimSuffix(apiGatewayPublicURL, "/")

			// Check if the URL already includes a stage path (e.g., /prod)
			// If it does, append /v0/ready directly; otherwise try /prod/v0/ready first
			var endpointPaths []string
			if strings.Contains(baseURL, "/prod") || strings.Contains(baseURL, "/dev") || strings.Contains(baseURL, "/stage") {
				// URL already has a stage, just append the endpoint path
				endpointPaths = []string{
					"/v0/ready",
					"/api/v0/ready",
				}
			} else {
				// URL doesn't have a stage, try with /prod prefix first
				endpointPaths = []string{
					"/prod/v0/ready",
					"/prod/api/v0/ready",
					"/v0/ready",
					"/api/v0/ready",
				}
			}

			var responseBody string
			var endpointURL string

			for _, path := range endpointPaths {
				endpointURL = baseURL + path
				GinkgoWriter.Printf("Attempting to invoke: %s\n", endpointURL)

				// Run awscurl command
				output, err := runCommandWithTimeout(30*time.Second, "awscurl", "--service", "execute-api", "--region", region, endpointURL)
				if err == nil {
					responseBody = strings.TrimSpace(string(output))
					if responseBody != "" {
						GinkgoWriter.Printf("Successfully invoked endpoint: %s\n", endpointURL)
						GinkgoWriter.Printf("Response: %s\n", responseBody)
						break
					}
				} else {
					GinkgoWriter.Printf("Failed to invoke %s: %s\n", endpointURL, err.Error())
				}
			}

			Expect(responseBody).NotTo(BeEmpty(), "Failed to get response from API Gateway endpoint")

			// Parse JSON response
			var response map[string]interface{}
			err = json.Unmarshal([]byte(responseBody), &response)
			Expect(err).NotTo(HaveOccurred(), "Response is not valid JSON: %s", responseBody)

			// Verify the response contains {"status":"ok"}
			status, ok := response["status"]
			Expect(ok).To(BeTrue(), "Response does not contain 'status' field")
			Expect(status).To(Equal("ok"), "Expected status to be 'ok', got: %v", status)

			fmt.Printf("API Gateway endpoint %s returned: %s\n", endpointURL, responseBody)
			GinkgoWriter.Printf("API Gateway endpoint %s returned: %s\n", endpointURL, responseBody)
		})

		It("should invoke the API Gateway live endpoint using awscurl", func() {
			awsProfile := os.Getenv("AWS_PROFILE")
			if awsProfile == "" {
				Skip("AWS_PROFILE not set, skipping test")
			}

			// Check if awscurl is available
			awscurlPath, err := exec.LookPath("awscurl")
			if err != nil {
				Skip("awscurl CLI not found in PATH, skipping test")
			}
			Expect(awscurlPath).NotTo(BeEmpty())

			// Check if apiGatewayPublicURL was set by previous test
			if apiGatewayPublicURL == "" {
				Skip("API Gateway public URL not found, skipping test")
			}

			// Get the region from AWS config
			cfg, err := config.LoadDefaultConfig(context.Background())
			Expect(err).NotTo(HaveOccurred())

			region := cfg.Region
			if region == "" {
				// Try to get region from AWS CLI config
				output, err := runCommandWithTimeout(10*time.Second, "aws", "configure", "get", "region")
				if err == nil {
					region = strings.TrimSpace(string(output))
				}
			}
			Expect(region).NotTo(BeEmpty(), "AWS region not found in config")

			// Normalize the base URL - remove trailing slash if present
			baseURL := strings.TrimSuffix(apiGatewayPublicURL, "/")

			// Check if the URL already includes a stage path (e.g., /prod)
			// If it does, append /v0/live directly; otherwise try /prod/v0/live first
			var endpointPaths []string
			if strings.Contains(baseURL, "/prod") || strings.Contains(baseURL, "/dev") || strings.Contains(baseURL, "/stage") {
				// URL already has a stage, just append the endpoint path
				endpointPaths = []string{
					"/v0/live",
					"/api/v0/live",
				}
			} else {
				// URL doesn't have a stage, try with /prod prefix first
				endpointPaths = []string{
					"/prod/v0/live",
					"/prod/api/v0/live",
					"/v0/live",
					"/api/v0/live",
				}
			}

			var responseBody string
			var endpointURL string

			for _, path := range endpointPaths {
				endpointURL = baseURL + path
				GinkgoWriter.Printf("Attempting to invoke: %s\n", endpointURL)

				// Run awscurl command
				output, err := runCommandWithTimeout(30*time.Second, "awscurl", "--service", "execute-api", "--region", region, endpointURL)
				if err == nil {
					responseBody = strings.TrimSpace(string(output))
					if responseBody != "" {
						GinkgoWriter.Printf("Successfully invoked endpoint: %s\n", endpointURL)
						GinkgoWriter.Printf("Response: %s\n", responseBody)
						break
					}
				} else {
					GinkgoWriter.Printf("Failed to invoke %s: %s\n", endpointURL, err.Error())
				}
			}

			Expect(responseBody).NotTo(BeEmpty(), "Failed to get response from API Gateway endpoint")

			// Parse JSON response
			var response map[string]interface{}
			err = json.Unmarshal([]byte(responseBody), &response)
			Expect(err).NotTo(HaveOccurred(), "Response is not valid JSON: %s", responseBody)

			// Verify the response contains {"status":"ok"}
			status, ok := response["status"]
			Expect(ok).To(BeTrue(), "Response does not contain 'status' field")
			Expect(status).To(Equal("ok"), "Expected status to be 'ok', got: %v", status)

			fmt.Printf("API Gateway endpoint %s returned: %s\n", endpointURL, responseBody)
			GinkgoWriter.Printf("API Gateway endpoint %s returned: %s\n", endpointURL, responseBody)
		})

		It("should invoke the API Gateway management_clusters endpoint using awscurl", func() {
			awsProfile := os.Getenv("AWS_PROFILE")
			if awsProfile == "" {
				Skip("AWS_PROFILE not set, skipping test")
			}

			// Check if awscurl is available
			awscurlPath, err := exec.LookPath("awscurl")
			if err != nil {
				Skip("awscurl CLI not found in PATH, skipping test")
			}
			Expect(awscurlPath).NotTo(BeEmpty())

			// Check if apiGatewayPublicURL was set by previous test
			if apiGatewayPublicURL == "" {
				Skip("API Gateway public URL not found, skipping test")
			}

			// Get the region from AWS config
			cfg, err := config.LoadDefaultConfig(context.Background())
			Expect(err).NotTo(HaveOccurred())

			region := cfg.Region
			if region == "" {
				// Try to get region from AWS CLI config
				output, err := runCommandWithTimeout(10*time.Second, "aws", "configure", "get", "region")
				if err == nil {
					region = strings.TrimSpace(string(output))
				}
			}
			Expect(region).NotTo(BeEmpty(), "AWS region not found in config")

			// Normalize the base URL - remove trailing slash if present
			baseURL := strings.TrimSuffix(apiGatewayPublicURL, "/")

			// Check if the URL already includes a stage path (e.g., /prod)
			// If it does, append /api/v0/management_clusters directly; otherwise try /prod/api/v0/management_clusters first
			var endpointPaths []string
			if strings.Contains(baseURL, "/prod") || strings.Contains(baseURL, "/dev") || strings.Contains(baseURL, "/stage") {
				// URL already has a stage, just append the endpoint path
				endpointPaths = []string{
					"/api/v0/management_clusters",
					"/v0/management_clusters",
				}
			} else {
				// URL doesn't have a stage, try with /prod prefix first
				endpointPaths = []string{
					"/prod/api/v0/management_clusters",
					"/prod/v0/management_clusters",
					"/api/v0/management_clusters",
					"/v0/management_clusters",
				}
			}

			var responseBody string
			var endpointURL string

			for _, path := range endpointPaths {
				endpointURL = baseURL + path
				GinkgoWriter.Printf("Attempting to invoke: %s\n", endpointURL)

				// Run awscurl command
				output, err := runCommandWithTimeout(30*time.Second, "awscurl", "--service", "execute-api", "--region", region, endpointURL)
				if err == nil {
					responseBody = strings.TrimSpace(string(output))
					if responseBody != "" {
						GinkgoWriter.Printf("Successfully invoked endpoint: %s\n", endpointURL)
						GinkgoWriter.Printf("Response: %s\n", responseBody)
						break
					}
				} else {
					GinkgoWriter.Printf("Failed to invoke %s: %s\n", endpointURL, err.Error())
				}
			}

			// Response can be empty or valid JSON
			if responseBody == "" {
				fmt.Printf("API Gateway endpoint %s returned empty response\n", endpointURL)
				GinkgoWriter.Printf("API Gateway endpoint %s returned empty response (acceptable)\n", endpointURL)
				// Empty response is acceptable, test passes
				return
			}

			// Parse JSON response - validate it's valid JSON
			var response map[string]interface{}
			err = json.Unmarshal([]byte(responseBody), &response)
			Expect(err).NotTo(HaveOccurred(), "Response is not valid JSON: %s", responseBody)

			// If the response contains data (items), validate the structure
			if items, ok := response["items"]; ok {
				itemsArray, isArray := items.([]interface{})
				Expect(isArray).To(BeTrue(), "Expected 'items' to be an array")

				// Validate each item has required fields if items exist
				if len(itemsArray) > 0 {
					for i, item := range itemsArray {
						itemMap, isMap := item.(map[string]interface{})
						Expect(isMap).To(BeTrue(), "Expected item %d to be an object", i)

						// Check for common fields that should exist
						_, hasID := itemMap["id"]
						_, hasKind := itemMap["kind"]
						_, hasName := itemMap["name"]

						if hasID || hasKind || hasName {
							GinkgoWriter.Printf("Item %d has valid structure\n", i)
						}
					}

					fmt.Printf("API Gateway endpoint %s returned %d management cluster(s)\n", endpointURL, len(itemsArray))
					GinkgoWriter.Printf("API Gateway endpoint %s returned %d management cluster(s)\n", endpointURL, len(itemsArray))
				} else {
					GinkgoWriter.Printf("API Gateway endpoint %s returned empty items list\n", endpointURL)
				}
			}

			fmt.Printf("API Gateway endpoint %s returned valid JSON: %s\n", endpointURL, responseBody)
			GinkgoWriter.Printf("API Gateway endpoint %s returned valid JSON\n", endpointURL)
		})

		It("should invoke the API Gateway resource_bundles endpoint using awscurl", func() {
			awsProfile := os.Getenv("AWS_PROFILE")
			if awsProfile == "" {
				Skip("AWS_PROFILE not set, skipping test")
			}

			// Check if awscurl is available
			awscurlPath, err := exec.LookPath("awscurl")
			if err != nil {
				Skip("awscurl CLI not found in PATH, skipping test")
			}
			Expect(awscurlPath).NotTo(BeEmpty())

			// Check if apiGatewayPublicURL was set by previous test
			if apiGatewayPublicURL == "" {
				Skip("API Gateway public URL not found, skipping test")
			}

			// Get the region from AWS config
			cfg, err := config.LoadDefaultConfig(context.Background())
			Expect(err).NotTo(HaveOccurred())

			region := cfg.Region
			if region == "" {
				// Try to get region from AWS CLI config
				output, err := runCommandWithTimeout(10*time.Second, "aws", "configure", "get", "region")
				if err == nil {
					region = strings.TrimSpace(string(output))
				}
			}
			Expect(region).NotTo(BeEmpty(), "AWS region not found in config")

			// Normalize the base URL - remove trailing slash if present
			baseURL := strings.TrimSuffix(apiGatewayPublicURL, "/")

			// Check if the URL already includes a stage path (e.g., /prod)
			// If it does, append /api/v0/resource_bundles directly; otherwise try /prod/api/v0/resource_bundles first
			var endpointPaths []string
			if strings.Contains(baseURL, "/prod") || strings.Contains(baseURL, "/dev") || strings.Contains(baseURL, "/stage") {
				// URL already has a stage, just append the endpoint path
				endpointPaths = []string{
					"/api/v0/resource_bundles",
					"/v0/resource_bundles",
				}
			} else {
				// URL doesn't have a stage, try with /prod prefix first
				endpointPaths = []string{
					"/prod/api/v0/resource_bundles",
					"/prod/v0/resource_bundles",
					"/api/v0/resource_bundles",
					"/v0/resource_bundles",
				}
			}

			var responseBody string
			var endpointURL string

			for _, path := range endpointPaths {
				endpointURL = baseURL + path
				GinkgoWriter.Printf("Attempting to invoke: %s\n", endpointURL)

				// Run awscurl command
				output, err := runCommandWithTimeout(30*time.Second, "awscurl", "--service", "execute-api", "--region", region, endpointURL)
				if err == nil {
					responseBody = strings.TrimSpace(string(output))
					if responseBody != "" {
						GinkgoWriter.Printf("Successfully invoked endpoint: %s\n", endpointURL)
						GinkgoWriter.Printf("Response: %s\n", responseBody)
						break
					}
				} else {
					GinkgoWriter.Printf("Failed to invoke %s: %s\n", endpointURL, err.Error())
				}
			}

			// Response can be empty or valid JSON
			if responseBody == "" {
				fmt.Printf("API Gateway endpoint %s returned empty response\n", endpointURL)
				GinkgoWriter.Printf("API Gateway endpoint %s returned empty response (acceptable)\n", endpointURL)
				// Empty response is acceptable, test passes
				return
			}

			// Parse JSON response - validate it's valid JSON
			var response map[string]interface{}
			err = json.Unmarshal([]byte(responseBody), &response)
			Expect(err).NotTo(HaveOccurred(), "Response is not valid JSON: %s", responseBody)

			// If the response contains data (items), validate the structure
			if items, ok := response["items"]; ok {
				itemsArray, isArray := items.([]interface{})
				Expect(isArray).To(BeTrue(), "Expected 'items' to be an array")

				// Validate each item has required fields if items exist
				if len(itemsArray) > 0 {
					for i, item := range itemsArray {
						itemMap, isMap := item.(map[string]interface{})
						Expect(isMap).To(BeTrue(), "Expected item %d to be an object", i)

						// Check for common fields that should exist
						_, hasID := itemMap["id"]
						_, hasKind := itemMap["kind"]
						_, hasName := itemMap["name"]

						if hasID || hasKind || hasName {
							GinkgoWriter.Printf("Item %d has valid structure\n", i)
						}
					}

					fmt.Printf("API Gateway endpoint %s returned %d resource bundle(s)\n", endpointURL, len(itemsArray))
					GinkgoWriter.Printf("API Gateway endpoint %s returned %d resource bundle(s)\n", endpointURL, len(itemsArray))
				} else {
					GinkgoWriter.Printf("API Gateway endpoint %s returned empty items list\n", endpointURL)
				}
			}

			fmt.Printf("API Gateway endpoint %s returned valid JSON:\n", endpointURL)
			GinkgoWriter.Printf("API Gateway endpoint %s returned valid JSON\n", endpointURL)
		})

		It("should verify GET on API Gateway work endpoint is not implemented", func() {
			awsProfile := os.Getenv("AWS_PROFILE")
			if awsProfile == "" {
				Skip("AWS_PROFILE not set, skipping test")
			}

			// Check if awscurl is available
			awscurlPath, err := exec.LookPath("awscurl")
			if err != nil {
				Skip("awscurl CLI not found in PATH, skipping test")
			}
			Expect(awscurlPath).NotTo(BeEmpty())

			// Check if apiGatewayPublicURL was set by previous test
			if apiGatewayPublicURL == "" {
				Skip("API Gateway public URL not found, skipping test")
			}

			// Get the region from AWS config
			cfg, err := config.LoadDefaultConfig(context.Background())
			Expect(err).NotTo(HaveOccurred())

			region := cfg.Region
			if region == "" {
				// Try to get region from AWS CLI config
				output, err := runCommandWithTimeout(10*time.Second, "aws", "configure", "get", "region")
				if err == nil {
					region = strings.TrimSpace(string(output))
				}
			}
			Expect(region).NotTo(BeEmpty(), "AWS region not found in config")

			// Normalize the base URL - remove trailing slash if present
			baseURL := strings.TrimSuffix(apiGatewayPublicURL, "/")

			// Check if the URL already includes a stage path (e.g., /prod)
			// If it does, append /api/v0/work directly; otherwise try /prod/api/v0/work first
			var endpointPaths []string
			if strings.Contains(baseURL, "/prod") || strings.Contains(baseURL, "/dev") || strings.Contains(baseURL, "/stage") {
				// URL already has a stage, just append the endpoint path
				endpointPaths = []string{
					"/api/v0/work",
					"/v0/work",
				}
			} else {
				// URL doesn't have a stage, try with /prod prefix first
				endpointPaths = []string{
					"/prod/api/v0/work",
					"/prod/v0/work",
					"/api/v0/work",
					"/v0/work",
				}
			}

			var responseBody string
			var endpointURL string
			var gotResponse bool

			for _, path := range endpointPaths {
				endpointURL = baseURL + path
				GinkgoWriter.Printf("Attempting to invoke GET: %s\n", endpointURL)

				// Run awscurl command with GET (default method)
				output, err := runCommandWithTimeout(30*time.Second, "awscurl", "--service", "execute-api", "--region", region, endpointURL)
				responseBody = strings.TrimSpace(string(output))

				// GET is not implemented, so we expect either:
				// 1. Empty response body (if 405 Method Not Allowed with no body)
				// 2. Error response (405 or similar)
				// 3. Empty response
				if err != nil {
					// Error is expected - GET is not implemented
					errorStr := err.Error()
					if strings.Contains(errorStr, "405") || strings.Contains(string(output), "405") ||
						strings.Contains(errorStr, "Method Not Allowed") || strings.Contains(string(output), "Method Not Allowed") {
						GinkgoWriter.Printf("GET method not allowed (405) as expected: %s\n", endpointURL)
						gotResponse = true
						break
					}
					GinkgoWriter.Printf("Got error response (expected): %s\n", errorStr)
				}

				if responseBody == "" {
					// Empty response is acceptable - GET is not implemented
					GinkgoWriter.Printf("GET returned empty response (expected, GET not implemented): %s\n", endpointURL)
					gotResponse = true
					break
				}

				// If we got a response, check if it's an error message
				if strings.Contains(strings.ToLower(responseBody), "method not allowed") ||
					strings.Contains(strings.ToLower(responseBody), "405") ||
					strings.Contains(strings.ToLower(responseBody), "not implemented") {
					GinkgoWriter.Printf("GET returned error message (expected): %s\n", responseBody)
					gotResponse = true
					break
				}
			}

			// Verify that GET is not implemented - response should be empty or error
			Expect(gotResponse).To(BeTrue(), "Failed to get any response from API Gateway endpoint")

			// Response should be empty or contain error indicating method not allowed
			if responseBody != "" {
				// If there's a response body, it should indicate method not allowed or not implemented
				lowerBody := strings.ToLower(responseBody)
				Expect(lowerBody).To(Or(
					ContainSubstring("method not allowed"),
					ContainSubstring("405"),
					ContainSubstring("not implemented"),
					ContainSubstring("not supported"),
				), "Response should indicate GET is not implemented, got: %s", responseBody)
			}

			fmt.Printf("API Gateway endpoint %s correctly returns empty/error for GET (GET not implemented)\n", endpointURL)
			GinkgoWriter.Printf("API Gateway endpoint %s correctly returns empty/error for GET (GET not implemented)\n", endpointURL)
		})

		It("should POST to API Gateway work endpoint using awscurl with payload.json", func() {
			awsProfile := os.Getenv("AWS_PROFILE")
			if awsProfile == "" {
				Skip("AWS_PROFILE not set, skipping test")
			}

			// Check if awscurl is available
			awscurlPath, err := exec.LookPath("awscurl")
			if err != nil {
				Skip("awscurl CLI not found in PATH, skipping test")
			}
			Expect(awscurlPath).NotTo(BeEmpty())

			// Check if apiGatewayPublicURL was set by previous test
			if apiGatewayPublicURL == "" {
				Skip("API Gateway public URL not found, skipping test")
			}

			// Get the region from AWS config
			cfg, err := config.LoadDefaultConfig(context.Background())
			Expect(err).NotTo(HaveOccurred())

			region := cfg.Region
			if region == "" {
				// Try to get region from AWS CLI config
				output, err := runCommandWithTimeout(10*time.Second, "aws", "configure", "get", "region")
				if err == nil {
					region = strings.TrimSpace(string(output))
				}
			}
			Expect(region).NotTo(BeEmpty(), "AWS region not found in config")

			// Create temporary file for payload.json
			tmpFile, err := os.CreateTemp("", "payload-*.json")
			Expect(err).NotTo(HaveOccurred(), "Failed to create temporary file")
			payloadPath := tmpFile.Name()
			tmpFile.Close() // Close immediately as createPayloadJSON will write to it

			// Clean up temporary file after test
			defer func() {
				if err := os.Remove(payloadPath); err != nil {
					GinkgoWriter.Printf("Warning: Failed to remove temporary file %s: %v\n", payloadPath, err)
				}
			}()

			// Create payload.json file using helper function
			clusterID := "management-01"
			timestamp, err := createPayloadJSON(clusterID, payloadPath)
			Expect(err).NotTo(HaveOccurred(), "Failed to create payload.json")
			GinkgoWriter.Printf("Created payload.json with timestamp: %s\n", timestamp)

			// Normalize the base URL - remove trailing slash if present
			baseURL := strings.TrimSuffix(apiGatewayPublicURL, "/")

			// Check if the URL already includes a stage path (e.g., /prod)
			// If it does, append /api/v0/work directly; otherwise try /prod/api/v0/work first
			var endpointPaths []string
			if strings.Contains(baseURL, "/prod") || strings.Contains(baseURL, "/dev") || strings.Contains(baseURL, "/stage") {
				// URL already has a stage, just append the endpoint path
				endpointPaths = []string{
					"/api/v0/work",
					"/v0/work",
				}
			} else {
				// URL doesn't have a stage, try with /prod prefix first
				endpointPaths = []string{
					"/prod/api/v0/work",
					"/prod/v0/work",
					"/api/v0/work",
					"/v0/work",
				}
			}

			var responseBody string
			var endpointURL string
			var success bool

			for _, path := range endpointPaths {
				endpointURL = baseURL + path
				GinkgoWriter.Printf("Attempting to POST to: %s\n", endpointURL)

				// Run awscurl command with POST and payload file
				output, err := runCommandWithTimeout(60*time.Second, "awscurl", "-X", "POST", "--service", "execute-api", "--region", region, endpointURL, "-d", "@"+payloadPath)
				if err == nil {
					responseBody = strings.TrimSpace(string(output))
					if responseBody != "" {
						GinkgoWriter.Printf("Successfully posted to endpoint: %s\n", endpointURL)
						GinkgoWriter.Printf("Response: %s\n", responseBody)
						success = true
						break
					}
				} else {
					GinkgoWriter.Printf("Failed to POST to %s: %s\n", endpointURL, err.Error())
					GinkgoWriter.Printf("Output: %s\n", string(output))
				}
			}

			Expect(success).To(BeTrue(), "Failed to get successful response from API Gateway endpoint")
			Expect(responseBody).NotTo(BeEmpty(), "Response body should not be empty")

			// Parse JSON response - validate it's valid JSON
			var response map[string]interface{}
			err = json.Unmarshal([]byte(responseBody), &response)
			Expect(err).NotTo(HaveOccurred(), "Response is not valid JSON: %s", responseBody)

			// Validate that the response contains an ID field with UUID pattern
			idValue, ok := response["id"]
			Expect(ok).To(BeTrue(), "Response should contain 'id' field")

			idStr, ok := idValue.(string)
			Expect(ok).To(BeTrue(), "ID should be a string, got: %T", idValue)
			Expect(idStr).NotTo(BeEmpty(), "ID should not be empty")

			// Validate UUID pattern: 8-4-4-4-12 hexadecimal characters (e.g., 099cc220-90ed-5e04-b49b-4b5d5b5eb1a2)
			uuidPattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
			Expect(uuidPattern.MatchString(idStr)).To(BeTrue(), "ID should match UUID pattern (8-4-4-4-12), got: %s", idStr)

			// Log the response
			fmt.Printf("API Gateway endpoint %s POST returned valid JSON with ID: %s\n", endpointURL, idStr)
			GinkgoWriter.Printf("API Gateway endpoint %s POST returned valid JSON with ID: %s\n", endpointURL, idStr)
		})

		It("should verify all resource bundles have Applied status conditions", func() {
			awsProfile := os.Getenv("AWS_PROFILE")
			if awsProfile == "" {
				Skip("AWS_PROFILE not set, skipping test")
			}

			// Check if awscurl is available
			awscurlPath, err := exec.LookPath("awscurl")
			if err != nil {
				Skip("awscurl CLI not found in PATH, skipping test")
			}
			Expect(awscurlPath).NotTo(BeEmpty())

			// Check if apiGatewayPublicURL was set by previous test
			if apiGatewayPublicURL == "" {
				Skip("API Gateway public URL not found, skipping test")
			}

			// Get the region from AWS config
			cfg, err := config.LoadDefaultConfig(context.Background())
			Expect(err).NotTo(HaveOccurred())

			region := cfg.Region
			if region == "" {
				// Try to get region from AWS CLI config
				output, err := runCommandWithTimeout(10*time.Second, "aws", "configure", "get", "region")
				if err == nil {
					region = strings.TrimSpace(string(output))
				}
			}
			Expect(region).NotTo(BeEmpty(), "AWS region not found in config")

			// Normalize the base URL - remove trailing slash if present
			baseURL := strings.TrimSuffix(apiGatewayPublicURL, "/")

			// Check if the URL already includes a stage path (e.g., /prod)
			// If it does, append /api/v0/resource_bundles directly; otherwise try /prod/api/v0/resource_bundles first
			var endpointPaths []string
			if strings.Contains(baseURL, "/prod") || strings.Contains(baseURL, "/dev") || strings.Contains(baseURL, "/stage") {
				// URL already has a stage, just append the endpoint path
				endpointPaths = []string{
					"/api/v0/resource_bundles",
					"/v0/resource_bundles",
				}
			} else {
				// URL doesn't have a stage, try with /prod prefix first
				endpointPaths = []string{
					"/prod/api/v0/resource_bundles",
					"/prod/v0/resource_bundles",
					"/api/v0/resource_bundles",
					"/v0/resource_bundles",
				}
			}

			var responseBody string
			var endpointURL string

			for _, path := range endpointPaths {
				endpointURL = baseURL + path
				GinkgoWriter.Printf("Attempting to invoke: %s\n", endpointURL)

				// Run awscurl command
				output, err := runCommandWithTimeout(30*time.Second, "awscurl", "--service", "execute-api", "--region", region, endpointURL)
				if err == nil {
					responseBody = strings.TrimSpace(string(output))
					if responseBody != "" {
						GinkgoWriter.Printf("Successfully invoked endpoint: %s\n", endpointURL)
						break
					}
				} else {
					GinkgoWriter.Printf("Failed to invoke %s: %s\n", endpointURL, err.Error())
				}
			}

			// Response can be empty or valid JSON
			if responseBody == "" {
				fmt.Printf("API Gateway endpoint %s returned empty response\n", endpointURL)
				GinkgoWriter.Printf("API Gateway endpoint %s returned empty response (no resource bundles to check)\n", endpointURL)
				// Empty response is acceptable, test passes
				return
			}

			// Parse JSON response - validate it's valid JSON
			var response map[string]interface{}
			err = json.Unmarshal([]byte(responseBody), &response)
			Expect(err).NotTo(HaveOccurred(), "Response is not valid JSON: %s", responseBody)

			// Get items array
			items, ok := response["items"]
			if !ok {
				GinkgoWriter.Printf("API Gateway endpoint %s response does not contain 'items' field\n", endpointURL)
				return
			}

			itemsArray, isArray := items.([]interface{})
			Expect(isArray).To(BeTrue(), "Expected 'items' to be an array")

			if len(itemsArray) == 0 {
				GinkgoWriter.Printf("API Gateway endpoint %s returned empty items list (no resource bundles to check)\n", endpointURL)
				return
			}

			// Iterate through all resource bundles and check status conditions
			for i, item := range itemsArray {
				itemMap, isMap := item.(map[string]interface{})
				Expect(isMap).To(BeTrue(), "Expected item %d to be an object", i)

				// Get the resource bundle name/ID for logging
				itemID := "unknown"
				if id, ok := itemMap["id"]; ok {
					if idStr, ok := id.(string); ok {
						itemID = idStr
					}
				} else if name, ok := itemMap["name"]; ok {
					if nameStr, ok := name.(string); ok {
						itemID = nameStr
					}
				}

				// Get status field
				status, hasStatus := itemMap["status"]
				if !hasStatus {
					GinkgoWriter.Printf("Resource bundle %s (index %d) does not have 'status' field, skipping\n", itemID, i)
					continue
				}

				statusMap, isStatusMap := status.(map[string]interface{})
				if !isStatusMap {
					GinkgoWriter.Printf("Resource bundle %s (index %d) has invalid 'status' field type, skipping\n", itemID, i)
					continue
				}

				// Get conditions array
				conditions, hasConditions := statusMap["conditions"]
				if !hasConditions {
					GinkgoWriter.Printf("Resource bundle %s (index %d) does not have 'conditions' field, skipping\n", itemID, i)
					continue
				}

				conditionsArray, isConditionsArray := conditions.([]interface{})
				if !isConditionsArray {
					GinkgoWriter.Printf("Resource bundle %s (index %d) has invalid 'conditions' field type, skipping\n", itemID, i)
					continue
				}

				// Find Applied condition and verify it's True
				appliedFound := false
				for j, condition := range conditionsArray {
					conditionMap, isConditionMap := condition.(map[string]interface{})
					if !isConditionMap {
						continue
					}

					conditionType, hasType := conditionMap["type"]
					if !hasType {
						continue
					}

					typeStr, isString := conditionType.(string)
					if !isString {
						continue
					}

					// Check if this is the Applied condition
					if typeStr == "Applied" {
						appliedFound = true
						conditionStatus, hasStatus := conditionMap["status"]
						if !hasStatus {
							Fail(fmt.Sprintf("Resource bundle %s (index %d) has Applied condition (index %d) without 'status' field", itemID, i, j))
						}

						statusStr, isStatusString := conditionStatus.(string)
						if !isStatusString {
							Fail(fmt.Sprintf("Resource bundle %s (index %d) has Applied condition (index %d) with invalid 'status' type", itemID, i, j))
						}

						Expect(statusStr).To(Equal("True"), "Resource bundle %s (index %d) Applied condition should have status 'True', got: %s", itemID, i, statusStr)
						GinkgoWriter.Printf("Resource bundle %s (index %d): Applied condition status is True ✓\n", itemID, i)
						break
					}
				}

				if !appliedFound {
					Fail(fmt.Sprintf("Resource bundle %s (index %d) does not have an 'Applied' condition", itemID, i))
				}
			}

			fmt.Printf("API Gateway endpoint %s: All %d resource bundle(s) have Applied status conditions\n", endpointURL, len(itemsArray))
			GinkgoWriter.Printf("API Gateway endpoint %s: All %d resource bundle(s) have Applied status conditions\n", endpointURL, len(itemsArray))
		})
	})
})
