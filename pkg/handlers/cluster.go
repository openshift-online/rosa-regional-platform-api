package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/openshift/rosa-regional-platform-api/pkg/clients/maestro"
	"github.com/openshift/rosa-regional-platform-api/pkg/middleware"
	"github.com/openshift/rosa-regional-platform-api/pkg/types"
)

// ClusterHandler handles cluster-related HTTP requests
type ClusterHandler struct {
	maestroClient *maestro.Client
	logger        *slog.Logger
}

// NewClusterHandler creates a new cluster handler
func NewClusterHandler(maestroClient *maestro.Client, logger *slog.Logger) *ClusterHandler {
	return &ClusterHandler{
		maestroClient: maestroClient,
		logger:        logger,
	}
}

// List handles GET /api/v0/clusters
func (h *ClusterHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := middleware.GetAccountID(ctx)

	// Parse query parameters
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	status := r.URL.Query().Get("status")

	limit := 50 // default
	offset := 0 // default

	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	h.logger.Info("listing clusters", "account_id", accountID, "limit", limit, "offset", offset, "status", status)

	// Call Maestro to list clusters
	clusters, total, err := h.maestroClient.ListClusters(ctx, accountID, limit, offset, status)
	if err != nil {
		h.logger.Error("failed to list clusters", "error", err, "account_id", accountID)
		h.writeError(w, http.StatusInternalServerError, "CLUSTERS-MGMT-LIST-001", "Failed to list clusters")
		return
	}

	response := map[string]interface{}{
		"clusters": clusters,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
	}

	h.writeJSON(w, http.StatusOK, response)
}

// Create handles POST /api/v0/clusters
func (h *ClusterHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := middleware.GetAccountID(ctx)
	userEmail := middleware.GetUserID(ctx) // May be empty if not provided

	var req types.ClusterCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "CLUSTERS-MGMT-CREATE-001", "Invalid request body")
		return
	}

	// Validate required fields
	if req.Name == "" || req.Spec == nil {
		h.writeError(w, http.StatusBadRequest, "CLUSTERS-MGMT-CREATE-002", "Missing required fields: name and spec")
		return
	}

	h.logger.Info("creating cluster", "account_id", accountID, "cluster_name", req.Name)

	cluster, err := h.maestroClient.CreateCluster(ctx, accountID, userEmail, &req)
	if err != nil {
		h.logger.Error("failed to create cluster", "error", err, "account_id", accountID)
		h.writeError(w, http.StatusInternalServerError, "CLUSTERS-MGMT-CREATE-003", "Failed to create cluster")
		return
	}

	h.writeJSON(w, http.StatusCreated, cluster)
}

// Get handles GET /api/v0/clusters/{id}
func (h *ClusterHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := middleware.GetAccountID(ctx)
	vars := mux.Vars(r)
	clusterID := vars["id"]

	h.logger.Info("getting cluster", "account_id", accountID, "cluster_id", clusterID)

	cluster, err := h.maestroClient.GetCluster(ctx, accountID, clusterID)
	if err != nil {
		if maestro.IsNotFound(err) {
			h.writeError(w, http.StatusNotFound, "CLUSTERS-MGMT-GET-001", "Cluster not found")
			return
		}
		h.logger.Error("failed to get cluster", "error", err, "account_id", accountID, "cluster_id", clusterID)
		h.writeError(w, http.StatusInternalServerError, "CLUSTERS-MGMT-GET-002", "Failed to get cluster")
		return
	}

	h.writeJSON(w, http.StatusOK, cluster)
}

// Update handles PUT /api/v0/clusters/{id}
func (h *ClusterHandler) Update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := middleware.GetAccountID(ctx)
	vars := mux.Vars(r)
	clusterID := vars["id"]

	var req types.ClusterUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "CLUSTERS-MGMT-UPDATE-001", "Invalid request body")
		return
	}

	if req.Spec == nil {
		h.writeError(w, http.StatusBadRequest, "CLUSTERS-MGMT-UPDATE-002", "Missing required field: spec")
		return
	}

	h.logger.Info("updating cluster", "account_id", accountID, "cluster_id", clusterID)

	cluster, err := h.maestroClient.UpdateCluster(ctx, accountID, clusterID, &req)
	if err != nil {
		if maestro.IsNotFound(err) {
			h.writeError(w, http.StatusNotFound, "CLUSTERS-MGMT-UPDATE-003", "Cluster not found")
			return
		}
		h.logger.Error("failed to update cluster", "error", err, "account_id", accountID, "cluster_id", clusterID)
		h.writeError(w, http.StatusInternalServerError, "CLUSTERS-MGMT-UPDATE-004", "Failed to update cluster")
		return
	}

	h.writeJSON(w, http.StatusOK, cluster)
}

// Delete handles DELETE /api/v0/clusters/{id}
func (h *ClusterHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := middleware.GetAccountID(ctx)
	vars := mux.Vars(r)
	clusterID := vars["id"]

	forceStr := r.URL.Query().Get("force")
	force := forceStr == "true"

	h.logger.Info("deleting cluster", "account_id", accountID, "cluster_id", clusterID, "force", force)

	err := h.maestroClient.DeleteCluster(ctx, accountID, clusterID, force)
	if err != nil {
		if maestro.IsNotFound(err) {
			h.writeError(w, http.StatusNotFound, "CLUSTERS-MGMT-DELETE-001", "Cluster not found")
			return
		}
		h.logger.Error("failed to delete cluster", "error", err, "account_id", accountID, "cluster_id", clusterID)
		h.writeError(w, http.StatusInternalServerError, "CLUSTERS-MGMT-DELETE-002", "Failed to delete cluster")
		return
	}

	response := map[string]interface{}{
		"message":    "Cluster deletion initiated",
		"cluster_id": clusterID,
	}

	h.writeJSON(w, http.StatusAccepted, response)
}

// GetStatus handles GET /api/v0/clusters/{id}/status
func (h *ClusterHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := middleware.GetAccountID(ctx)
	vars := mux.Vars(r)
	clusterID := vars["id"]

	h.logger.Info("getting cluster status", "account_id", accountID, "cluster_id", clusterID)

	status, err := h.maestroClient.GetClusterStatus(ctx, accountID, clusterID)
	if err != nil {
		if maestro.IsNotFound(err) {
			h.writeError(w, http.StatusNotFound, "CLUSTERS-MGMT-STATUS-001", "Cluster not found")
			return
		}
		h.logger.Error("failed to get cluster status", "error", err, "account_id", accountID, "cluster_id", clusterID)
		h.writeError(w, http.StatusInternalServerError, "CLUSTERS-MGMT-STATUS-002", "Failed to get cluster status")
		return
	}

	h.writeJSON(w, http.StatusOK, status)
}

// Helper methods
func (h *ClusterHandler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode response", "error", err)
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
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.Error("failed to encode error response", "error", err)
	}
}
