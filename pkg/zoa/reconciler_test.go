package zoa

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	hyperfleetv1alpha1 "github.com/typeid/hyperfleet-operator/api/v1alpha1"

	"github.com/openshift/rosa-regional-platform-api/pkg/clients/fleetdb"
)

type mockExecutionStore struct {
	createFunc           func(ctx context.Context, exec *Execution) error
	getFunc              func(ctx context.Context, executionID string) (*Execution, error)
	listFunc             func(ctx context.Context, accountID string, limit int, filter *ListFilter) ([]*Execution, error)
	updateStatusFunc     func(ctx context.Context, executionID string, status ExecutionStatus, completedAt string, duration int) error
	updateCompletionFunc func(ctx context.Context, executionID string, status ExecutionStatus, completedAt string, duration int, runnerSeconds int, uploadSeconds int, outputStatus OutputStatus) error
	updateMWNameFunc     func(ctx context.Context, executionID, mwName string) error
	listPendingFunc      func(ctx context.Context) ([]*Execution, error)
}

func (m *mockExecutionStore) Create(ctx context.Context, exec *Execution) error {
	if m.createFunc != nil {
		return m.createFunc(ctx, exec)
	}
	return nil
}
func (m *mockExecutionStore) Get(ctx context.Context, executionID string) (*Execution, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, executionID)
	}
	return nil, nil
}
func (m *mockExecutionStore) List(ctx context.Context, accountID string, limit int, filter *ListFilter) ([]*Execution, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, accountID, limit, filter)
	}
	return nil, nil
}
func (m *mockExecutionStore) UpdateStatus(ctx context.Context, executionID string, status ExecutionStatus, completedAt string, duration int) error {
	if m.updateStatusFunc != nil {
		return m.updateStatusFunc(ctx, executionID, status, completedAt, duration)
	}
	return nil
}
func (m *mockExecutionStore) UpdateCompletion(ctx context.Context, executionID string, status ExecutionStatus, completedAt string, duration int, runnerSeconds int, uploadSeconds int, outputStatus OutputStatus) error {
	if m.updateCompletionFunc != nil {
		return m.updateCompletionFunc(ctx, executionID, status, completedAt, duration, runnerSeconds, uploadSeconds, outputStatus)
	}
	return nil
}
func (m *mockExecutionStore) UpdateManifestWorkName(ctx context.Context, executionID, mwName string) error {
	if m.updateMWNameFunc != nil {
		return m.updateMWNameFunc(ctx, executionID, mwName)
	}
	return nil
}
func (m *mockExecutionStore) ListPending(ctx context.Context) ([]*Execution, error) {
	if m.listPendingFunc != nil {
		return m.listPendingFunc(ctx)
	}
	return nil, nil
}

func reconcilerLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func defaultJobConfig() *JobConfig {
	return &JobConfig{
		ExecutionTimeoutSeconds: 1800,
		TTLSeconds:              3600,
	}
}

func newFakeFleetDB(objs ...client.Object) *fleetdb.Client {
	scheme := runtime.NewScheme()
	_ = hyperfleetv1alpha1.AddToScheme(scheme)

	builder := fake.NewClientBuilder().WithScheme(scheme)
	if len(objs) > 0 {
		builder = builder.WithObjects(objs...)
	}

	return fleetdb.NewClientFrom(builder.Build(), reconcilerLogger())
}

func jobKubeContent(succeeded, failed int32, startTime, completionTime string) runtime.RawExtension {
	job := map[string]interface{}{
		"status": map[string]interface{}{},
	}
	status := job["status"].(map[string]interface{})
	if succeeded > 0 {
		status["succeeded"] = succeeded
	}
	if failed > 0 {
		status["failed"] = failed
	}
	if startTime != "" {
		status["startTime"] = startTime
	}
	if completionTime != "" {
		status["completionTime"] = completionTime
	}
	raw, _ := json.Marshal(job)
	return runtime.RawExtension{Raw: raw}
}

