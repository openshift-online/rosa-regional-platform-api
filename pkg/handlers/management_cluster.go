package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/openshift/rosa-regional-platform-api/pkg/clients/fleetdb"
	"github.com/openshift/rosa-regional-platform-api/pkg/middleware"
)

const (
	mcConfigMapName      = "hyperfleet-mc-config"
	mcConfigMapNamespace = "hyperfleet-system"
	mcConfigMapKey       = "clusters.yaml"
)

// ManagementCluster represents a registered management cluster.
type ManagementCluster struct {
	ID        string `json:"id" yaml:"id"`
	Region    string `json:"region" yaml:"region"`
	AccountID string `json:"accountId" yaml:"accountId"`
}

// ManagementClusterCreateRequest is the request body for creating an MC registration.
type ManagementClusterCreateRequest struct {
	ID        string `json:"id"`
	Region    string `json:"region"`
	AccountID string `json:"accountId"`
}

// ManagementClusterHandler handles management cluster endpoints
type ManagementClusterHandler struct {
	fleetDB *fleetdb.Client
	logger  *slog.Logger
}

// NewManagementClusterHandler creates a new ManagementClusterHandler
func NewManagementClusterHandler(fleetDB *fleetdb.Client, logger *slog.Logger) *ManagementClusterHandler {
	return &ManagementClusterHandler{
		fleetDB: fleetDB,
		logger:  logger,
	}
}

// Create handles POST /api/v0/management_clusters
func (h *ManagementClusterHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := middleware.GetAccountID(ctx)

	h.logger.Info("creating management cluster", "account_id", accountID)

	var req ManagementClusterCreateRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid-request", "Invalid request body")
			return
		}
	}

	if req.ID == "" {
		h.writeError(w, http.StatusBadRequest, "missing-id", "id is required")
		return
	}

	clusters, cm, err := h.loadMCConfig(ctx)
	if err != nil {
		h.logger.Error("failed to load MC config", "error", err)
		h.writeError(w, http.StatusInternalServerError, "config-error", "Failed to load management cluster config")
		return
	}

	for _, mc := range clusters {
		if mc.ID == req.ID {
			h.writeError(w, http.StatusConflict, "already-exists", "Management cluster already registered: "+req.ID)
			return
		}
	}

	mc := ManagementCluster{
		ID:        req.ID,
		Region:    req.Region,
		AccountID: req.AccountID,
	}
	clusters = append(clusters, mc)

	if err := h.saveMCConfig(ctx, cm, clusters); err != nil {
		h.logger.Error("failed to save MC config", "error", err)
		h.writeError(w, http.StatusInternalServerError, "config-error", "Failed to save management cluster config")
		return
	}

	h.logger.Info("management cluster created", "id", mc.ID, "account_id", accountID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(mc)
}

// List handles GET /api/v0/management_clusters
func (h *ManagementClusterHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := middleware.GetAccountID(ctx)

	h.logger.Debug("listing management clusters", "account_id", accountID)

	clusters, _, err := h.loadMCConfig(ctx)
	if err != nil {
		h.logger.Error("failed to load MC config", "error", err)
		h.writeError(w, http.StatusInternalServerError, "config-error", "Failed to load management cluster config")
		return
	}

	h.logger.Debug("management clusters listed", "total", len(clusters), "account_id", accountID)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"kind":  "ManagementClusterList",
		"items": clusters,
		"total": len(clusters),
	})
}

// Get handles GET /api/v0/management_clusters/{id}
func (h *ManagementClusterHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := middleware.GetAccountID(ctx)
	vars := mux.Vars(r)
	id := vars["id"]

	h.logger.Debug("getting management cluster", "id", id, "account_id", accountID)

	clusters, _, err := h.loadMCConfig(ctx)
	if err != nil {
		h.logger.Error("failed to load MC config", "error", err)
		h.writeError(w, http.StatusInternalServerError, "config-error", "Failed to load management cluster config")
		return
	}

	for _, mc := range clusters {
		if mc.ID == id {
			h.logger.Debug("management cluster retrieved", "id", mc.ID, "account_id", accountID)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(mc)
			return
		}
	}

	h.writeError(w, http.StatusNotFound, "not-found", "Management cluster not found")
}

func (h *ManagementClusterHandler) loadMCConfig(ctx context.Context) ([]ManagementCluster, *corev1.ConfigMap, error) {
	var cm corev1.ConfigMap
	key := client.ObjectKey{Namespace: mcConfigMapNamespace, Name: mcConfigMapName}
	if err := h.fleetDB.Get(ctx, key, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("get mc configmap: %w", err)
	}

	data, ok := cm.Data[mcConfigMapKey]
	if !ok || data == "" {
		return nil, &cm, nil
	}

	var clusters []ManagementCluster
	if err := yaml.Unmarshal([]byte(data), &clusters); err != nil {
		return nil, &cm, fmt.Errorf("parse mc config: %w", err)
	}

	return clusters, &cm, nil
}

func (h *ManagementClusterHandler) saveMCConfig(ctx context.Context, existing *corev1.ConfigMap, clusters []ManagementCluster) error {
	yamlData, err := yaml.Marshal(clusters)
	if err != nil {
		return fmt.Errorf("marshal mc config: %w", err)
	}

	if existing == nil {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mcConfigMapName,
				Namespace: mcConfigMapNamespace,
			},
			Data: map[string]string{
				mcConfigMapKey: string(yamlData),
			},
		}
		return h.fleetDB.Create(ctx, cm)
	}

	existing.Data[mcConfigMapKey] = string(yamlData)
	return h.fleetDB.Update(ctx, existing)
}

func (h *ManagementClusterHandler) writeError(w http.ResponseWriter, status int, code, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	resp := map[string]interface{}{
		"kind":   "Error",
		"code":   code,
		"reason": reason,
	}

	_ = json.NewEncoder(w).Encode(resp)
}
