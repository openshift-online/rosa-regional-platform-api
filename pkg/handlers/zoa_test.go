package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/rosa-regional-platform-api/pkg/clients/maestro"
	"github.com/openshift/rosa-regional-platform-api/pkg/middleware"
	"github.com/openshift/rosa-regional-platform-api/pkg/zoa"
	workv1 "open-cluster-management.io/api/work/v1"
)

type mockExecutionStore struct {
	createFunc               func(ctx context.Context, exec *zoa.Execution) error
	getFunc                  func(ctx context.Context, executionID string) (*zoa.Execution, error)
	listFunc                 func(ctx context.Context, accountID string, limit int, filter *zoa.ListFilter) ([]*zoa.Execution, error)
	updateStatusFunc         func(ctx context.Context, executionID string, status zoa.ExecutionStatus, completedAt string, duration int) error
	updateTACompletionFunc   func(ctx context.Context, executionID string, taCompletedAt string, taDuration int) error
	updateCompletionFunc     func(ctx context.Context, executionID string, status zoa.ExecutionStatus, completedAt string, duration int, outputStatus zoa.OutputStatus, taCompletedAt string, taDuration int) error
	updateManifestWorkFunc   func(ctx context.Context, executionID, mwName string) error
	listPendingFunc          func(ctx context.Context) ([]*zoa.Execution, error)
}

func (m *mockExecutionStore) Create(ctx context.Context, exec *zoa.Execution) error {
	if m.createFunc != nil {
		return m.createFunc(ctx, exec)
	}
	return nil
}

func (m *mockExecutionStore) Get(ctx context.Context, executionID string) (*zoa.Execution, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, executionID)
	}
	return nil, nil
}

func (m *mockExecutionStore) List(ctx context.Context, accountID string, limit int, filter *zoa.ListFilter) ([]*zoa.Execution, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, accountID, limit, filter)
	}
	return nil, nil
}

func (m *mockExecutionStore) UpdateStatus(ctx context.Context, executionID string, status zoa.ExecutionStatus, completedAt string, duration int) error {
	if m.updateStatusFunc != nil {
		return m.updateStatusFunc(ctx, executionID, status, completedAt, duration)
	}
	return nil
}

func (m *mockExecutionStore) UpdateTACompletion(ctx context.Context, executionID string, taCompletedAt string, taDuration int) error {
	if m.updateTACompletionFunc != nil {
		return m.updateTACompletionFunc(ctx, executionID, taCompletedAt, taDuration)
	}
	return nil
}

func (m *mockExecutionStore) UpdateCompletion(ctx context.Context, executionID string, status zoa.ExecutionStatus, completedAt string, duration int, outputStatus zoa.OutputStatus, taCompletedAt string, taDuration int) error {
	if m.updateCompletionFunc != nil {
		return m.updateCompletionFunc(ctx, executionID, status, completedAt, duration, outputStatus, taCompletedAt, taDuration)
	}
	return nil
}

func (m *mockExecutionStore) UpdateManifestWorkName(ctx context.Context, executionID, mwName string) error {
	if m.updateManifestWorkFunc != nil {
		return m.updateManifestWorkFunc(ctx, executionID, mwName)
	}
	return nil
}

func (m *mockExecutionStore) ListPending(ctx context.Context) ([]*zoa.Execution, error) {
	if m.listPendingFunc != nil {
		return m.listPendingFunc(ctx)
	}
	return nil, nil
}

type zoaMockMaestroClient struct {
	createManifestWorkFunc func(ctx context.Context, clusterName string, mw *workv1.ManifestWork) (*workv1.ManifestWork, error)
}

func (m *zoaMockMaestroClient) CreateConsumer(ctx context.Context, req *maestro.ConsumerCreateRequest) (*maestro.Consumer, error) {
	return nil, nil
}

func (m *zoaMockMaestroClient) ListConsumers(ctx context.Context, page, size int) (*maestro.ConsumerList, error) {
	return nil, nil
}

func (m *zoaMockMaestroClient) GetConsumer(ctx context.Context, id string) (*maestro.Consumer, error) {
	return nil, nil
}

func (m *zoaMockMaestroClient) ListResourceBundles(ctx context.Context, page, size int, search, orderBy, fields string) (*maestro.ResourceBundleList, error) {
	return nil, nil
}