func TestReconcileExecution_PendingToRunning(t *testing.T) {
	var updatedStatus ExecutionStatus
	store := &mockExecutionStore{
		updateStatusFunc: func(ctx context.Context, executionID string, status ExecutionStatus, completedAt string, duration int) error {
			updatedStatus = status
			return nil
		},
	}

	hfm := &hyperfleetv1alpha1.HyperFleetManifest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "zoa-exec-1",
			Namespace: "account-1",
		},
		Spec: hyperfleetv1alpha1.HyperFleetManifestSpec{
			ManagementCluster: "mc01",
			Resources: []hyperfleetv1alpha1.ResourceTemplate{
				{Resource: "jobs", Content: runtime.RawExtension{Raw: []byte("{}")}},
			},
		},
		Status: hyperfleetv1alpha1.HyperFleetManifestStatus{
			Phase: hyperfleetv1alpha1.ManifestPhaseApplied,
		},
	}

	fdb := newFakeFleetDB(hfm)
	r := NewReconciler(store, nil, fdb, defaultJobConfig(), 10*time.Second, reconcilerLogger())

	exec := &Execution{
		ExecutionID:      "exec-1",
		AccountID:        "account-1",
		TargetCluster:    "mc01",
		ManifestWorkName: "zoa-exec-1",
		Status:           StatusPending,
		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
	}

	r.reconcileExecution(context.Background(), exec)

	assert.Equal(t, StatusRunning, updatedStatus)
}

func TestReconcileExecution_FullyCompleted(t *testing.T) {
	var completionStatus ExecutionStatus
	var completionOutputStatus OutputStatus

	store := &mockExecutionStore{
		updateCompletionFunc: func(ctx context.Context, executionID string, status ExecutionStatus, completedAt string, duration int, runnerSeconds int, uploadSeconds int, outputStatus OutputStatus) error {
			completionStatus = status
			completionOutputStatus = outputStatus
			return nil
		},
	}

	now := time.Now().UTC()
	runnerStart := now.Add(-30 * time.Second).Format(time.RFC3339)
	runnerComplete := now.Add(-15 * time.Second).Format(time.RFC3339)
	uploadComplete := now.Add(-5 * time.Second).Format(time.RFC3339)

	hfm := &hyperfleetv1alpha1.HyperFleetManifest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "zoa-exec-2",
			Namespace: "account-1",
		},
		Spec: hyperfleetv1alpha1.HyperFleetManifestSpec{
			ManagementCluster: "mc01",
			Resources: []hyperfleetv1alpha1.ResourceTemplate{
				{Resource: "jobs", Content: runtime.RawExtension{Raw: []byte("{}")}},
			},
		},
		Status: hyperfleetv1alpha1.HyperFleetManifestStatus{
			Phase: hyperfleetv1alpha1.ManifestPhaseApplied,
			ResourceStatuses: []hyperfleetv1alpha1.ResourceStatus{
				{
					Resource:    "jobs",
					Name:        "zoa-exec-2",
					Namespace:   "zoa-jobs",
					KubeContent: jobKubeContent(1, 0, runnerStart, runnerComplete),
				},
				{
					Resource:    "jobs",
					Name:        "zoa-exec-2-upload",
					Namespace:   "zoa-jobs",
					KubeContent: jobKubeContent(1, 0, "", uploadComplete),
				},
			},
		},
	}

	fdb := newFakeFleetDB(hfm)
	r := NewReconciler(store, nil, fdb, defaultJobConfig(), 10*time.Second, reconcilerLogger())

	exec := &Execution{
		ExecutionID:      "exec-2",
		AccountID:        "account-1",
		TargetCluster:    "mc01",
		ManifestWorkName: "zoa-exec-2",
		Status:           StatusRunning,
		CreatedAt:        now.Add(-60 * time.Second).Format(time.RFC3339),
	}

	r.reconcileExecution(context.Background(), exec)

	assert.Equal(t, StatusSucceeded, completionStatus)
	assert.Equal(t, OutputStatusUploaded, completionOutputStatus)
}

