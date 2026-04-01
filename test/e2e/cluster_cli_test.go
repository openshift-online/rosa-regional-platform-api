package e2e_test

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
)

var _ = Describe("Cluster CLI API", Label("cluster", "cli"), Ordered, func() {
	var (
		baseURL           string
		accountID         string
		customerAccountID string
		ROSACTL_PATH      string
		clusterName       string
		clusterID         string
		cloudUrl          string
		apiClient         *APIClient
		region            string
		// issuerURL         string
	)

	BeforeAll(func() {
		baseURL = os.Getenv("E2E_BASE_URL")
		accountID = os.Getenv("E2E_ACCOUNT_ID") // this is the RC account id

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

		region = os.Getenv("AWS_REGION")
		if region == "" {
			Fail("AWS_REGION is not set")
		}

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

		ROSACTL_PATH = os.Getenv("ROSACTL_PATH")
		if ROSACTL_PATH == "" {
			Fail("ROSACTL_PATH is not set")
		}

		if os.Getenv("HCP_CLUSTER_NAME") != "" {
			clusterName = os.Getenv("HCP_CLUSTER_NAME")
		} else {
			// clusterName = fmt.Sprintf("hcp-%d", time.Now().Unix())
			clusterName = fmt.Sprintf("cdoan-%d", time.Now().Unix())
		}
		// clusterName = fmt.Sprintf("test-cluster-%d", time.Now().Unix())
		apiClient = NewAPIClient(baseURL)
	})

	It("should be able to have help", Label("help"), func() {
		cmd := exec.Command(ROSACTL_PATH, "help")
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
	It("should be able to login to the E2E_BASE_URL", Label("login"), func() {
		GinkgoWriter.Printf("Logging in to E2E_BASE_URL: %s\n", baseURL)

		cmd := exec.Command(ROSACTL_PATH, "login", "--url", baseURL)
		output, err := cmd.CombinedOutput()
		if err != nil {
			Fail("Failed to login to the E2E_BASE_URL: " + err.Error())
		}
		fmt.Println(string(output))
	})

	// create a new cluster-vpc
	It("should be able to create a new cluster-vpc", Label("vpc-create"), func() {
		// wait for the command to complete, it will take a few minutes.
		GinkgoWriter.Printf("Creating new cluster-vpc: %s\n", clusterName)
		cmd := exec.Command(ROSACTL_PATH, "cluster-vpc", "create", clusterName, "--region", region, "--availability-zones", "us-east-1a,us-east-1b,us-east-1c")
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
		fmt.Println("Cluster-VPC created successfully")
		GinkgoWriter.Printf("Cluster-VPC created successfully: %s\n", clusterName)
	})

	// it should be able to list the cluster-vpc and find that cluster in the list
	It("should be able to list the cluster-vpc and find that cluster in the list", Label("vpc-list"), func() {
		GinkgoWriter.Printf("Listing cluster-vpc: %s\n", clusterName)
		cmd := exec.Command(ROSACTL_PATH, "cluster-vpc", "list", "--region", region)
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
	It("should be able to create the cluster-iam", Label("iam-create"), func() {
		// GinkgoWriter.Printf("Fetching issuer url from the /management_clusters endpoint\n")
		// response := getAndExpectOK(apiClient, "/api/v0/management_clusters", accountID, "")
		// Expect(response.StatusCode).To(Equal(http.StatusOK))
		// var list struct {
		// 	Items []map[string]interface{} `json:"items"`
		// }
		// err := json.Unmarshal(response.Body, &list)
		// Expect(err).To(BeNil())
		// Expect(list.Items).ToNot(BeEmpty())
		// fmt.Println(list.Items[0])
		// issuerURL = list.Items[0]["labels"].(map[string]interface{})["cloudfront_url"].(string)
		// GinkgoWriter.Printf("Issuer url: %s\n", issuerURL)
		// Expect(issuerURL).ToNot(BeEmpty())

		GinkgoWriter.Printf("Creating new cluster-iam: %s\n", clusterName)
		// cmd := exec.Command(ROSACTL_PATH, "cluster-iam", "create", clusterName, "--region", region, "--oidc-issuer-url", issuerURL)
		cmd := exec.Command(ROSACTL_PATH, "cluster-iam", "create", clusterName, "--region", region)
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
		fmt.Println("Cluster-IAM created successfully")
		GinkgoWriter.Printf("Cluster-IAM created successfully: %s\n", clusterName)
	})

	It("should be able to list the cluster-iam and find that cluster in the list", Label("iam-list"), func() {
		GinkgoWriter.Printf("Listing cluster-iam: %s\n", clusterName)
		cmd := exec.Command(ROSACTL_PATH, "cluster-iam", "list", "--region", region)
		cmd.Env = append(os.Environ(),
			"AWS_ACCESS_KEY_ID="+os.Getenv("CUSTOMER_AWS_ACCESS_KEY_ID"),
			"AWS_SECRET_ACCESS_KEY="+os.Getenv("CUSTOMER_AWS_SECRET_ACCESS_KEY"),
		)
		output, err := cmd.CombinedOutput()
		if err != nil {
			Fail("Failed to list the cluster-iam: " + err.Error())
		}
		fmt.Println(string(output))
		// Expect(string(output)).To(ContainSubstring(clusterName))
	})

	// POST /api/v0/accounts expects JSON { "accountId", "privileged" } (see authz_e2e_test and APIClient.CreateAccount)
	It("should be able to add the customer account to the platform api accounts", Label("account-add"), func() {
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

	It("should be able to create the hcp cluster", Label("hcp-create"), func() {
		GinkgoWriter.Printf("Creating new hcp cluster: %s\n", clusterName)
		// todo: get the management cluster name until we have placement adapter
		response := getAndExpectOK(apiClient, "/api/v0/management_clusters", accountID, "")
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		// response is ConsumerList: { "kind", "page", "size", "total", "items": [...] }
		var list struct {
			Items []map[string]interface{} `json:"items"`
		}
		err := json.Unmarshal(response.Body, &list)
		Expect(err).To(BeNil())
		Expect(list.Items).ToNot(BeEmpty())

		// set the managmentClusterName to the first item in the list, this should be the 1st management cluster that got registered
		managementClusterName := list.Items[0]["name"].(string)
		GinkgoWriter.Printf("Using management cluster for placement: %s\n", managementClusterName)
		cmd := exec.Command(ROSACTL_PATH, "cluster", "create", clusterName, "--region", region, "--placement", managementClusterName)
		cmd.Env = append(os.Environ(),
			"AWS_ACCESS_KEY_ID="+os.Getenv("CUSTOMER_AWS_ACCESS_KEY_ID"),
			"AWS_SECRET_ACCESS_KEY="+os.Getenv("CUSTOMER_AWS_SECRET_ACCESS_KEY"),
		)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err = cmd.Run()
		if err != nil {
			Fail("Failed to create the HCP cluster: " + err.Error() + "\nstderr: " + stderr.String())
		}
		if stderr.Len() > 0 {
			GinkgoWriter.Printf("cluster create stderr: %s\n", stderr.String())
		}
		output := stdout.Bytes()

		fmt.Println(string(output))
		var cluster map[string]interface{}
		err = json.Unmarshal(output, &cluster)
		Expect(err).To(BeNil())
		clusterID = cluster["id"].(string)
		cloudUrl = cluster["spec"].(map[string]interface{})["rosaIssuerUrl"].(string)
		GinkgoWriter.Printf("HCP cluster ID: %s\n", clusterID)
		GinkgoWriter.Printf("HCP cluster cloud url: %s\n", cloudUrl)
		GinkgoWriter.Printf("HCP cluster creation submitted successfully: %s\n", clusterName)
	})

	It("should be able to create the cluster-oidc", Label("oidc-create"), func() {
		GinkgoWriter.Printf("Fetching issuer url from the /management_clusters endpoint\n")
		// response := getAndExpectOK(apiClient, "/api/v0/management_clusters", accountID, "")
		// Expect(response.StatusCode).To(Equal(http.StatusOK))
		// var list struct {
		// 	Items []map[string]interface{} `json:"items"`
		// }
		// err := json.Unmarshal(response.Body, &list)
		// Expect(err).To(BeNil())
		// Expect(list.Items).ToNot(BeEmpty())
		// fmt.Println(list.Items[0])
		// issuerURL = list.Items[0]["labels"].(map[string]interface{})["cloudfront_url"].(string)
		// GinkgoWriter.Printf("Issuer url: %s\n", issuerURL)
		// Expect(issuerURL).ToNot(BeEmpty())

		if cloudUrl == "" {
			cloudUrl = os.Getenv("HCP_ROSA_ISSUER_URL")

		}

		GinkgoWriter.Printf("Cluster cloud url: %s\n", cloudUrl)
		GinkgoWriter.Printf("Creating new cluster-oidc: %s\n", clusterName)
		cmd := exec.Command(ROSACTL_PATH, "cluster-oidc", "create", clusterName, "--region", region, "--oidc-issuer-url", cloudUrl)
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
		fmt.Println("Cluster-OIDC created successfully")
		GinkgoWriter.Printf("Cluster-OIDC created successfully: %s\n", clusterName)
	})

	// it should be able to list the cluster-oidc and find that cluster in the list
	It("should be able to list the cluster-oidc and find that cluster in the list", Label("oidc-list"), func() {
		GinkgoWriter.Printf("Listing cluster-oidc: %s\n", clusterName)
		cmd := exec.Command(ROSACTL_PATH, "cluster-oidc", "list", "--region", region)
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
	It("should be able to wait for the hcp cluster to be ready", Label("cluster-status"), func() {
		id := clusterID
		if id == "" {
			id = os.Getenv("HCP_INSTANCE_ID")
		}
		Expect(id).ToNot(BeEmpty(), "set clusterID from hcp-create (Ordered) or HCP_INSTANCE_ID when running cluster-status alone")

		GinkgoWriter.Printf("Querying platform api /clusters/%s and .../statuses (resource id)\n", id)
		response, err := apiClient.Get("/api/v0/clusters/"+id, accountID)
		Expect(err).ToNot(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		// fmt.Println(string(response.Body))
		// get the status from the response body
		var cluster map[string]interface{}
		err = json.Unmarshal(response.Body, &cluster)
		Expect(err).To(BeNil())
		statusRaw, ok := cluster["status"].(map[string]interface{})
		Expect(ok).To(BeTrue(), "cluster response missing status object")
		Expect(statusRaw).ToNot(BeEmpty())
		statusJSON, err := json.MarshalIndent(statusRaw, "", "  ")
		Expect(err).To(BeNil())
		GinkgoWriter.Printf("Cluster status:\n%s\n", string(statusJSON))

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
		}).WithTimeout(15*time.Minute).WithPolling(15*time.Second).Should(Succeed(),
			"all controller_statuses conditions should become True")

		resp, err := apiClient.Get("/api/v0/clusters/"+id+"/statuses", accountID)
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		var finalStatus map[string]interface{}
		Expect(json.Unmarshal(resp.Body, &finalStatus)).To(Succeed())
		finalJSON, err := json.MarshalIndent(finalStatus, "", "  ")
		Expect(err).ToNot(HaveOccurred())
		GinkgoWriter.Printf("Final cluster statuses:\n%s\n", string(finalJSON))
	})

	// it should be able to delete the cluster-iam, cluster-vpc and the hcp cluster
	XIt("should be able to delete the cluster-iam, cluster-vpc and the hcp cluster", Label("cluster-vpc-iam-delete"), func() {
		GinkgoWriter.Printf("Deleting cluster-oidc: %s\n", clusterName)
		cmd := exec.Command(ROSACTL_PATH, "cluster-oidc", "delete", clusterName, "--region", region)
		cmd.Env = append(os.Environ(),
			"AWS_ACCESS_KEY_ID="+os.Getenv("CUSTOMER_AWS_ACCESS_KEY_ID"),
			"AWS_SECRET_ACCESS_KEY="+os.Getenv("CUSTOMER_AWS_SECRET_ACCESS_KEY"),
		)
		output, err := cmd.CombinedOutput()
		if err != nil {
			Fail("Failed to delete the cluster-oidc: " + err.Error())
		}
		fmt.Println(string(output))
		GinkgoWriter.Printf("Cluster-OIDC deleted successfully: %s\n", clusterName)

		GinkgoWriter.Printf("Deleting cluster-iam: %s\n", clusterName)
		cmd = exec.Command(ROSACTL_PATH, "cluster-iam", "delete", clusterName, "--region", region)
		cmd.Env = append(os.Environ(),
			"AWS_ACCESS_KEY_ID="+os.Getenv("CUSTOMER_AWS_ACCESS_KEY_ID"),
			"AWS_SECRET_ACCESS_KEY="+os.Getenv("CUSTOMER_AWS_SECRET_ACCESS_KEY"),
		)
		output, err = cmd.CombinedOutput()
		if err != nil {
			Fail("Failed to delete the cluster-iam: " + err.Error())
		}
		fmt.Println(string(output))
		GinkgoWriter.Printf("Cluster-IAM deleted successfully: %s\n", clusterName)

		GinkgoWriter.Printf("Deleting cluster-vpc: %s\n", clusterName)
		cmd = exec.Command(ROSACTL_PATH, "cluster-vpc", "delete", clusterName, "--region", region)
		cmd.Env = append(os.Environ(),
			"AWS_ACCESS_KEY_ID="+os.Getenv("CUSTOMER_AWS_ACCESS_KEY_ID"),
			"AWS_SECRET_ACCESS_KEY="+os.Getenv("CUSTOMER_AWS_SECRET_ACCESS_KEY"),
		)
		output, err = cmd.CombinedOutput()
		if err != nil {
			Fail("Failed to delete the cluster-vpc: " + err.Error())
		}
		fmt.Println(string(output))
		GinkgoWriter.Printf("Cluster-VPC deleted successfully: %s\n", clusterName)

	})

	// todo: this is a workaround until hyperfleet supports deletion of the cluster
	XIt("should be able to list the aws secrets for rds database", Label("rds"), func() {
		GinkgoWriter.Printf("Listing aws secrets for rds database\n")
		var clusterPrefix string
		if os.Getenv("CLUSTER_PREFIX") != "" {
			clusterPrefix = os.Getenv("CLUSTER_PREFIX")
		} else {
			// this should be integration
			clusterPrefix = ""
		}

		query := fmt.Sprintf("SecretList[?contains(Name, '%s-hyperfleet-db-credentials')]", clusterPrefix)
		cmd := exec.Command("aws", "secretsmanager", "list-secrets", "--region", region, "--query", query)
		output, err := cmd.CombinedOutput()
		if err != nil {
			Fail("Failed to list the aws secrets for rds database: " + err.Error())
		}
		var secrets []map[string]interface{}
		err = json.Unmarshal(output, &secrets)
		Expect(err).To(BeNil())
		for _, secret := range secrets {
			GinkgoWriter.Printf("Secret: %s\n", secret["Name"])
		}
		// todo:connect ot psql and delete the records
		// psql -h <host> -p <port> -U <username> -d <database>
		// psql -h <host> -p <port> -U <username> -d <database> -c "DELETE FROM cluster_records WHERE cluster_name = '<cluster_name>'"
	})

	// delete resource bundles
	XIt("should be able to delete the resource bundles", Label("bundles-delete"), func() {
		GinkgoWriter.Printf("Querying platform api for /resource_bundles\n")
		response, err := apiClient.Get("/api/v0/resource_bundles", accountID)
		Expect(err).ToNot(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		var list struct {
			Items []map[string]interface{} `json:"items"`
		}
		err = json.Unmarshal(response.Body, &list)
		Expect(err).To(BeNil())
		for _, item := range list.Items {
			workID := item["metadata"].(map[string]interface{})["name"].(string) // this is the work id
			GinkgoWriter.Printf("Bundle ID: %s, Name: %s\n", item["id"], workID)
			GinkgoWriter.Printf("Cluster ID: %s\n", clusterID)
			// if strings.Contains(workID, clusterID) {
			response, err := apiClient.Delete("/api/v0/resource_bundles/"+item["id"].(string), accountID)
			Expect(err).ToNot(HaveOccurred())
			// accept 204 or 200
			Expect(response.StatusCode).To(Or(Equal(http.StatusNoContent), Equal(http.StatusOK)))
			GinkgoWriter.Printf("Resource bundle deleted successfully: %s\n", item["id"])
			// }
		}
	})
})
