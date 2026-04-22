package e2e_cli_test

// CLI E2E Tests - HCP Cluster Creation via rosactl
//
// Run individual tests using label filters:
//
// Setup phase:
//   ginkgo --label-filter="setup" ./test/e2e-cli         # All setup tests
//   ginkgo --label-filter="vpc-create" ./test/e2e-cli    # Just VPC creation
//   ginkgo --label-filter="iam-create" ./test/e2e-cli    # Just IAM creation
//
// Create phase:
//   ginkgo --label-filter="create" ./test/e2e-cli        # Cluster creation
//   ginkgo --label-filter="hcp-create" ./test/e2e-cli    # Just HCP cluster
//
// Monitor phase:
//   ginkgo --label-filter="monitor" ./test/e2e-cli       # Status checks
//   ginkgo --label-filter="cluster-status" ./test/e2e-cli # Just status polling
//
// Cleanup phase:
//   ginkgo --label-filter="cleanup" ./test/e2e-cli       # All cleanup tests
//   ginkgo --label-filter="vpc-delete" ./test/e2e-cli    # Just VPC deletion
//
// Available labels:
//   help, login, vpc-create, vpc-list, iam-create, iam-list, account-add,
//   hcp-create, oidc-create, oidc-list, cluster-status, nodepools-wait,
//   hcp-patch, bundles-delete, bundles-wait, oidc-delete, iam-delete, vpc-delete
//
// Group labels: setup, create, monitor, update, cleanup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	e2e "github.com/openshift/rosa-regional-platform-api/test/e2e"
)

// runRosactl runs rosactl with customer AWS credentials under a deadline.
// Stdin is nil so the child does not block waiting for input when Ginkgo has no TTY.
// AWS_PAGER is cleared to avoid aws-cli-style pagers hanging the subprocess.
// Override the deadline with E2E_ROSACTL_TIMEOUT (e.g. 45m, 1h).
// func runRosactl(bin, region string, args ...string) ([]byte, error) {
// 	timeout := 30 * time.Minute
// 	if s := os.Getenv("E2E_ROSACTL_TIMEOUT"); s != "" {
// 		if d, err := time.ParseDuration(s); err == nil {
// 			timeout = d
// 		}
// 	}
// 	ctx, cancel := context.WithTimeout(context.Background(), timeout)
// 	defer cancel()

// 	cmd := exec.CommandContext(ctx, bin, args...)
// 	cmd.Env = append(os.Environ(),
// 		"AWS_ACCESS_KEY_ID="+os.Getenv("CUSTOMER_AWS_ACCESS_KEY_ID"),
// 		"AWS_SECRET_ACCESS_KEY="+os.Getenv("CUSTOMER_AWS_SECRET_ACCESS_KEY"),
// 		"AWS_REGION="+region,
// 		"AWS_PAGER=",
// 	)
// 	cmd.Stdin = nil

// 	out, err := cmd.CombinedOutput()
// 	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
// 		return out, fmt.Errorf("rosactl %q timed out after %v (set E2E_ROSACTL_TIMEOUT): %w\noutput:\n%s",
// 			strings.Join(args, " "), timeout, err, string(out))
// 	}
// 	return out, err
// }

