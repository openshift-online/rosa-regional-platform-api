package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

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

// GetStatus handles GET /api/v0/clusters/{id}/status
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
	resp, err := h.hyperfleetClient.ProxyRequest(ctx, r.Method, path, r.Body, r.URL.Query())
	if err != nil {
		h.logger.Error("failed to proxy request to hyperfleet", "error", err, "path", path, "account_id", accountID)
		h.writeError(w, http.StatusBadGateway, "HYPERFLEET-PROXY-001", "Failed to communicate with Hyperfleet API")
		return
	}
	defer func() { _ = resp.Body.Close() }()

	// Copy response headers
	for key, values := range resp.Header {
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

func (h *ClusterHandler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
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