func (m *zoaMockMaestroClient) GetResourceBundle(ctx context.Context, id string) (*maestro.ResourceBundle, error) {
	return nil, nil
}

func (m *zoaMockMaestroClient) GetManifestWork(ctx context.Context, clusterName string, name string) (*workv1.ManifestWork, error) {
	return nil, nil
}

func (m *zoaMockMaestroClient) DeleteResourceBundle(ctx context.Context, id string) error {
	return nil
}

func (m *zoaMockMaestroClient) DeleteManifestWork(ctx context.Context, clusterName string, name string) error {
	return nil
}

func (m *zoaMockMaestroClient) CreateManifestWork(ctx context.Context, clusterName string, mw *workv1.ManifestWork) (*workv1.ManifestWork, error) {
	if m.createManifestWorkFunc != nil {
		return m.createManifestWorkFunc(ctx, clusterName, mw)
	}
	result := mw.DeepCopy()
	result.Name = "zoa-test-work"
	return result, nil
}

type mockS3Client struct{}

func (m *mockS3Client) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return &s3.GetObjectOutput{
		Body: io.NopCloser(strings.NewReader(`{"summary": "test output"}`)),
	}, nil
}

func testZoaLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func testJobConfig() *zoa.JobConfig {
	return &zoa.JobConfig{
		Image:         "quay.io/test/zoa-tools:latest",
		CPURequest:    "100m",
		MemoryRequest: "128Mi",
		CPULimit:      "500m",
		MemoryLimit:   "512Mi",
		TTLSeconds:    3600,
		EntrypointScript: `#!/bin/bash
set -uo pipefail
/zoa/run.sh
`,
	}
}

func testTemplateRegistry(t *testing.T) *zoa.TemplateRegistry {
	t.Helper()
	dir := t.TempDir()
	templateContent := `name: get_nodes
scope: kube-api
type: read
description: List all nodes in the target cluster
params:
  - name: node_selector
    required: false
    default: ""
rbac:
  cluster_scoped: true
  rules:
    - apiGroups: [""]
      resources: ["nodes"]
      verbs: ["get", "list"]
script: |
  kubectl get nodes -o json > /artifacts/output.json
`
	err := os.WriteFile(dir+"/get_nodes.yaml", []byte(templateContent), 0644)
	require.NoError(t, err)

	registry := zoa.NewTemplateRegistry(testZoaLogger())
	err = registry.LoadFromDir(dir)
	require.NoError(t, err)
	return registry
}

func newTestZoaHandler(t *testing.T, store zoa.ExecutionStore, maestroClient *zoaMockMaestroClient) *ZoaHandler {
	t.Helper()
	return NewZoaHandler(store, testTemplateRegistry(t), maestroClient, &mockS3Client{}, ZoaConfig{
		BucketName: "test-bucket",
		JobConfig:  testJobConfig(),
	}, testZoaLogger())
}