var _ = Describe("ROSACTL CLI E2E Tests", Ordered, func() {
	var (
		baseURL           string
		accountID         string
		customerAccountID string
		ROSACTL_BIN       string
		clusterName       string
		clusterID         string
		cloudUrl          string
		region            string
		apiClient         *e2e.APIClient
	)

	BeforeAll(func() {

		//--------------------------------
		// Required environment variables for e2e testing
		//--------------------------------
		baseURL = os.Getenv("BASE_URL")
		if baseURL == "" {
			Skip("BASE_URL is not set")
		}
		region = os.Getenv("AWS_REGION")
		if region == "" {
			region = "us-east-1"
			GinkgoWriter.Printf("No AWS_REGION set, defaulting to %s\n", region)
		}
		ROSACTL_BIN = os.Getenv("ROSACTL_BIN")
		if ROSACTL_BIN == "" {
			Skip("ROSACTL_BIN is not set")
		}
		if os.Getenv("CUSTOMER_AWS_ACCESS_KEY_ID") == "" {
			Skip("CUSTOMER_AWS_ACCESS_KEY_ID is not set")
		}
		if os.Getenv("CUSTOMER_AWS_SECRET_ACCESS_KEY") == "" {
			Skip("CUSTOMER_AWS_SECRET_ACCESS_KEY is not set")
		}

		// this is the RC account id, a privileged account id to the baseURL orAPI_URL
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
		GinkgoWriter.Printf("E2E_ACCOUNT_ID: %s\n", accountID)

		customerAccountID = os.Getenv("E2E_CUSTOMER_ACCOUNT_ID")
		if customerAccountID == "" {
			GinkgoWriter.Printf("No E2E_CUSTOMER_ACCOUNT_ID set, using AWS STS caller identity\n")
			cmd := exec.Command("aws", "sts", "get-caller-identity", "--query", "Account", "--output", "text")
			cmd.Env = append(os.Environ(),
				"AWS_ACCESS_KEY_ID="+os.Getenv("CUSTOMER_AWS_ACCESS_KEY_ID"),
				"AWS_SECRET_ACCESS_KEY="+os.Getenv("CUSTOMER_AWS_SECRET_ACCESS_KEY"),
			)
			output, err := cmd.CombinedOutput()
			if err != nil {
				Fail("Failed to get AWS customer account ID: " + err.Error())
			}
			customerAccountID = strings.TrimSpace(string(output))
			GinkgoWriter.Printf("Customer account ID: %s\n", customerAccountID)
		}

		//--------------------------------
		// Optional: development overrides
		//--------------------------------
		if os.Getenv("HCP_CLUSTER_NAME") != "" {
			clusterName = os.Getenv("HCP_CLUSTER_NAME")
		} else {
			// Default to e2e-<timestamp>
			clusterName = fmt.Sprintf("e2e-%d", time.Now().Unix())
		}

		apiClient = e2e.NewAPIClient(baseURL)
	})

	It("should be able to have help", Label("help"), func() {
		cmd := exec.Command(ROSACTL_BIN, "help")
		output, err := cmd.CombinedOutput()
		if err != nil {
			Fail("Failed to get help: " + err.Error())
		}
		fmt.Println(string(output))
		Expect(string(output)).To(ContainSubstring("Usage:"))
	})

	// Add your CLI-based cluster tests here
	// locate the rosactl cli command
	// run the rosactl cli command
	// it should be able to run the rosactl command and login to the e2e_base_url
	// it should be able to create a new cluster with the given name and region
	It("should be able to login to the BASE_URL", Label("login", "setup"), func() {
		GinkgoWriter.Printf("Logging in to BASE_URL: %s\n", baseURL)

		cmd := exec.Command(ROSACTL_BIN, "login", "--url", baseURL)
		output, err := cmd.CombinedOutput()
		if err != nil {
			Fail("Failed to login to the BASE_URL: " + err.Error())
		}
		fmt.Println(string(output))
	})

	// create a new cluster-vpc
	It("should be able to create a new cluster-vpc", Label("vpc-create", "setup"), func() {
		// wait for the command to complete, it will take a few minutes.
		GinkgoWriter.Printf("Creating new cluster-vpc: %s\n", clusterName)
		// GinkgoWriter.Printf("Command: %s %s %s %s %s\n", ROSACTL_BIN, "cluster-vpc", "create", clusterName, "--region", region, "--availability-zones", "us-east-1a")
		cmd := exec.Command(ROSACTL_BIN, "cluster-vpc", "create", clusterName, "--region", region, "--availability-zones", "us-east-1a")
		cmd.Env = append(os.Environ(),
			"AWS_ACCESS_KEY_ID="+os.Getenv("CUSTOMER_AWS_ACCESS_KEY_ID"),
			"AWS_SECRET_ACCESS_KEY="+os.Getenv("CUSTOMER_AWS_SECRET_ACCESS_KEY"),
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			Fail("Failed to create a new cluster-vpc: " + err.Error())
		}
		GinkgoWriter.Printf("Cluster-VPC created successfully: %s\n", clusterName)
	})

	// it should be able to list the cluster-vpc and find that cluster in the list
	It("should be able to list the cluster-vpc and find that cluster in the list", Label("vpc-list", "setup"), func() {
		GinkgoWriter.Printf("Listing cluster-vpc: %s\n", clusterName)
		cmd := exec.Command(ROSACTL_BIN, "cluster-vpc", "list", "--region", region)
		cmd.Env = append(os.Environ(),
			"AWS_ACCESS_KEY_ID="+os.Getenv("CUSTOMER_AWS_ACCESS_KEY_ID"),
			"AWS_SECRET_ACCESS_KEY="+os.Getenv("CUSTOMER_AWS_SECRET_ACCESS_KEY"),
		)
		output, err := cmd.CombinedOutput()
		if err != nil {
			Fail("Failed to list the cluster-vpc: " + err.Error())
		}
		fmt.Println(string(output))
		Expect(string(output)).To(ContainSubstring(clusterName))
	})

	// create a new cluster-iam
	It("should be able to create the cluster-iam", Label("iam-create", "setup"), func() {
		GinkgoWriter.Printf("Creating new cluster-iam: %s\n", clusterName)
		cmd := exec.Command(ROSACTL_BIN, "cluster-iam", "create", clusterName, "--region", region)
		cmd.Env = append(os.Environ(),
			"AWS_ACCESS_KEY_ID="+os.Getenv("CUSTOMER_AWS_ACCESS_KEY_ID"),
			"AWS_SECRET_ACCESS_KEY="+os.Getenv("CUSTOMER_AWS_SECRET_ACCESS_KEY"),
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			Fail("Failed to create the cluster-iam: " + err.Error())
		}
		GinkgoWriter.Printf("Cluster-IAM created successfully: %s\n", clusterName)
	})

	It("should be able to list the cluster-iam and find that cluster in the list", Label("iam-list", "setup"), func() {
		GinkgoWriter.Printf("Listing cluster-iam: %s\n", clusterName)
		cmd := exec.Command(ROSACTL_BIN, "cluster-iam", "list", "--region", region)
		cmd.Env = append(os.Environ(),
			"AWS_ACCESS_KEY_ID="+os.Getenv("CUSTOMER_AWS_ACCESS_KEY_ID"),
			"AWS_SECRET_ACCESS_KEY="+os.Getenv("CUSTOMER_AWS_SECRET_ACCESS_KEY"),
		)
		output, err := cmd.CombinedOutput()
		if err != nil {
			Fail("Failed to list the cluster-iam: " + err.Error())
		}
		fmt.Println(string(output))
		Expect(string(output)).To(ContainSubstring(clusterName))
	})

	It("should be able to add the customer account to the platform api accounts", Label("account-add", "setup"), func() {
		GinkgoWriter.Printf("Adding customer account to the platform api accounts: %s %s\n", accountID, customerAccountID)
		body := map[string]interface{}{
			"accountId":  customerAccountID,
			"privileged": true,
		}
		response, err := apiClient.Post("/api/v0/accounts", body, accountID)
		Expect(err).ToNot(HaveOccurred())
		switch response.StatusCode {
		case http.StatusCreated:
			GinkgoWriter.Printf("Customer account %s enabled\n", customerAccountID)
		case http.StatusConflict:
			var errBody map[string]interface{}
			Expect(json.Unmarshal(response.Body, &errBody)).To(Succeed())
			Expect(errBody["code"]).To(Equal("account-exists"), "unexpected 409 body: %s", string(response.Body))
			GinkgoWriter.Printf("Customer account %s already enabled (409 account-exists)\n", customerAccountID)
		default:
			Fail(fmt.Sprintf("failed to enable customer account: status %d body: %s", response.StatusCode, string(response.Body)))
		}
		GinkgoWriter.Printf("Customer account %s ready in platform api accounts (RC %s)\n", customerAccountID, accountID)
	})

	It("should be able to create the hcp cluster", Label("hcp-create", "create"), func() {
		GinkgoWriter.Printf("Creating new HCP cluster: %s\n", clusterName)
		cmd := exec.Command(ROSACTL_BIN, "cluster", "create", clusterName, "--region", region, "--output", "json")
		cmd.Env = append(os.Environ(),
			"AWS_ACCESS_KEY_ID="+os.Getenv("CUSTOMER_AWS_ACCESS_KEY_ID"),
			"AWS_SECRET_ACCESS_KEY="+os.Getenv("CUSTOMER_AWS_SECRET_ACCESS_KEY"),
		)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		// Check if cluster creation failed due to conflict (cluster already exists)
		if err != nil {
			stderrStr := stderr.String()
			// Check for 409 Conflict or "already exists" in stderr
			if strings.Contains(stderrStr, "409") || strings.Contains(stderrStr, "already exists") || strings.Contains(stderrStr, "Conflict") {
				GinkgoWriter.Printf("Cluster %s already exists (409 Conflict), retrieving existing cluster\n", clusterName)
				// List clusters to find the existing one
				response, listErr := apiClient.Get(fmt.Sprintf("/api/v0/clusters?limit=100"), accountID)
				Expect(listErr).ToNot(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusOK))

				var clusterList struct {
					Items []map[string]interface{} `json:"items"`
				}
				Expect(json.Unmarshal(response.Body, &clusterList)).To(Succeed())

				// Find our cluster by name
				var found bool
				for _, item := range clusterList.Items {
					if item["name"] == clusterName {
						clusterID = item["id"].(string)
						if spec, ok := item["spec"].(map[string]interface{}); ok {
							if issuerUrl, ok := spec["cloudUrl"].(string); ok {
								cloudUrl = issuerUrl
							}
						}
						found = true
						break
					}
				}
				Expect(found).To(BeTrue(), "cluster %s should exist after 409 conflict", clusterName)
				GinkgoWriter.Printf("Found existing HCP cluster ID: %s\n", clusterID)
				GinkgoWriter.Printf("Found existing HCP cluster cloud url: %s\n", cloudUrl)
				return
			}
			Fail("Failed to create the HCP cluster: " + err.Error() + "\nstderr: " + stderrStr)
		}

		if stderr.Len() > 0 {
			GinkgoWriter.Printf("HCP cluster create stderr: %s\n", stderr.String())
		}
		output := stdout.Bytes()

		// Print the create cluster output
		if os.Getenv("E2E_CREATE_CLUSTER_LOG") != "" {
			fmt.Println(string(output))
		}

		var cluster map[string]interface{}
		err = json.Unmarshal(output, &cluster)
		Expect(err).To(BeNil())
		clusterID = cluster["id"].(string)
		if spec, ok := cluster["spec"].(map[string]interface{}); ok {
			if issuerUrl, ok := spec["cloudUrl"].(string); ok {
				cloudUrl = issuerUrl
			}
		}
		GinkgoWriter.Printf("HCP cluster ID: %s\n", clusterID)
		GinkgoWriter.Printf("HCP cluster cloud url: %s\n", cloudUrl)
		GinkgoWriter.Printf("HCP cluster created successfully: %s\n", clusterName)
	})

	It("should be able to create the cluster-oidc", Label("oidc-create", "setup"), func() {
		GinkgoWriter.Printf("Creating new cluster-oidc: %s\n", clusterName)
		if cloudUrl == "" {
			cloudUrl = os.Getenv("HCP_ROSA_ISSUER_URL")
		}
		GinkgoWriter.Printf("HCP cluster cloud url: %s\n", cloudUrl)
		cmd := exec.Command(ROSACTL_BIN, "cluster-oidc", "create", clusterName, "--region", region, "--oidc-issuer-url", cloudUrl)
		cmd.Env = append(os.Environ(),
			"AWS_ACCESS_KEY_ID="+os.Getenv("CUSTOMER_AWS_ACCESS_KEY_ID"),
			"AWS_SECRET_ACCESS_KEY="+os.Getenv("CUSTOMER_AWS_SECRET_ACCESS_KEY"),
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			Fail("Failed to create the cluster-oidc: " + err.Error())
		}
		GinkgoWriter.Printf("HCP cluster-oidc created successfully: %s\n", clusterName)
	})

	// it should be able to list the cluster-oidc and find that cluster in the list
	It("should be able to list the cluster-oidc and find that cluster in the list", Label("oidc-list", "setup"), func() {
		GinkgoWriter.Printf("Listing cluster-oidc: %s\n", clusterName)
		cmd := exec.Command(ROSACTL_BIN, "cluster-oidc", "list", "--region", region)
		cmd.Env = append(os.Environ(),
			"AWS_ACCESS_KEY_ID="+os.Getenv("CUSTOMER_AWS_ACCESS_KEY_ID"),
			"AWS_SECRET_ACCESS_KEY="+os.Getenv("CUSTOMER_AWS_SECRET_ACCESS_KEY"),
		)
		output, err := cmd.CombinedOutput()
		if err != nil {
			Fail("Failed to list the cluster-oidc: " + err.Error())
		}
		fmt.Println(string(output))
		Expect(string(output)).To(ContainSubstring(clusterName))
	})

	// GET /api/v0/clusters/{id} and /statuses use the Hyperfleet resource id (e.g. "2pdl6eud5btdtvgv2f4roaca96e9mvtn"),
	// not the cluster display name. List responses are { "items": [ { "id", "name", "spec", "status", ... } ], ... }.
	It("should be able to wait for the hcp cluster to be ready", Label("cluster-status", "monitor"), func() {
		id := clusterID
		if id == "" {
			id = os.Getenv("HCP_INSTANCE_ID")
		}
		Expect(id).ToNot(BeEmpty(), "set clusterID from hcp-create (Ordered) or HCP_INSTANCE_ID when running cluster-status alone")

		GinkgoWriter.Printf("Querying platform api /clusters/%s and .../statuses (HCP cluster resource id)\n", id)
		response, err := apiClient.Get("/api/v0/clusters/"+id, accountID)
		Expect(err).ToNot(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		// get the status from the response body
		var cluster map[string]interface{}
		err = json.Unmarshal(response.Body, &cluster)
		Expect(err).To(BeNil())
		statusRaw, ok := cluster["status"].(map[string]interface{})
		Expect(ok).To(BeTrue(), "cluster response missing status object")
		Expect(statusRaw).ToNot(BeEmpty())
		statusJSON, err := json.MarshalIndent(statusRaw, "", "  ")
		Expect(err).To(BeNil())
		GinkgoWriter.Printf("HCP initial cluster status:\n%s\n", string(statusJSON))

		// Top-level status uses camelCase; message/reason live on conditions[], not on status root.
		// GinkgoWriter.Printf("Cluster status phase: %v lastUpdateTime: %v observedGeneration: %v\n",
		// statusRaw["phase"], statusRaw["lastUpdateTime"], statusRaw["observedGeneration"])

		// Response is pkg/types.ClusterStatusResponse: { "cluster_id", "status", "controller_statuses": [...] }.
		// Poll until reconcilers report every condition as True (right after create they are often False).
		//
		// Logging notes:
		// - Code after a failing g.Expect never runs, so you only see logs that run *before* the assertion that fails.
		// - GinkgoWriter is buffered unless you run `ginkgo -v` (then it usually streams); it is not the same as os.Stdout.
		// - For a snapshot on every poll (including failed attempts), set E2E_STATUS_POLL_LOG=1 (writes to stderr).
		Eventually(func(g Gomega) {
			resp, err := apiClient.Get("/api/v0/clusters/"+id+"/statuses", accountID)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var statusEnvelope struct {
				ClusterID          string                   `json:"cluster_id"`
				Status             map[string]interface{}   `json:"status"`
				ControllerStatuses []map[string]interface{} `json:"controller_statuses"`
			}
			g.Expect(json.Unmarshal(resp.Body, &statusEnvelope)).To(Succeed())

			if os.Getenv("E2E_STATUS_POLL_LOG") != "" {
				snap, mErr := json.MarshalIndent(statusEnvelope, "", "  ")
				if mErr == nil {
					_, _ = fmt.Fprintf(os.Stderr, "\n[%s] GET /clusters/%s/statuses (poll)\n%s\n",
						time.Now().Format(time.RFC3339), id, snap)
				}
			}
			GinkgoWriter.Printf("[%s] polled cluster /statuses (stream with: ginkgo -v)\n", time.Now().Format(time.RFC3339))

			g.Expect(statusEnvelope.ControllerStatuses).NotTo(BeEmpty(), "controller_statuses should be populated")

			// Nested JSON arrays decode as []interface{} with map elements, not []map[string]interface{}.
			for _, cs := range statusEnvelope.ControllerStatuses {
				raw, ok := cs["conditions"].([]interface{})
				g.Expect(ok).To(BeTrue(), "controller status should include conditions: %#v", cs)
				g.Expect(raw).NotTo(BeEmpty(), "conditions should be non-empty while cluster reconciles")
				for _, item := range raw {
					cond, ok := item.(map[string]interface{})
					g.Expect(ok).To(BeTrue())
					g.Expect(cond["status"]).To(Equal("True"), "condition %#v should be True", cond)
				}
			}
		}).WithTimeout(20*time.Minute).WithPolling(20*time.Second).Should(Succeed(),
			"all controller_statuses conditions should become True")

		resp, err := apiClient.Get("/api/v0/clusters/"+id+"/statuses", accountID)
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		var finalStatus map[string]interface{}
		Expect(json.Unmarshal(resp.Body, &finalStatus)).To(Succeed())
		finalJSON, err := json.MarshalIndent(finalStatus, "", "  ")
		Expect(err).ToNot(HaveOccurred())
		GinkgoWriter.Printf("HCP final cluster statuses:\n%s\n", string(finalJSON))
	})

	// it should wait 5m for the hcp and nodepools to be deployed
	It("should be able to wait 5m for the hcp and nodepools to be deployed", Label("nodepools-wait", "monitor"), func() {
		GinkgoWriter.Printf("Waiting 5m for the hcp and nodepools to be deployed\n")
		time.Sleep(5 * time.Minute)
		GinkgoWriter.Printf("HCP and nodepools deployed successfully\n")
	})

	It("should be able to delete the hcp cluster", Label("hcp-delete", "cleanup"), func() {
		if clusterID == "" {
			clusterID = os.Getenv("HCP_INSTANCE_ID")
			if clusterID == "" {
				Skip("clusterID not set - run full Ordered suite or set HCP_INSTANCE_ID")
			}
		}
		GinkgoWriter.Printf("Deleting the hcp clusterId: %s\n", clusterID)
		response, err := apiClient.Delete("/api/v0/clusters/"+clusterID, accountID)
		Expect(err).ToNot(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusAccepted))
		GinkgoWriter.Printf("HCP cluster deleted successfully: %s\n", clusterName)
	})

	// it should be able to query the /cluster/id until it is deleted
	It("should be able to query the /cluster/id until it is deleted", Label("hcp-delete", "cluster-query", "cleanup"), func() {
		GinkgoWriter.Printf("Querying the hcp clusterId: %s\n", clusterID)
		if clusterID == "" {
			clusterID = os.Getenv("HCP_INSTANCE_ID")
			if clusterID == "" {
				Skip("clusterID not set - run full Ordered suite or set HCP_INSTANCE_ID")
			}
		}
		Eventually(func(g Gomega) {
			response, err := apiClient.Get("/api/v0/clusters/"+clusterID, accountID)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(response.StatusCode).To(Or(Equal(http.StatusNotFound), Equal(http.StatusGone)))
		}).WithTimeout(10*time.Minute).WithPolling(30*time.Second).Should(Succeed(), "cluster should be deleted")
		GinkgoWriter.Printf("HCP cluster deleted successfully: %s\n", clusterName)
	})

	// delete the hcp cluster
	// delete all resource bundles
	It("should be able to delete the resource bundles", Label("hcp-delete", "bundles-delete", "cleanup"), func() {
		if clusterID == "" {
			clusterID = os.Getenv("HCP_INSTANCE_ID")
			if clusterID == "" {
				Skip("clusterID not set - run full Ordered suite or set HCP_INSTANCE_ID")
			}
		}

		GinkgoWriter.Printf("Querying platform api for /resource_bundles\n")

		// Fetch all pages of resource bundles
		allBundles := []map[string]interface{}{}
		page := 1
		for {
			response, err := apiClient.Get(fmt.Sprintf("/api/v0/resource_bundles?page=%d&size=100", page), accountID)
			Expect(err).ToNot(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusOK))

			var list struct {
				Page  int                      `json:"page"`
				Size  int                      `json:"size"`
				Total int                      `json:"total"`
				Items []map[string]interface{} `json:"items"`
			}
			err = json.Unmarshal(response.Body, &list)
			Expect(err).To(BeNil())

			allBundles = append(allBundles, list.Items...)
			GinkgoWriter.Printf("Fetched page %d: %d bundles (total: %d)\n", page, len(list.Items), list.Total)

			// Check if we've fetched all items
			if len(allBundles) >= list.Total || len(list.Items) == 0 {
				break
			}
			page++
		}

		GinkgoWriter.Printf("Total bundles fetched: %d\n", len(allBundles))

		// Delete bundles matching the cluster ID
		deletedCount := 0
		for _, item := range allBundles {
			workID := item["metadata"].(map[string]interface{})["name"].(string) // this is the work id
			if strings.Contains(workID, clusterID) {
				GinkgoWriter.Printf("Deleting bundle ID: %s, Name: %s\n", item["id"], workID)
				response, err := apiClient.Delete("/api/v0/resource_bundles/"+item["id"].(string), accountID)
				Expect(err).ToNot(HaveOccurred())
				// accept 204 or 200
				Expect(response.StatusCode).To(Or(Equal(http.StatusNoContent), Equal(http.StatusOK)))
				GinkgoWriter.Printf("Resource bundle deleted successfully: %s\n", item["id"])
				deletedCount++
			}
		}
		GinkgoWriter.Printf("Deleted %d resource bundles for cluster %s\n", deletedCount, clusterID)
	})

	It("should be able to delete the cluster-oidc", Label("oidc-delete", "cleanup"), func() {
		GinkgoWriter.Printf("Deleting the cluster-oidc: %s\n", clusterName)
		cmd := exec.Command(ROSACTL_BIN, "cluster-oidc", "delete", clusterName, "--region", region)
		cmd.Env = append(os.Environ(),
			"AWS_ACCESS_KEY_ID="+os.Getenv("CUSTOMER_AWS_ACCESS_KEY_ID"),
			"AWS_SECRET_ACCESS_KEY="+os.Getenv("CUSTOMER_AWS_SECRET_ACCESS_KEY"),
		)
		output, err := cmd.CombinedOutput()
		if err != nil {
			Fail(fmt.Sprintf("Failed to delete the cluster-oidc: %v\nOutput:\n%s", err, string(output)))
		}
		GinkgoWriter.Printf("Cluster-OIDC deleted successfully: %s\n", clusterName)
	})

	// Delete cluster-vpc with up to 3 attempts; fail the spec if all attempts return an error.
	It("should be able to try to delete the cluster-vpc, trying 3 times", Label("vpc-delete", "cleanup"), func() {
		const maxAttempts = 3
		const backoffBetweenAttempts = 5 * time.Minute

		var lastErr error
		var lastOutput []byte
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			GinkgoWriter.Printf("cluster-vpc delete attempt %d/%d\n", attempt, maxAttempts)

			// before trying to delete the cluster-vpc, we should list and
			// grep if the cluster-vpc is still there
			cmd := exec.Command(ROSACTL_BIN, "cluster-vpc", "list", "--region", region)
			cmd.Env = append(os.Environ(),
				"AWS_ACCESS_KEY_ID="+os.Getenv("CUSTOMER_AWS_ACCESS_KEY_ID"),
				"AWS_SECRET_ACCESS_KEY="+os.Getenv("CUSTOMER_AWS_SECRET_ACCESS_KEY"),
			)
			output, err := cmd.CombinedOutput()
			if err != nil {
				Fail(fmt.Sprintf("Failed to list the cluster-vpc: %v\nOutput:\n%s", err, string(output)))
			}
			if !strings.Contains(string(output), clusterName) {
				GinkgoWriter.Printf("cluster-vpc does not exist: %s\n", clusterName)
				// we can return successfully because the cluster-vpc is already deleted
				return
			}

			cmd = exec.Command(ROSACTL_BIN, "cluster-vpc", "delete", clusterName, "--region", region)
			cmd.Env = append(os.Environ(),
				"AWS_ACCESS_KEY_ID="+os.Getenv("CUSTOMER_AWS_ACCESS_KEY_ID"),
				"AWS_SECRET_ACCESS_KEY="+os.Getenv("CUSTOMER_AWS_SECRET_ACCESS_KEY"),
			)
			// rosactl may block with its own internal wait
			output, err = cmd.CombinedOutput()
			if err == nil {
				GinkgoWriter.Printf("cluster-vpc deleted successfully: %s\n", clusterName)
				return
			}
			lastErr, lastOutput = err, output
			GinkgoWriter.Printf("cluster-vpc delete attempt %d failed: %v\nOutput:\n%s\n", attempt, err, string(output))
			if attempt < maxAttempts {
				time.Sleep(backoffBetweenAttempts)
			}
		}
		Fail(fmt.Sprintf("cluster-vpc delete failed after %d attempts: %v\nOutput:\n%s", maxAttempts, lastErr, string(lastOutput)))
	})

	// it should be able to delete the cluster-iam
	It("should be able to delete the cluster-iam", Label("iam-delete", "cleanup"), func() {
		GinkgoWriter.Printf("Deleting the cluster-iam: %s\n", clusterName)
		cmd := exec.Command(ROSACTL_BIN, "cluster-iam", "delete", clusterName, "--region", region)
		cmd.Env = append(os.Environ(),
			"AWS_ACCESS_KEY_ID="+os.Getenv("CUSTOMER_AWS_ACCESS_KEY_ID"),
			"AWS_SECRET_ACCESS_KEY="+os.Getenv("CUSTOMER_AWS_SECRET_ACCESS_KEY"),
		)
		output, err := cmd.CombinedOutput()
		if err != nil {
			Fail(fmt.Sprintf("Failed to delete the cluster-iam: %v\nOutput:\n%s", err, string(output)))
		}
		GinkgoWriter.Printf("Cluster-IAM deleted successfully: %s\n", clusterName)
	})

})