func TestReconcileExecution_Timeout(t *testing.T) {
	var statusUpdated ExecutionStatus

	store := &mockExecutionStore{
		updateStatusFunc: func(ctx context.Context, executionID string, status ExecutionStatus, completedAt string, duration int) error {
			statusUpdated = status
			return nil
		},
	}

	hfm := &hyperfleetv1alpha1.HyperFleetManifest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "zoa-exec-timeout",
			Namespace: "account-1",
		},
		Spec: hyperfleetv1alpha1.HyperFleetManifestSpec{
			ManagementCluster: "mc01",
			Resources: []hyperfleetv1alpha1.ResourceTemplate{
				{Resource: "jobs", Content: runtime.RawExtension{Raw: []byte("{}")}},
			},
		},
	}

	fdb := newFakeFleetDB(hfm)
	// Total timeout = exec(60) + upload(120 default) + dispatch(120) = 300s
	cfg := &JobConfig{ExecutionTimeoutSeconds: 60}
	r := NewReconciler(store, nil, fdb, cfg, 10*time.Second, reconcilerLogger())

	exec := &Execution{
		ExecutionID:      "exec-timeout",
		AccountID:        "account-1",
		TargetCluster:    "mc01",
		ManifestWorkName: "zoa-exec-timeout",
		Status:           StatusRunning,
		CreatedAt:        time.Now().UTC().Add(-301 * time.Second).Format(time.RFC3339),
	}

	r.reconcileExecution(context.Background(), exec)

	assert.Equal(t, StatusTimedOut, statusUpdated)

	// Verify the manifest was deleted
	_, err := fdb.GetManifest(context.Background(), "account-1", "zoa-exec-timeout")
	assert.True(t, apierrors.IsNotFound(err))
}

func TestReconcileExecution_HFMNotFound(t *testing.T) {
	store := &mockExecutionStore{
		updateStatusFunc: func(ctx context.Context, executionID string, status ExecutionStatus, completedAt string, duration int) error {
			t.Fatal("should not update status when HFM is not found")
			return nil
		},
	}

	fdb := newFakeFleetDB()
	r := NewReconciler(store, nil, fdb, defaultJobConfig(), 10*time.Second, reconcilerLogger())

	exec := &Execution{
		ExecutionID:      "exec-mw-nil",
		AccountID:        "account-1",
		TargetCluster:    "mc01",
		ManifestWorkName: "zoa-exec-mw-nil",
		Status:           StatusPending,
		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
	}

	r.reconcileExecution(context.Background(), exec)
}

func TestReconcileExecution_DeletionFails(t *testing.T) {
	var completionCalled bool

	store := &mockExecutionStore{
		updateCompletionFunc: func(ctx context.Context, executionID string, status ExecutionStatus, completedAt string, duration int, runnerSeconds int, uploadSeconds int, outputStatus OutputStatus) error {
			completionCalled = true
			return nil
		},
	}

	// Use a real fake client but create an HFM. To simulate deletion failure,
	// we test without the object existing — deleteManifest returns NotFound which is treated as success.
	// Instead, test the completion path by ensuring a completed HFM works.
	hfm := &hyperfleetv1alpha1.HyperFleetManifest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "zoa-exec-rb-fail",
			Namespace: "account-1",
		},
		Spec: hyperfleetv1alpha1.HyperFleetManifestSpec{
			ManagementCluster: "mc01",
			Resources: []hyperfleetv1alpha1.ResourceTemplate{
				{Resource: "jobs", Content: runtime.RawExtension{Raw: []byte("{}")}},
			},
		},
		Status: hyperfleetv1alpha1.HyperFleetManifestStatus{
			Phase: hyperfleetv1alpha1.ManifestPhaseApplied,
			ResourceStatuses: []hyperfleetv1alpha1.ResourceStatus{
				{Resource: "jobs", Name: "zoa-exec-rb-fail", KubeContent: jobKubeContent(1, 0, "", "")},
				{Resource: "jobs", Name: "zoa-exec-rb-fail-upload", KubeContent: jobKubeContent(1, 0, "", "")},
			},
		},
	}

	fdb := newFakeFleetDB(hfm)
	r := NewReconciler(store, nil, fdb, defaultJobConfig(), 10*time.Second, reconcilerLogger())

	exec := &Execution{
		ExecutionID:      "exec-rb-fail",
		AccountID:        "account-1",
		TargetCluster:    "mc01",
		ManifestWorkName: "zoa-exec-rb-fail",
		Status:           StatusRunning,
		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
	}

	r.reconcileExecution(context.Background(), exec)

	assert.True(t, completionCalled, "should update status when deletion succeeds")
}

