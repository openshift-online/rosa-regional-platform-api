package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/openshift/rosa-regional-platform-api/pkg/clients/fleetdb"
	"github.com/openshift/rosa-regional-platform-api/pkg/middleware"
	"github.com/openshift/rosa-regional-platform-api/pkg/types"
)

// ClusterHandler handles cluster-related HTTP requests
type ClusterHandler struct {
	fleetDB *fleetdb.Client
	logger  *slog.Logger
}

// NewClusterHandler creates a new cluster handler
func NewClusterHandler(fleetDB *fleetdb.Client, logger *slog.Logger) *ClusterHandler {
	return &ClusterHandler{
		fleetDB: fleetDB,
		logger:  logger,
	}
}

// List handles GET /api/v0/clusters
func (h *ClusterHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := middleware.GetAccountID(ctx)

	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := 50
	offset := 0

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

	h.logger.Info("listing clusters", "account_id", accountID, "limit", limit, "offset", offset)

	list, err := h.fleetDB.ListClusters(ctx, accountID)
	if err != nil {
		h.logger.Error("failed to list clusters", "error", err, "account_id", accountID)
		h.writeError(w, http.StatusInternalServerError, "CLUSTERS-MGMT-LIST-001", "Failed to list clusters")
		return
	}

	clusters := make([]*types.Cluster, 0, len(list.Items))
	for i := range list.Items {
		clusters = append(clusters, fleetdb.ClusterCRToPlatform(&list.Items[i]))
	}

	total := len(clusters)

	// Apply offset/limit pagination in-memory.
	if offset >= len(clusters) {
		clusters = nil
	} else {
		end := offset + limit
		if end > len(clusters) {
			end = len(clusters)
		}
		clusters = clusters[offset:end]
	}

	response := map[string]interface{}{
		"items":  clusters,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	}

	h.writeJSON(w, http.StatusOK, response)
}

// Create handles POST /api/v0/clusters
func (h *ClusterHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := middleware.GetAccountID(ctx)

	var req types.ClusterCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "CLUSTERS-MGMT-CREATE-001", "Invalid request body")
		return
	}

	if req.Name == "" || req.Spec == nil {
		h.writeError(w, http.StatusBadRequest, "CLUSTERS-MGMT-CREATE-002", "Missing required fields: name and spec")
		return
	}

	existing, err := h.fleetDB.ListClusters(ctx, accountID)
	if err != nil {
		h.logger.Error("failed to check cluster name uniqueness", "error", err, "account_id", accountID)
		h.writeError(w, http.StatusInternalServerError, "CLUSTERS-MGMT-CREATE-004", "Failed to validate cluster name")
		return
	}
	for i := range existing.Items {
		if existing.Items[i].Spec.Name == req.Name {
			h.writeError(w, http.StatusConflict, "CLUSTERS-MGMT-CREATE-005",
				fmt.Sprintf("A cluster named %q already exists in this account", req.Name))
			return
		}
	}

	if callerARN := middleware.GetCallerARN(ctx); callerARN != "" {
		req.Spec["creatorARN"] = callerARN
	}

	clusterID := uuid.New().String()

	h.logger.Info("creating cluster", "account_id", accountID, "cluster_name", req.Name, "cluster_id", clusterID)

	cr, err := fleetdb.PlatformCreateToClusterCR(clusterID, accountID, &req)
	if err != nil {
		h.logger.Error("failed to convert cluster spec", "error", err, "account_id", accountID)
		h.writeError(w, http.StatusBadRequest, "CLUSTERS-MGMT-CREATE-002", "Invalid cluster spec")
		return
	}

	if err := h.fleetDB.CreateCluster(ctx, accountID, cr); err != nil {
		h.logger.Error("failed to create cluster", "error", err, "account_id", accountID)
		if fleetdb.IsAlreadyExists(err) {
			h.writeError(w, http.StatusConflict, "CLUSTERS-MGMT-CREATE-003", "Cluster already exists")
			return
		}
		h.writeError(w, http.StatusInternalServerError, "CLUSTERS-MGMT-CREATE-003", "Failed to create cluster")
		return
	}

	cluster := fleetdb.ClusterCRToPlatform(cr)
	h.writeJSON(w, http.StatusCreated, cluster)
}

