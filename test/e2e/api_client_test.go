package e2e_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPatchMethodHeaders(t *testing.T) {
	// Create a test server that captures the request
	var capturedMethod string
	var capturedContentType string
	var capturedAccountID string
	var capturedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedContentType = r.Header.Get("Content-Type")
		capturedAccountID = r.Header.Get("X-Amz-Account-Id")

		// Decode body
		json.NewDecoder(r.Body).Decode(&capturedBody)

		// Return success
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	// Create API client pointing to test server
	client := NewAPIClient(server.URL)

	// Make a PATCH request
	body := map[string]interface{}{
		"spec": map[string]interface{}{
			"compute_replicas": 3,
		},
	}

	resp, err := client.Patch("/api/v0/clusters/test-id", body, "123456789012")
	if err != nil {
		t.Fatalf("PATCH request failed: %v", err)
	}

	// Verify response
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Verify method
	if capturedMethod != http.MethodPatch {
		t.Errorf("Expected method PATCH, got %s", capturedMethod)
	}

	// Verify Content-Type header
	if capturedContentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", capturedContentType)
	}

	// Verify Account ID header
	if capturedAccountID != "123456789012" {
		t.Errorf("Expected X-Amz-Account-Id 123456789012, got %s", capturedAccountID)
	}

	// Verify body was sent correctly
	if capturedBody == nil {
		t.Error("Body was not captured")
	} else {
		spec, ok := capturedBody["spec"].(map[string]interface{})
		if !ok {
			t.Error("spec not found in body")
		} else {
			replicas, ok := spec["compute_replicas"].(float64)
			if !ok || int(replicas) != 3 {
				t.Errorf("Expected compute_replicas=3, got %v", replicas)
			}
		}
	}
}
