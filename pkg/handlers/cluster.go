package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/openshift/rosa-regional-platform-api/pkg/clients/hyperfleet"
	"github.com/openshift/rosa-regional-platform-api/pkg/clients/maestro"
	"github.com/openshift/rosa-regional-platform-api/pkg/middleware"
)

// ClusterHandler handles cluster-related HTTP requests
type ClusterHandler struct {
	maestroClient    *maestro.Client
	hyperfleetClient *hyperfleet.Client
	logger           *slog.Logger
}

// NewClusterHandler creates a new cluster handler
func NewClusterHandler(maestroClient *maestro.Client, hyperfleetClient *hyperfleet.Client, logger *slog.Logger) *ClusterHandler {
	return &ClusterHandler{
		maestroClient:    maestroClient,
		hyperfleetClient: hyperfleetClient,
		logger:           logger,
	}
}

// List handles GET /api/v0/clusters
func (h *ClusterHandler) List(w http.ResponseWriter, r *http.Request) {
	h.proxyToHyperfleet(w, r, "/api/hyperfleet/v1/clusters")
}

// Create handles POST /api/v0/clusters
func (h *ClusterHandler) Create(w http.ResponseWriter, r *http.Request) {
	h.proxyToHyperfleet(w, r, "/api/hyperfleet/v1/clusters")
}

// Get handles GET /api/v0/clusters/{id}
func (h *ClusterHandler) Get(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	clusterID := vars["id"]
	h.proxyToHyperfleet(w, r, fmt.Sprintf("/api/hyperfleet/v1/clusters/%s", clusterID))
}

// Update handles PUT /api/v0/clusters/{id}
func (h *ClusterHandler) Update(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	clusterID := vars["id"]
	h.proxyToHyperfleet(w, r, fmt.Sprintf("/api/hyperfleet/v1/clusters/%s", clusterID))
}

// Delete handles DELETE /api/v0/clusters/{id}
func (h *ClusterHandler) Delete(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	clusterID := vars["id"]
	h.proxyToHyperfleet(w, r, fmt.Sprintf("/api/hyperfleet/v1/clusters/%s", clusterID))
}

// GetStatus handles GET /api/v0/clusters/{id}/statuses
func (h *ClusterHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	clusterID := vars["id"]
	h.proxyToHyperfleet(w, r, fmt.Sprintf("/api/hyperfleet/v1/clusters/%s/statuses", clusterID))
}

// Helper methods

// proxyToHyperfleet proxies the request to the Hyperfleet API
func (h *ClusterHandler) proxyToHyperfleet(w http.ResponseWriter, r *http.Request, path string) {
	ctx := r.Context()
	accountID := middleware.GetAccountID(ctx)

	h.logger.Info("proxying cluster request to hyperfleet", "method", r.Method, "path", path, "account_id", accountID)

	// Proxy the request to Hyperfleet
	resp, err := h.hyperfleetClient.ProxyRequest(ctx, r.Method, path, r.Body, r.URL.Query(), r.Header)
	if err != nil {
		h.logger.Error("failed to proxy request to hyperfleet", "error", err, "path", path, "account_id", accountID)
		h.writeError(w, http.StatusBadGateway, "HYPERFLEET-PROXY-001", "Failed to communicate with Hyperfleet API")
		return
	}
	defer func() { _ = resp.Body.Close() }()

	// Build skip set for hop-by-hop headers
	hopByHopHeaders := map[string]bool{
		"Connection":          true,
		"Keep-Alive":          true,
		"Proxy-Authenticate":  true,
		"Proxy-Authorization": true,
		"Te":                  true,
		"Trailers":            true,
		"Transfer-Encoding":   true,
		"Upgrade":             true,
	}

	// Parse Connection header for additional hop-by-hop headers
	if connHeader := resp.Header.Get("Connection"); connHeader != "" {
		for _, token := range splitHeaderTokens(connHeader) {
			hopByHopHeaders[http.CanonicalHeaderKey(token)] = true
		}
	}

	// Copy response headers, skipping hop-by-hop headers
	for key, values := range resp.Header {
		if hopByHopHeaders[key] {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Copy status code
	w.WriteHeader(resp.StatusCode)

	// Copy response body
	if _, err := io.Copy(w, resp.Body); err != nil {
		h.logger.Error("failed to copy response body", "error", err, "path", path)
	}
}

func (h *ClusterHandler) writeError(w http.ResponseWriter, status int, code, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := map[string]interface{}{
		"kind":   "Error",
		"code":   code,
		"reason": reason,
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// splitHeaderTokens parses a comma-separated header value into individual tokens
func splitHeaderTokens(header string) []string {
	var tokens []string
	for _, token := range strings.Split(header, ",") {
		if trimmed := strings.TrimSpace(token); trimmed != "" {
			tokens = append(tokens, trimmed)
		}
	}
	return tokens
}