// Get handles GET /api/v0/clusters/{id}
func (h *ClusterHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := middleware.GetAccountID(ctx)
	vars := mux.Vars(r)
	clusterID := vars["id"]

	h.logger.Info("getting cluster", "account_id", accountID, "cluster_id", clusterID)

	cr, err := h.fleetDB.GetCluster(ctx, accountID, clusterID)
	if err != nil {
		if fleetdb.IsNotFound(err) {
			h.writeError(w, http.StatusNotFound, "CLUSTERS-MGMT-GET-001", "Cluster not found")
			return
		}
		h.logger.Error("failed to get cluster", "error", err, "account_id", accountID, "cluster_id", clusterID)
		h.writeError(w, http.StatusInternalServerError, "CLUSTERS-MGMT-GET-002", "Failed to get cluster")
		return
	}

	h.writeJSON(w, http.StatusOK, fleetdb.ClusterCRToPlatform(cr))
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

	cr, err := h.fleetDB.GetCluster(ctx, accountID, clusterID)
	if err != nil {
		if fleetdb.IsNotFound(err) {
			h.writeError(w, http.StatusNotFound, "CLUSTERS-MGMT-UPDATE-003", "Cluster not found")
			return
		}
		h.logger.Error("failed to get cluster for update", "error", err, "account_id", accountID, "cluster_id", clusterID)
		h.writeError(w, http.StatusInternalServerError, "CLUSTERS-MGMT-UPDATE-004", "Failed to update cluster")
		return
	}

	if err := fleetdb.ApplyPlatformUpdateToClusterCR(cr, &req); err != nil {
		h.logger.Error("failed to merge cluster spec", "error", err)
		h.writeError(w, http.StatusBadRequest, "CLUSTERS-MGMT-UPDATE-002", "Invalid cluster spec")
		return
	}

	if err := h.fleetDB.UpdateCluster(ctx, cr); err != nil {
		h.logger.Error("failed to update cluster", "error", err, "account_id", accountID, "cluster_id", clusterID)
		h.writeError(w, http.StatusInternalServerError, "CLUSTERS-MGMT-UPDATE-004", "Failed to update cluster")
		return
	}

	h.writeJSON(w, http.StatusOK, fleetdb.ClusterCRToPlatform(cr))
}

// Delete handles DELETE /api/v0/clusters/{id}
func (h *ClusterHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := middleware.GetAccountID(ctx)
	vars := mux.Vars(r)
	clusterID := vars["id"]

	h.logger.Info("deleting cluster", "account_id", accountID, "cluster_id", clusterID)

	err := h.fleetDB.DeleteCluster(ctx, accountID, clusterID)
	if err != nil {
		if fleetdb.IsNotFound(err) {
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

// GetStatus handles GET /api/v0/clusters/{id}/statuses
func (h *ClusterHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := middleware.GetAccountID(ctx)
	vars := mux.Vars(r)
	clusterID := vars["id"]

	h.logger.Info("getting cluster status", "account_id", accountID, "cluster_id", clusterID)

	cr, err := h.fleetDB.GetCluster(ctx, accountID, clusterID)
	if err != nil {
		if fleetdb.IsNotFound(err) {
			h.writeError(w, http.StatusNotFound, "CLUSTERS-MGMT-STATUS-001", "Cluster not found")
			return
		}
		h.logger.Error("failed to get cluster status", "error", err, "account_id", accountID, "cluster_id", clusterID)
		h.writeError(w, http.StatusInternalServerError, "CLUSTERS-MGMT-STATUS-002", "Failed to get cluster status")
		return
	}

	h.writeJSON(w, http.StatusOK, fleetdb.ClusterStatusFromCR(cr))
}

// Helper methods
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