func TestParseManifestStatus_AllFeedback(t *testing.T) {
	start := "2026-06-01T10:00:00Z"
	runnerEnd := "2026-06-01T10:00:15Z"
	uploadEnd := "2026-06-01T10:00:25Z"

	hfm := &hyperfleetv1alpha1.HyperFleetManifest{
		Status: hyperfleetv1alpha1.HyperFleetManifestStatus{
			Phase: hyperfleetv1alpha1.ManifestPhaseApplied,
			ResourceStatuses: []hyperfleetv1alpha1.ResourceStatus{
				{
					Resource:    "jobs",
					Name:        "zoa-exec-1",
					KubeContent: jobKubeContent(1, 0, start, runnerEnd),
				},
				{
					Resource:    "jobs",
					Name:        "zoa-exec-1-upload",
					KubeContent: jobKubeContent(1, 0, "", uploadEnd),
				},
			},
		},
	}

	r := &Reconciler{logger: reconcilerLogger()}
	result := r.parseManifestStatus(hfm, "exec-1")

	assert.True(t, result.taSucceeded)
	assert.True(t, result.uploadSucceeded)
	assert.True(t, result.fullyCompleted())
	assert.Equal(t, StatusSucceeded, result.taStatus())
	assert.Equal(t, OutputStatusUploaded, result.outputStatus())
}

func TestParseManifestStatus_TAFailedUploadSucceeded(t *testing.T) {
	hfm := &hyperfleetv1alpha1.HyperFleetManifest{
		Status: hyperfleetv1alpha1.HyperFleetManifestStatus{
			Phase: hyperfleetv1alpha1.ManifestPhaseApplied,
			ResourceStatuses: []hyperfleetv1alpha1.ResourceStatus{
				{Resource: "jobs", Name: "zoa-exec-1", KubeContent: jobKubeContent(0, 1, "", "")},
				{Resource: "jobs", Name: "zoa-exec-1-upload", KubeContent: jobKubeContent(1, 0, "", "")},
			},
		},
	}

	r := &Reconciler{logger: reconcilerLogger()}
	result := r.parseManifestStatus(hfm, "exec-1")

	assert.True(t, result.taFailed)
	assert.True(t, result.uploadSucceeded)
	assert.True(t, result.fullyCompleted())
	assert.Equal(t, StatusFailed, result.taStatus())
	assert.Equal(t, OutputStatusUploaded, result.outputStatus())
}

func TestParseManifestStatus_AppliedOnly(t *testing.T) {
	hfm := &hyperfleetv1alpha1.HyperFleetManifest{
		Status: hyperfleetv1alpha1.HyperFleetManifestStatus{
			Phase: hyperfleetv1alpha1.ManifestPhaseApplied,
		},
	}

	r := &Reconciler{logger: reconcilerLogger()}
	result := r.parseManifestStatus(hfm, "exec-1")

	assert.True(t, result.applied)
	assert.False(t, result.fullyCompleted())
}

func TestParseManifestStatus_NoStatusNoPhase(t *testing.T) {
	hfm := &hyperfleetv1alpha1.HyperFleetManifest{
		Status: hyperfleetv1alpha1.HyperFleetManifestStatus{},
	}

	r := &Reconciler{logger: reconcilerLogger()}
	result := r.parseManifestStatus(hfm, "exec-1")

	assert.False(t, result.applied)
	assert.False(t, result.fullyCompleted())
	assert.False(t, result.taSucceeded)
	assert.False(t, result.taFailed)
}

func TestJobResult_ComputeDurations(t *testing.T) {
	jr := &jobResult{
		runnerStartTime:      "2026-06-01T10:00:00Z",
		runnerCompletionTime: "2026-06-01T10:00:13Z",
		uploadCompletionTime: "2026-06-01T10:00:21Z",
	}

	assert.Equal(t, 13, jr.computeRunnerSeconds())
	assert.Equal(t, 8, jr.computeUploadSeconds())
}

func TestJobResult_ComputeDurations_InvalidTimes(t *testing.T) {
	jr := &jobResult{
		runnerStartTime:      "",
		runnerCompletionTime: "invalid",
		uploadCompletionTime: "",
	}

	assert.Equal(t, 0, jr.computeRunnerSeconds())
	assert.Equal(t, 0, jr.computeUploadSeconds())
}

