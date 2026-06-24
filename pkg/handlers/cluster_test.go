package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gorilla/mux"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	hyperfleetv1alpha1 "github.com/typeid/hyperfleet-operator/api/v1alpha1"

	"github.com/openshift/rosa-regional-platform-api/pkg/clients/fleetdb"
	"github.com/openshift/rosa-regional-platform-api/pkg/middleware"
)

const testAccountID = "123456789012"

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = hyperfleetv1alpha1.AddToScheme(s)
	return s
}

func testContext(accountID string) context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, middleware.ContextKeyAccountID, accountID)
	ctx = context.WithValue(ctx, middleware.ContextKeyCallerARN, "arn:aws:iam::"+accountID+":user/test")
	return ctx
}

func testClusterCR(name, accountID string) *hyperfleetv1alpha1.Cluster {
	return &hyperfleetv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: accountID,
		},
		Spec: hyperfleetv1alpha1.ClusterSpec{
			Name:      "test-cluster",
			AccountID: accountID,
			Region:    "us-east-1",
		},
	}
}

func TestClusterHandler_List_Success(t *testing.T) {
	scheme := newTestScheme()
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		testClusterCR("cluster-1", testAccountID),
		testClusterCR("cluster-2", testAccountID),
	).Build()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	handler := NewClusterHandler(fleetdb.NewClientFrom(fc, logger), logger)

	req := httptest.NewRequest(http.MethodGet, "/api/v0/clusters", nil)
	req = req.WithContext(testContext(testAccountID))

	w := httptest.NewRecorder()
	handler.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&result)

	if int(result["total"].(float64)) != 2 {
		t.Errorf("expected total=2, got %v", result["total"])
	}
	items := result["items"].([]interface{})
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestClusterHandler_List_Empty(t *testing.T) {
	scheme := newTestScheme()
	fc := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	handler := NewClusterHandler(fleetdb.NewClientFrom(fc, logger), logger)

	req := httptest.NewRequest(http.MethodGet, "/api/v0/clusters", nil)
	req = req.WithContext(testContext(testAccountID))

	w := httptest.NewRecorder()
	handler.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&result)

	if int(result["total"].(float64)) != 0 {
		t.Errorf("expected total=0, got %v", result["total"])
	}
}

func TestClusterHandler_List_Pagination(t *testing.T) {
	scheme := newTestScheme()
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		testClusterCR("c1", testAccountID),
		testClusterCR("c2", testAccountID),
		testClusterCR("c3", testAccountID),
	).Build()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	handler := NewClusterHandler(fleetdb.NewClientFrom(fc, logger), logger)

	req := httptest.NewRequest(http.MethodGet, "/api/v0/clusters?limit=2&offset=1", nil)
	req = req.WithContext(testContext(testAccountID))

	w := httptest.NewRecorder()
	handler.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&result)

	if int(result["total"].(float64)) != 3 {
		t.Errorf("expected total=3, got %v", result["total"])
	}
	items := result["items"].([]interface{})
	if len(items) != 2 {
		t.Errorf("expected 2 items (offset=1, limit=2 of 3), got %d", len(items))
	}
}

