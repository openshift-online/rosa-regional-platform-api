package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
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
	s3Client      S3Client
	bucketName    string
	jobConfig     *zoa.JobConfig
	logger        *slog.Logger
}

// S3Client provides operations for accessing S3 objects.
type S3Client interface {
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

// ZoaConfig holds configuration for the ZOA handler.
type ZoaConfig struct {
	BucketName string
	JobConfig  *zoa.JobConfig
}

// NewZoaHandler creates a new ZoaHandler.
func NewZoaHandler(
	store zoa.ExecutionStore,
	registry *zoa.TemplateRegistry,
	maestroClient maestro.ClientInterface,
	s3Client S3Client,
	cfg ZoaConfig,
	logger *slog.Logger,
) *ZoaHandler {
	return &ZoaHandler{
		store:         store,
		registry:      registry,
		maestroClient: maestroClient,
		s3Client:      s3Client,
		bucketName:    cfg.BucketName,
		jobConfig:     cfg.JobConfig,
		logger:        logger,
	}
}

// Create handles POST /api/v0/trusted-actions/{action}/run
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

	if err := validateParams(tmpl, req.Params); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid-params", err.Error())
		return
	}

	execID := uuid.New().String()
	operator := extractOperator(callerARN)

	exec := &zoa.Execution{
		ExecutionID:   execID,
		AccountID:     accountID,
		CallerARN:     callerARN,
		Operator:      operator,
		Action:        action,
		TargetCluster: req.TargetCluster,
		Scope:         tmpl.Scope,
		Type:          tmpl.Type,
		Revision:      h.jobConfig.Revision,
		Status:        zoa.StatusPending,
		OutputStatus:  zoa.OutputStatusPending,
		OutputPath:    "s3://" + h.bucketName + "/" + execID + "/output.json",
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
		Operator:      operator,
		Revision:      h.jobConfig.Revision,
		Type:          tmpl.Type,
		Scope:         tmpl.Scope,
		Params:        req.Params,
		Config:        *h.jobConfig,
	}

	mw, err := zoa.BuildManifestWork(tmpl, renderCtx)
	if err != nil {
		h.logger.Error("failed to build manifestwork", "error", err, "execution_id", execID)
		_ = h.store.UpdateStatus(ctx, execID, zoa.StatusFailed, time.Now().UTC().Format(time.RFC3339), 0)
		h.writeError(w, http.StatusInternalServerError, "render-error", "Failed to build trusted action manifest")
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
		"operator", operator,
		"scope", tmpl.Scope,
		"type", tmpl.Type,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(exec)
}

