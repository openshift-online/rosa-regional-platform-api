package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/openshift/rosa-regional-platform-api/pkg/clients/maestro"
	"github.com/openshift/rosa-regional-platform-api/pkg/middleware"
	"github.com/openshift/rosa-regional-platform-api/pkg/zoa"
)

// ZoaHandler handles ZOA Trusted Action endpoints.
type ZoaHandler struct {
	store         zoa.ExecutionStore
	registry      *zoa.TemplateRegistry
	maestroClient maestro.ClientInterface
	s3Client      S3PresignClient
	bucketName    string
	jobRoleARN    string
	jobImage      string
	logger        *slog.Logger
}

// S3PresignClient is the interface for generating presigned S3 URLs.
type S3PresignClient interface {
	PresignGetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error)
}

// ZoaConfig holds configuration for the ZOA handler.
type ZoaConfig struct {
	BucketName string
	JobRoleARN string
	JobImage   string
}

// NewZoaHandler creates a new ZoaHandler.
func NewZoaHandler(
	store zoa.ExecutionStore,
	registry *zoa.TemplateRegistry,
	maestroClient maestro.ClientInterface,
	s3Client S3PresignClient,
	cfg ZoaConfig,
	logger *slog.Logger,
) *ZoaHandler {
	return &ZoaHandler{
		store:         store,
		registry:      registry,
		maestroClient: maestroClient,
		s3Client:      s3Client,
		bucketName:    cfg.BucketName,
		jobRoleARN:    cfg.JobRoleARN,
		jobImage:      cfg.JobImage,
		logger:        logger,
	}
}

// Create handles POST /api/v0/trusted_actions/{action}
func (h *ZoaHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := middleware.GetAccountID(ctx)
	callerARN := middleware.GetCallerARN(ctx)
	action := mux.Vars(r)["action"]

	tmpl, ok := h.registry.Get(action)
	if !ok {
		h.writeError(w, http.StatusNotFound, "unknown-action", "Trusted action not found: "+action)
		return
	}

	var req zoa.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid-request", "Invalid request body")
		return
	}

	if req.TargetCluster == "" {
		h.writeError(w, http.StatusBadRequest, "missing-target-cluster", "target_cluster is required")
		return
	}

	execID := uuid.New().String()

	exec := &zoa.Execution{
		ExecutionID:   execID,
		AccountID:     accountID,
		CallerARN:     callerARN,
		Action:        action,
		TargetCluster: req.TargetCluster,
		Scope:         tmpl.Scope,
		Status:        zoa.StatusPending,
		OutputPath:    execID + "/output.json",
	}

	if err := h.store.Create(ctx, exec); err != nil {
		h.logger.Error("failed to create execution record", "error", err, "execution_id", execID)
		h.writeError(w, http.StatusInternalServerError, "store-error", "Failed to create execution")
		return
	}

	renderCtx := zoa.RenderContext{
		ExecID:        execID,
		ActionName:    action,
		TargetCluster: req.TargetCluster,
		Namespace:     zoa.JobNamespace,
		OutputBucket:  h.bucketName,
		JobRoleARN:    h.jobRoleARN,
		Image:         h.jobImage,
		Params:        req.Params,
	}

	mw, err := tmpl.BuildManifestWork(renderCtx)
	if err != nil {
		h.logger.Error("failed to render manifestwork", "error", err, "execution_id", execID)
		_ = h.store.UpdateStatus(ctx, execID, zoa.StatusFailed, time.Now().UTC().Format(time.RFC3339), 0)
		h.writeError(w, http.StatusInternalServerError, "render-error", "Failed to render trusted action template")
		return
	}

	result, err := h.maestroClient.CreateManifestWork(ctx, req.TargetCluster, mw)
	if err != nil {
		h.logger.Error("failed to dispatch manifestwork", "error", err, "execution_id", execID)
		_ = h.store.UpdateStatus(ctx, execID, zoa.StatusFailed, time.Now().UTC().Format(time.RFC3339), 0)
		h.writeError(w, http.StatusBadGateway, "maestro-error", "Failed to dispatch trusted action")
		return
	}

	exec.ManifestWorkName = result.Name

	if err := h.store.UpdateManifestWorkName(ctx, execID, result.Name); err != nil {
		h.logger.Error("failed to update manifestwork name", "error", err, "execution_id", execID)
	}

	h.logger.Info("trusted action dispatched",
		"execution_id", execID,
		"action", action,
		"target_cluster", req.TargetCluster,
		"manifest_work", result.Name,
		"account_id", accountID,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(exec)
}

// Get handles GET /api/v0/trusted_actions/runs/{id}
func (h *ZoaHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	execID := mux.Vars(r)["id"]

	exec, err := h.store.Get(ctx, execID)
	if err != nil {
		h.logger.Error("failed to get execution", "error", err, "execution_id", execID)
		h.writeError(w, http.StatusInternalServerError, "store-error", "Failed to retrieve execution")
		return
	}

	if exec == nil {
		h.writeError(w, http.StatusNotFound, "not-found", "Execution not found")
		return
	}

	if exec.Status == zoa.StatusSucceeded && exec.OutputPath != "" {
		presignedURL, err := h.generatePresignedURL(ctx, exec.OutputPath)
		if err != nil {
			h.logger.Error("failed to generate presigned URL", "error", err, "execution_id", execID)
		} else {
			exec.OutputURL = presignedURL
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(exec)
}

// List handles GET /api/v0/trusted_actions/runs
func (h *ZoaHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := middleware.GetAccountID(ctx)

	executions, err := h.store.List(ctx, accountID, 50)
	if err != nil {
		h.logger.Error("failed to list executions", "error", err, "account_id", accountID)
		h.writeError(w, http.StatusInternalServerError, "store-error", "Failed to list executions")
		return
	}

	response := &zoa.ExecutionList{
		Kind:  "ExecutionList",
		Items: executions,
		Total: len(executions),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func (h *ZoaHandler) generatePresignedURL(ctx context.Context, key string) (string, error) {
	result, err := h.s3Client.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: &h.bucketName,
		Key:    &key,
	}, func(opts *s3.PresignOptions) {
		opts.Expires = 15 * time.Minute
	})
	if err != nil {
		return "", err
	}
	return result.URL, nil
}

func (h *ZoaHandler) writeError(w http.ResponseWriter, status int, code, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"kind":   "Error",
		"code":   code,
		"reason": reason,
	})
}