func TestZoaHandler_Create_Success(t *testing.T) {
	store := &mockExecutionStore{}
	mc := &zoaMockMaestroClient{}
	handler := newTestZoaHandler(t, store, mc)

	body := `{"target_cluster": "mc01"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v0/trusted-actions/get_nodes/run", bytes.NewBufferString(body))
	req = mux.SetURLVars(req, map[string]string{"action": "get_nodes"})
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyAccountID, "111222333444"))
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyCallerARN, "arn:aws:iam::111222333444:user/test"))

	rr := httptest.NewRecorder()
	handler.Create(rr, req)

	assert.Equal(t, http.StatusAccepted, rr.Code)

	var resp zoa.Execution
	err := json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "get_nodes", resp.Action)
	assert.Equal(t, "mc01", resp.TargetCluster)
	assert.Equal(t, zoa.StatusPending, resp.Status)
	assert.Equal(t, "read", resp.Type)
	assert.Equal(t, "kube-api", resp.Scope)
	assert.Equal(t, "test", resp.Operator)
	assert.NotEmpty(t, resp.ExecutionID)
}

func TestZoaHandler_Create_UnknownAction(t *testing.T) {
	store := &mockExecutionStore{}
	mc := &zoaMockMaestroClient{}
	handler := newTestZoaHandler(t, store, mc)

	body := `{"target_cluster": "mc01"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v0/trusted-actions/nonexistent/run", bytes.NewBufferString(body))
	req = mux.SetURLVars(req, map[string]string{"action": "nonexistent"})
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyAccountID, "111222333444"))

	rr := httptest.NewRecorder()
	handler.Create(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestZoaHandler_Create_MissingTargetCluster(t *testing.T) {
	store := &mockExecutionStore{}
	mc := &zoaMockMaestroClient{}
	handler := newTestZoaHandler(t, store, mc)

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/v0/trusted-actions/get_nodes/run", bytes.NewBufferString(body))
	req = mux.SetURLVars(req, map[string]string{"action": "get_nodes"})
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyAccountID, "111222333444"))

	rr := httptest.NewRecorder()
	handler.Create(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestZoaHandler_Get_Found(t *testing.T) {
	store := &mockExecutionStore{
		getFunc: func(ctx context.Context, executionID string) (*zoa.Execution, error) {
			return &zoa.Execution{
				ExecutionID:   "exec-123",
				Action:        "get_nodes",
				Status:        zoa.StatusSucceeded,
				TargetCluster: "mc01",
				OutputPath:    "exec-123/output.json",
				OutputStatus:  zoa.OutputStatusUploaded,
			}, nil
		},
	}
	handler := newTestZoaHandler(t, store, &zoaMockMaestroClient{})

	req := httptest.NewRequest(http.MethodGet, "/api/v0/trusted-actions/runs/exec-123", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "exec-123"})

	rr := httptest.NewRecorder()
	handler.Get(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp zoa.ExecutionResponse
	err := json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "exec-123", resp.Execution.ExecutionID)
	assert.NotNil(t, resp.Output)
}

func TestZoaHandler_Get_NotFound(t *testing.T) {
	store := &mockExecutionStore{
		getFunc: func(ctx context.Context, executionID string) (*zoa.Execution, error) {
			return nil, nil
		},
	}
	handler := newTestZoaHandler(t, store, &zoaMockMaestroClient{})

	req := httptest.NewRequest(http.MethodGet, "/api/v0/trusted-actions/runs/nonexistent", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "nonexistent"})

	rr := httptest.NewRecorder()
	handler.Get(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestZoaHandler_List(t *testing.T) {
	store := &mockExecutionStore{
		listFunc: func(ctx context.Context, accountID string, limit int, filter *zoa.ListFilter) ([]*zoa.Execution, error) {
			return []*zoa.Execution{
				{ExecutionID: "exec-1", Action: "get_nodes", Status: zoa.StatusSucceeded},
				{ExecutionID: "exec-2", Action: "get_nodes", Status: zoa.StatusPending},
			}, nil
		},
	}
	handler := newTestZoaHandler(t, store, &zoaMockMaestroClient{})

	req := httptest.NewRequest(http.MethodGet, "/api/v0/trusted-actions/runs", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyAccountID, "111222333444"))

	rr := httptest.NewRecorder()
	handler.List(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp zoa.ExecutionList
	err := json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, 2, resp.Total)
	assert.Len(t, resp.Items, 2)
}

func TestZoaHandler_Describe(t *testing.T) {
	handler := newTestZoaHandler(t, &mockExecutionStore{}, &zoaMockMaestroClient{})

	req := httptest.NewRequest(http.MethodGet, "/api/v0/trusted-actions/get_nodes", nil)
	req = mux.SetURLVars(req, map[string]string{"action": "get_nodes"})

	rr := httptest.NewRecorder()
	handler.Describe(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp zoa.TADescribeResponse
	err := json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "get_nodes", resp.Name)
	assert.Equal(t, "read", resp.Type)
	assert.Equal(t, "kube-api", resp.Scope)
	assert.Equal(t, "List all nodes in the target cluster", resp.Description)
}

func TestZoaHandler_Catalog(t *testing.T) {
	handler := newTestZoaHandler(t, &mockExecutionStore{}, &zoaMockMaestroClient{})

	req := httptest.NewRequest(http.MethodGet, "/api/v0/trusted-actions", nil)

	rr := httptest.NewRecorder()
	handler.Catalog(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp map[string]interface{}
	err := json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, float64(1), resp["total"])
}