func TestClusterHandler_Create_Success(t *testing.T) {
	scheme := newTestScheme()
	fc := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	handler := NewClusterHandler(fleetdb.NewClientFrom(fc, logger), logger)

	body, _ := json.Marshal(map[string]interface{}{
		"name": "my-cluster",
		"spec": map[string]interface{}{
			"platform": map[string]interface{}{
				"aws": map[string]interface{}{
					"region": "us-east-1",
				},
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v0/clusters", bytes.NewReader(body))
	req = req.WithContext(testContext(testAccountID))

	w := httptest.NewRecorder()
	handler.Create(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&result)

	if result["id"] == nil || result["id"] == "" {
		t.Error("expected non-empty cluster ID")
	}
	if result["name"] != "my-cluster" {
		t.Errorf("expected name=my-cluster, got %v", result["name"])
	}
}

func TestClusterHandler_Create_SetsCreatorARN(t *testing.T) {
	scheme := newTestScheme()
	fc := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	handler := NewClusterHandler(fleetdb.NewClientFrom(fc, logger), logger)

	body, _ := json.Marshal(map[string]interface{}{
		"name": "my-cluster",
		"spec": map[string]interface{}{},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v0/clusters", bytes.NewReader(body))
	req = req.WithContext(testContext(testAccountID))

	w := httptest.NewRecorder()
	handler.Create(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&result)

	if result["created_by"] != "arn:aws:iam::"+testAccountID+":user/test" {
		t.Errorf("expected creatorARN in created_by, got %v", result["created_by"])
	}
}

func TestClusterHandler_Create_InvalidJSON(t *testing.T) {
	scheme := newTestScheme()
	fc := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	handler := NewClusterHandler(fleetdb.NewClientFrom(fc, logger), logger)

	req := httptest.NewRequest(http.MethodPost, "/api/v0/clusters", bytes.NewReader([]byte("not json")))
	req = req.WithContext(testContext(testAccountID))

	w := httptest.NewRecorder()
	handler.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestClusterHandler_Create_MissingFields(t *testing.T) {
	tests := []struct {
		name string
		body map[string]interface{}
	}{
		{"missing name", map[string]interface{}{"spec": map[string]interface{}{}}},
		{"missing spec", map[string]interface{}{"name": "test"}},
		{"empty name", map[string]interface{}{"name": "", "spec": map[string]interface{}{}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := newTestScheme()
			fc := fake.NewClientBuilder().WithScheme(scheme).Build()
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			handler := NewClusterHandler(fleetdb.NewClientFrom(fc, logger), logger)

			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/v0/clusters", bytes.NewReader(body))
			req = req.WithContext(testContext(testAccountID))

			w := httptest.NewRecorder()
			handler.Create(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", w.Code)
			}
		})
	}
}

func TestClusterHandler_Get_Success(t *testing.T) {
	scheme := newTestScheme()
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		testClusterCR("cluster-123", testAccountID),
	).Build()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	handler := NewClusterHandler(fleetdb.NewClientFrom(fc, logger), logger)

	req := httptest.NewRequest(http.MethodGet, "/api/v0/clusters/cluster-123", nil)
	req = req.WithContext(testContext(testAccountID))
	req = mux.SetURLVars(req, map[string]string{"id": "cluster-123"})

	w := httptest.NewRecorder()
	handler.Get(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&result)

	if result["id"] != "cluster-123" {
		t.Errorf("expected id=cluster-123, got %v", result["id"])
	}
	if result["name"] != "test-cluster" {
		t.Errorf("expected name=test-cluster, got %v", result["name"])
	}
}

func TestClusterHandler_Get_NotFound(t *testing.T) {
	scheme := newTestScheme()
	fc := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	handler := NewClusterHandler(fleetdb.NewClientFrom(fc, logger), logger)

	req := httptest.NewRequest(http.MethodGet, "/api/v0/clusters/no-such-cluster", nil)
	req = req.WithContext(testContext(testAccountID))
	req = mux.SetURLVars(req, map[string]string{"id": "no-such-cluster"})

	w := httptest.NewRecorder()
	handler.Get(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	var errResp map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp["code"] != "CLUSTERS-MGMT-GET-001" {
		t.Errorf("expected code CLUSTERS-MGMT-GET-001, got %v", errResp["code"])
	}
}

func TestClusterHandler_Delete_Success(t *testing.T) {
	scheme := newTestScheme()
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		testClusterCR("cluster-123", testAccountID),
	).Build()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	handler := NewClusterHandler(fleetdb.NewClientFrom(fc, logger), logger)

	req := httptest.NewRequest(http.MethodDelete, "/api/v0/clusters/cluster-123", nil)
	req = req.WithContext(testContext(testAccountID))
	req = mux.SetURLVars(req, map[string]string{"id": "cluster-123"})

	w := httptest.NewRecorder()
	handler.Delete(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&result)
	if result["cluster_id"] != "cluster-123" {
		t.Errorf("expected cluster_id=cluster-123, got %v", result["cluster_id"])
	}
}

func TestClusterHandler_Delete_NotFound(t *testing.T) {
	scheme := newTestScheme()
	fc := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	handler := NewClusterHandler(fleetdb.NewClientFrom(fc, logger), logger)

	req := httptest.NewRequest(http.MethodDelete, "/api/v0/clusters/no-such-cluster", nil)
	req = req.WithContext(testContext(testAccountID))
	req = mux.SetURLVars(req, map[string]string{"id": "no-such-cluster"})

	w := httptest.NewRecorder()
	handler.Delete(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestClusterHandler_GetStatus_Success(t *testing.T) {
	cr := testClusterCR("cluster-123", testAccountID)
	cr.Status = hyperfleetv1alpha1.ClusterStatus{
		ObservedGeneration: 1,
		Phase:              "Ready",
		Conditions: []metav1.Condition{
			{
				Type:   "Ready",
				Status: metav1.ConditionTrue,
				Reason: "ClusterReady",
			},
		},
	}

	scheme := newTestScheme()
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr).
		WithStatusSubresource(cr).Build()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	handler := NewClusterHandler(fleetdb.NewClientFrom(fc, logger), logger)

	req := httptest.NewRequest(http.MethodGet, "/api/v0/clusters/cluster-123/statuses", nil)
	req = req.WithContext(testContext(testAccountID))
	req = mux.SetURLVars(req, map[string]string{"id": "cluster-123"})

	w := httptest.NewRecorder()
	handler.GetStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&result)

	if result["cluster_id"] != "cluster-123" {
		t.Errorf("expected cluster_id=cluster-123, got %v", result["cluster_id"])
	}
}

func TestClusterHandler_GetStatus_NotFound(t *testing.T) {
	scheme := newTestScheme()
	fc := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	handler := NewClusterHandler(fleetdb.NewClientFrom(fc, logger), logger)

	req := httptest.NewRequest(http.MethodGet, "/api/v0/clusters/no-such/statuses", nil)
	req = req.WithContext(testContext(testAccountID))
	req = mux.SetURLVars(req, map[string]string{"id": "no-such"})

	w := httptest.NewRecorder()
	handler.GetStatus(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestClusterHandler_Update_Success(t *testing.T) {
	scheme := newTestScheme()
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		testClusterCR("cluster-123", testAccountID),
	).Build()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	handler := NewClusterHandler(fleetdb.NewClientFrom(fc, logger), logger)

	body, _ := json.Marshal(map[string]interface{}{
		"spec": map[string]interface{}{
			"name": "updated-name",
		},
	})

	req := httptest.NewRequest(http.MethodPut, "/api/v0/clusters/cluster-123", bytes.NewReader(body))
	req = req.WithContext(testContext(testAccountID))
	req = mux.SetURLVars(req, map[string]string{"id": "cluster-123"})

	w := httptest.NewRecorder()
	handler.Update(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&result)

	if result["name"] != "updated-name" {
		t.Errorf("expected name=updated-name, got %v", result["name"])
	}
}

func TestClusterHandler_Update_NotFound(t *testing.T) {
	scheme := newTestScheme()
	fc := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	handler := NewClusterHandler(fleetdb.NewClientFrom(fc, logger), logger)

	body, _ := json.Marshal(map[string]interface{}{
		"spec": map[string]interface{}{"name": "x"},
	})

	req := httptest.NewRequest(http.MethodPut, "/api/v0/clusters/no-such", bytes.NewReader(body))
	req = req.WithContext(testContext(testAccountID))
	req = mux.SetURLVars(req, map[string]string{"id": "no-such"})

	w := httptest.NewRecorder()
	handler.Update(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestClusterHandler_Update_MissingSpec(t *testing.T) {
	scheme := newTestScheme()
	fc := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	handler := NewClusterHandler(fleetdb.NewClientFrom(fc, logger), logger)

	body, _ := json.Marshal(map[string]interface{}{})

	req := httptest.NewRequest(http.MethodPut, "/api/v0/clusters/cluster-123", bytes.NewReader(body))
	req = req.WithContext(testContext(testAccountID))
	req = mux.SetURLVars(req, map[string]string{"id": "cluster-123"})

	w := httptest.NewRecorder()
	handler.Update(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
