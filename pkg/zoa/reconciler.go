package zoa

import (
	"context"
	"log/slog"
	"time"

	"github.com/openshift/rosa-regional-platform-api/pkg/clients/maestro"
	workv1 "open-cluster-management.io/api/work/v1"
)

const defaultExecutionTimeout = 30 * time.Minute

// Reconciler periodically checks pending/running TA executions and updates their
// status by inspecting Maestro ManifestWork feedback via gRPC.
type Reconciler struct {
	store            ExecutionStore
	maestroClient    maestro.ClientInterface
	logger           *slog.Logger
	interval         time.Duration
	executionTimeout time.Duration
}

// NewReconciler creates a new ZOA status reconciler.
func NewReconciler(store ExecutionStore, maestroClient maestro.ClientInterface, interval time.Duration, logger *slog.Logger) *Reconciler {
	return &Reconciler{
		store:            store,
		maestroClient:    maestroClient,
		logger:           logger,
		interval:         interval,
		executionTimeout: defaultExecutionTimeout,
	}
}

// Run starts the reconciliation loop. It blocks until ctx is cancelled.
func (r *Reconciler) Run(ctx context.Context) {
	r.logger.Info("ZOA reconciler started", "interval", r.interval)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("ZOA reconciler stopped")
			return
		case <-ticker.C:
			r.reconcilePending(ctx)
		}
	}
}

func (r *Reconciler) reconcilePending(ctx context.Context) {
	executions, err := r.store.ListPending(ctx)
	if err != nil {
		r.logger.Error("failed to list pending executions", "error", err)
		return
	}

	if len(executions) == 0 {
		return
	}

	r.logger.Debug("reconciling pending executions", "count", len(executions))

	for _, exec := range executions {
		r.reconcileExecution(ctx, exec)
	}
}

func (r *Reconciler) reconcileExecution(ctx context.Context, exec *Execution) {
	if exec.ManifestWorkName == "" || exec.TargetCluster == "" {
		return
	}

	if r.isTimedOut(exec) {
		now := time.Now().UTC()
		createdAt, _ := time.Parse(time.RFC3339, exec.CreatedAt)
		duration := int(now.Sub(createdAt).Seconds())
		if err := r.store.UpdateStatus(ctx, exec.ExecutionID, StatusFailed, now.Format(time.RFC3339), duration); err != nil {
			r.logger.Error("failed to mark execution as timed out", "execution_id", exec.ExecutionID, "error", err)
		} else {
			r.logger.Warn("execution timed out",
				"execution_id", exec.ExecutionID,
				"age", now.Sub(createdAt).String(),
				"timeout", r.executionTimeout.String(),
			)
		}
		return
	}

	mw, err := r.maestroClient.GetManifestWork(ctx, exec.TargetCluster, exec.ManifestWorkName)
	if err != nil {
		r.logger.Error("failed to get manifestwork from maestro",
			"execution_id", exec.ExecutionID,
			"manifest_work", exec.ManifestWorkName,
			"target_cluster", exec.TargetCluster,
			"error", err,
		)
		return
	}

	if mw == nil {
		return
	}

	newStatus, completed := r.parseManifestWorkStatus(mw)
	if newStatus == "" {
		return
	}

	if ExecutionStatus(newStatus) == exec.Status {
		return
	}

	var completedAt string
	var duration int
	if completed {
		now := time.Now().UTC()
		completedAt = now.Format(time.RFC3339)
		createdAt, err := time.Parse(time.RFC3339, exec.CreatedAt)
		if err == nil {
			duration = int(now.Sub(createdAt).Seconds())
		}
	}

	if err := r.store.UpdateStatus(ctx, exec.ExecutionID, ExecutionStatus(newStatus), completedAt, duration); err != nil {
		r.logger.Error("failed to update execution status",
			"execution_id", exec.ExecutionID,
			"new_status", newStatus,
			"error", err,
		)
		return
	}

	r.logger.Info("execution status updated",
		"execution_id", exec.ExecutionID,
		"status", newStatus,
	)
}

func (r *Reconciler) isTimedOut(exec *Execution) bool {
	createdAt, err := time.Parse(time.RFC3339, exec.CreatedAt)
	if err != nil {
		return false
	}
	return time.Since(createdAt) > r.executionTimeout
}

// parseManifestWorkStatus extracts the Job completion status from ManifestWork status feedback.
func (r *Reconciler) parseManifestWorkStatus(mw *workv1.ManifestWork) (string, bool) {
	for _, resourceStatus := range mw.Status.ResourceStatus.Manifests {
		for _, value := range resourceStatus.StatusFeedbacks.Values {
			switch value.Name {
			case "succeeded":
				if value.Value.Integer != nil && *value.Value.Integer > 0 {
					return string(StatusSucceeded), true
				}
			case "failed":
				if value.Value.Integer != nil && *value.Value.Integer > 0 {
					return string(StatusFailed), true
				}
			}
		}
	}

	for _, condition := range mw.Status.Conditions {
		if condition.Type == "Applied" && condition.Status == "True" {
			return string(StatusRunning), false
		}
	}

	return "", false
}