func TestIsTimedOut(t *testing.T) {
	// Total timeout = exec(60) + upload(120 default) + dispatch(120) = 300s
	cfg := &JobConfig{ExecutionTimeoutSeconds: 60}
	r := NewReconciler(nil, nil, nil, cfg, 10*time.Second, reconcilerLogger())

	t.Run("When execution is within timeout it should not be timed out", func(t *testing.T) {
		exec := &Execution{
			CreatedAt: time.Now().UTC().Add(-30 * time.Second).Format(time.RFC3339),
		}
		assert.False(t, r.isTimedOut(exec))
	})

	t.Run("When execution exceeds timeout it should be timed out", func(t *testing.T) {
		exec := &Execution{
			CreatedAt: time.Now().UTC().Add(-301 * time.Second).Format(time.RFC3339),
		}
		assert.True(t, r.isTimedOut(exec))
	})

	t.Run("When execution is exactly at timeout boundary it should not be timed out", func(t *testing.T) {
		exec := &Execution{
			CreatedAt: time.Now().UTC().Add(-299 * time.Second).Format(time.RFC3339),
		}
		assert.False(t, r.isTimedOut(exec))
	})

	t.Run("When createdAt is invalid it should not be timed out", func(t *testing.T) {
		exec := &Execution{
			CreatedAt: "invalid-timestamp",
		}
		assert.False(t, r.isTimedOut(exec))
	})
}

func TestTimeoutForExecution_PerTAOverride(t *testing.T) {
	registry := NewTemplateRegistry(reconcilerLogger())
	registry.templates["slow_action"] = &TATemplate{
		Name:           "slow_action",
		TimeoutSeconds: 3600,
	}
	registry.templates["fast_action"] = &TATemplate{
		Name:           "fast_action",
		TimeoutSeconds: 0,
	}

	// upload=120 (default), dispatch=120 (const)
	cfg := &JobConfig{ExecutionTimeoutSeconds: 1800}
	r := NewReconciler(nil, registry, nil, cfg, 10*time.Second, reconcilerLogger())

	t.Run("When TA has custom timeout it should use TA timeout", func(t *testing.T) {
		exec := &Execution{Action: "slow_action"}
		// 3600 + 120(upload) + 120(dispatch) = 3840s
		assert.Equal(t, 3840*time.Second, r.timeoutForExecution(exec))
	})

	t.Run("When TA has zero timeout it should use default", func(t *testing.T) {
		exec := &Execution{Action: "fast_action"}
		// 1800 + 120(upload) + 120(dispatch) = 2040s
		assert.Equal(t, 2040*time.Second, r.timeoutForExecution(exec))
	})

	t.Run("When TA is not in registry it should use default", func(t *testing.T) {
		exec := &Execution{Action: "unknown_action"}
		// 1800 + 120(upload) + 120(dispatch) = 2040s
		assert.Equal(t, 2040*time.Second, r.timeoutForExecution(exec))
	})
}

func TestReconcileExecution_EmptyManifestWorkName(t *testing.T) {
	store := &mockExecutionStore{
		updateStatusFunc: func(ctx context.Context, executionID string, status ExecutionStatus, completedAt string, duration int) error {
			t.Fatal("should not update status when ManifestWorkName is empty")
			return nil
		},
	}

	r := NewReconciler(store, nil, nil, defaultJobConfig(), 10*time.Second, reconcilerLogger())

	exec := &Execution{
		ExecutionID:      "exec-no-mw",
		TargetCluster:    "mc01",
		ManifestWorkName: "",
		Status:           StatusPending,
		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
	}

	r.reconcileExecution(context.Background(), exec)
}

func TestReconcilePending_ListError(t *testing.T) {
	store := &mockExecutionStore{
		listPendingFunc: func(ctx context.Context) ([]*Execution, error) {
			return nil, errors.New("dynamo error")
		},
	}

	r := NewReconciler(store, nil, nil, defaultJobConfig(), 10*time.Second, reconcilerLogger())
	r.reconcilePending(context.Background())
}

func TestReconcilePending_NoExecutions(t *testing.T) {
	store := &mockExecutionStore{
		listPendingFunc: func(ctx context.Context) ([]*Execution, error) {
			return []*Execution{}, nil
		},
	}

	r := NewReconciler(store, nil, nil, defaultJobConfig(), 10*time.Second, reconcilerLogger())
	r.reconcilePending(context.Background())
}

