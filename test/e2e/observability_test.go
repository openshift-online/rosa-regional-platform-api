package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// thanosQueryResponse represents the Prometheus/Thanos HTTP query API response.
type thanosQueryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []interface{}     `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

var _ = Describe("Observability", Ordered, func() {
	var (
		rhobsAPIURL string
		rhobsClient *APIClient
	)

	BeforeAll(func() {
		rhobsAPIURL = os.Getenv("E2E_RHOBS_API_URL")
		if rhobsAPIURL == "" {
			Skip("E2E_RHOBS_API_URL not set — skipping observability tests")
		}
		rhobsClient = NewAPIClient(rhobsAPIURL)
	})

	It("should have RC CloudWatch metrics in Thanos", func() {
		query := `count(aws_eks_apiserver_storage_size_bytes_maximum{cluster_type="regional-cluster"}) > 0`
		Eventually(func() bool {
			resp := queryThanos(rhobsClient, query)
			return resp.Status == "success" && len(resp.Data.Result) > 0
		}, "10m", "15s").Should(BeTrue(),
			"Expected CloudWatch EKS metrics with cluster_type=regional-cluster in Thanos "+
				"(CW Exporter → RC Prometheus → Thanos Receive)")
	})

	It("should have MC CloudWatch metrics in Thanos via remote-write", func() {
		query := `count(aws_eks_apiserver_storage_size_bytes_maximum{cluster_type="management-cluster"}) > 0`
		Eventually(func() bool {
			resp := queryThanos(rhobsClient, query)
			return resp.Status == "success" && len(resp.Data.Result) > 0
		}, "10m", "15s").Should(BeTrue(),
			"Expected CloudWatch EKS metrics with cluster_type=management-cluster in Thanos "+
				"(CW Exporter → MC Prometheus → remote-write → RHOBS API GW → Thanos Receive)")
	})
})

func queryThanos(client *APIClient, promql string) thanosQueryResponse {
	path := fmt.Sprintf("/api/v1/query?query=%s", url.QueryEscape(promql))
	resp, err := client.Get(path, "")
	if err != nil {
		GinkgoWriter.Printf("Thanos query error: %v\n", err)
		return thanosQueryResponse{}
	}
	if resp.StatusCode != http.StatusOK {
		GinkgoWriter.Printf("Thanos query returned %d: %s\n", resp.StatusCode, string(resp.Body))
		return thanosQueryResponse{}
	}

	var result thanosQueryResponse
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		GinkgoWriter.Printf("Failed to parse Thanos response: %v\n", err)
		return thanosQueryResponse{}
	}

	GinkgoWriter.Printf("Thanos query: %s → %d results\n", promql, len(result.Data.Result))
	return result
}