// Get handles GET /api/v0/trusted-actions/runs/{id}
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

	fields := parseFields(r.URL.Query().Get("fields"))

	response := &zoa.ExecutionResponse{
		Execution: exec,
	}

	if exec.Status == zoa.StatusSucceeded || exec.Status == zoa.StatusFailed || exec.Status == zoa.StatusTimedOut {
		if exec.OutputStatus == zoa.OutputStatusUploaded {
			if fields.includeOutput {
				outputURI := exec.OutputPath
				if outputURI == "" {
					outputURI = exec.ExecutionID + "/output.json"
				}
				output, err := h.fetchS3Content(ctx, outputURI)
				if err != nil {
					h.logger.Error("failed to fetch output from S3", "error", err, "uri", outputURI)
				} else if output != nil {
					var parsed interface{}
					if json.Unmarshal(output, &parsed) == nil {
						response.Output = parsed
					} else {
						response.Output = string(output)
					}
				}
			}

			if fields.includeLogs {
				logsURI := strings.Replace(exec.OutputPath, "/output.json", "/execution.log", 1)
				if exec.OutputPath == "" {
					logsURI = exec.ExecutionID + "/execution.log"
				}
				logs, err := h.fetchS3Content(ctx, logsURI)
				if err != nil {
					h.logger.Error("failed to fetch logs from S3", "error", err, "uri", logsURI)
				}
				response.Logs = string(logs)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

// List handles GET /api/v0/trusted-actions/runs
func (h *ZoaHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := middleware.GetAccountID(ctx)
	query := r.URL.Query()

	limit := 20
	if v := query.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	filter := &zoa.ListFilter{
		Status:        query.Get("status"),
		Action:        query.Get("action"),
		TargetCluster: query.Get("target"),
		Operator:      query.Get("operator"),
		Scope:         query.Get("scope"),
		Type:          query.Get("type"),
	}

	if since := query.Get("since"); since != "" {
		if ts, err := parseSince(since); err == nil {
			filter.Since = ts
		}
	}

	executions, err := h.store.List(ctx, accountID, limit, filter)
	if err != nil {
		h.logger.Error("failed to list executions", "error", err, "account_id", accountID)
		h.writeError(w, http.StatusInternalServerError, "store-error", "Failed to list executions")
		return
	}

	response := &zoa.ExecutionList{
		Items:   executions,
		Total:   len(executions),
		Page:    1,
		Limit:   limit,
		HasMore: len(executions) >= limit,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

// parseSince converts a duration shorthand (e.g. "1h", "24h", "7d") or RFC3339 timestamp to an RFC3339 string.
func parseSince(s string) (string, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format(time.RFC3339), nil
	}

	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return "", fmt.Errorf("invalid since value: %s", s)
	}

	unit := s[len(s)-1]
	numStr := s[:len(s)-1]
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return "", fmt.Errorf("invalid since value: %s", s)
	}

	var d time.Duration
	switch unit {
	case 's':
		d = time.Duration(num) * time.Second
	case 'm':
		d = time.Duration(num) * time.Minute
	case 'h':
		d = time.Duration(num) * time.Hour
	case 'd':
		d = time.Duration(num) * 24 * time.Hour
	default:
		return "", fmt.Errorf("invalid since unit: %c (use s, m, h, or d)", unit)
	}

	return time.Now().UTC().Add(-d).Format(time.RFC3339), nil
}

// Catalog handles GET /api/v0/trusted-actions
func (h *ZoaHandler) Catalog(w http.ResponseWriter, r *http.Request) {
	templates := h.registry.ListAll()

	items := make([]zoa.TADescribeResponse, 0, len(templates))
	for _, t := range templates {
		items = append(items, zoa.TADescribeResponse{
			Name:        t.Name,
			Scope:       t.Scope,
			Type:        t.Type,
			Description: t.Description,
			Params:      t.Params,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"items": items,
		"total": len(items),
	})
}

// Describe handles GET /api/v0/trusted-actions/{action}
func (h *ZoaHandler) Describe(w http.ResponseWriter, r *http.Request) {
	action := mux.Vars(r)["action"]

	tmpl, ok := h.registry.Get(action)
	if !ok {
		h.writeError(w, http.StatusNotFound, "unknown-action", "Trusted action not found: "+action)
		return
	}

	response := &zoa.TADescribeResponse{
		Name:        tmpl.Name,
		Scope:       tmpl.Scope,
		Type:        tmpl.Type,
		Description: tmpl.Description,
		Params:      tmpl.Params,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func (h *ZoaHandler) fetchS3Content(ctx context.Context, s3URI string) ([]byte, error) {
	bucket, key := parseS3URI(s3URI)
	if bucket == "" {
		bucket = h.bucketName
		key = s3URI
	}
	result, err := h.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})
	if err != nil {
		return nil, err
	}
	defer result.Body.Close()
	return io.ReadAll(result.Body)
}

func parseS3URI(uri string) (bucket, key string) {
	if !strings.HasPrefix(uri, "s3://") {
		return "", uri
	}
	path := strings.TrimPrefix(uri, "s3://")
	idx := strings.Index(path, "/")
	if idx < 0 {
		return path, ""
	}
	return path[:idx], path[idx+1:]
}

type fieldsSelection struct {
	includeOutput bool
	includeLogs   bool
}

func parseFields(raw string) fieldsSelection {
	if raw == "" {
		return fieldsSelection{includeOutput: true}
	}

	if raw == "all" {
		return fieldsSelection{includeOutput: true, includeLogs: true}
	}

	sel := fieldsSelection{}
	for _, f := range strings.Split(raw, ",") {
		switch strings.TrimSpace(f) {
		case "output":
			sel.includeOutput = true
		case "logs":
			sel.includeLogs = true
		}
	}

	return sel
}

func validateParams(tmpl *zoa.TATemplate, params map[string]string) error {
	for _, p := range tmpl.Params {
		if p.Required {
			val, ok := params[p.Name]
			if !ok || val == "" {
				return fmt.Errorf("required parameter '%s' is missing", p.Name)
			}
		}
	}

	if hasParamWithDefault(tmpl, "namespace", "") && hasParamWithDefault(tmpl, "all_namespaces", "false") {
		ns := params["namespace"]
		allNs := params["all_namespaces"]
		if ns == "" && allNs != "true" {
			return fmt.Errorf("specify namespace or set all_namespaces=true")
		}
	}

	return nil
}

func hasParamWithDefault(tmpl *zoa.TATemplate, name, defaultVal string) bool {
	for _, p := range tmpl.Params {
		if p.Name == name && p.Default == defaultVal {
			return true
		}
	}
	return false
}

func extractOperator(callerARN string) string {
	parts := strings.Split(callerARN, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return callerARN
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