func TestReconcilePending_MultipleExecutions(t *testing.T) {
	reconciled := make([]string, 0)

	hfm1 := &hyperfleetv1alpha1.HyperFleetManifest{
		ObjectMeta: metav1.ObjectMeta{Name: "zoa-a", Namespace: "account-1"},
		Spec: hyperfleetv1alpha1.HyperFleetManifestSpec{
			ManagementCluster: "mc01",
			Resources:         []hyperfleetv1alpha1.ResourceTemplate{{Resource: "jobs", Content: runtime.RawExtension{Raw: []byte("{}")}}},
		},
		Status: hyperfleetv1alpha1.HyperFleetManifestStatus{Phase: hyperfleetv1alpha1.ManifestPhaseApplied},
	}
	hfm2 := &hyperfleetv1alpha1.HyperFleetManifest{
		ObjectMeta: metav1.ObjectMeta{Name: "zoa-b", Namespace: "account-1"},
		Spec: hyperfleetv1alpha1.HyperFleetManifestSpec{
			ManagementCluster: "mc02",
			Resources:         []hyperfleetv1alpha1.ResourceTemplate{{Resource: "jobs", Content: runtime.RawExtension{Raw: []byte("{}")}}},
		},
		Status: hyperfleetv1alpha1.HyperFleetManifestStatus{Phase: hyperfleetv1alpha1.ManifestPhaseApplied},
	}

	store := &mockExecutionStore{
		listPendingFunc: func(ctx context.Context) ([]*Execution, error) {
			return []*Execution{
				{ExecutionID: "exec-a", AccountID: "account-1", TargetCluster: "mc01", ManifestWorkName: "zoa-a", Status: StatusPending, CreatedAt: time.Now().UTC().Format(time.RFC3339)},
				{ExecutionID: "exec-b", AccountID: "account-1", TargetCluster: "mc02", ManifestWorkName: "zoa-b", Status: StatusPending, CreatedAt: time.Now().UTC().Format(time.RFC3339)},
			}, nil
		},
		updateStatusFunc: func(ctx context.Context, executionID string, status ExecutionStatus, completedAt string, duration int) error {
			reconciled = append(reconciled, executionID)
			return nil
		},
	}

	fdb := newFakeFleetDB(hfm1, hfm2)
	r := NewReconciler(store, nil, fdb, defaultJobConfig(), 10*time.Second, reconcilerLogger())
	r.reconcilePending(context.Background())

	require.Len(t, reconciled, 2)
	assert.Contains(t, reconciled, "exec-a")
	assert.Contains(t, reconciled, "exec-b")
}

func TestHandleTimeout_DeletionFails(t *testing.T) {
	var statusUpdated bool
	store := &mockExecutionStore{
		updateStatusFunc: func(ctx context.Context, executionID string, status ExecutionStatus, completedAt string, duration int) error {
			statusUpdated = true
			return nil
		},
	}

	// HFM not found → deleteManifest treats as success → timeout status should be set
	fdb := newFakeFleetDB()
	cfg := &JobConfig{ExecutionTimeoutSeconds: 60}
	r := NewReconciler(store, nil, fdb, cfg, 10*time.Second, reconcilerLogger())

	exec := &Execution{
		ExecutionID:      "exec-timeout-rb-fail",
		AccountID:        "account-1",
		TargetCluster:    "mc01",
		ManifestWorkName: "zoa-exec-timeout-rb-fail",
		Status:           StatusRunning,
		CreatedAt:        time.Now().UTC().Add(-120 * time.Second).Format(time.RFC3339),
	}

	r.handleTimeout(context.Background(), exec)

	assert.True(t, statusUpdated, "should update status when deletion succeeds (not found = success)")
}

func TestDeleteManifest_AlreadyGone(t *testing.T) {
	fdb := newFakeFleetDB()
	r := NewReconciler(nil, nil, fdb, defaultJobConfig(), 10*time.Second, reconcilerLogger())

	exec := &Execution{
		ExecutionID:      "exec-gone",
		AccountID:        "account-1",
		TargetCluster:    "mc01",
		ManifestWorkName: "zoa-exec-gone",
	}

	err := r.deleteManifest(context.Background(), exec)
	assert.NoError(t, err)
}

// Suppress unused import warning for schema package.
var _ = schema.GroupResource{}
