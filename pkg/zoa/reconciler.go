package zoa

import (
	"context"
	"log/slog"
	"time"

	"github.com/openshift/rosa-regional-platform-api/pkg/clients/maestro"
	workv1 "open-cluster-management.io/api/work/v1"
)

// Reconciler periodically checks pending/running TA executions and updates their
// status by inspecting Maestro ManifestWork feedback via gRPC.
// On terminal states (succeeded, failed, timeout), it deletes the ResourceBundle
// BEFORE updating status, preventing stale RBs if the status update were to fail.
type Reconciler struct {
	store         ExecutionStore
	registry      *TemplateRegistry
	maestroClient maestro.ClientInterface
	jobConfig     *JobConfig
	logger        *slog.Logger
	interval      time.Duration
}

func NewReconciler(
	store ExecutionStore,
	registry *TemplateRegistry,
	maestroClient maestro.ClientInterface,
	jobConfig *JobConfig,
	interval time.Duration,
	logger *slog.Logger,
) *Reconciler {
	return &Reconciler{
		store:         store,
		registry:      registry,
		maestroClient: maestroClient,
		jobConfig:     jobConfig,
		logger:        logger,
		interval:      interval,
	}
}

func (r *Reconciler) Run(ctx context.Context) {
	r.logger.Info("ZOA reconciler started", "interval", r.interval, "default_timeout_seconds", r.jobConfig.ExecutionTimeoutSeconds)
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
		r.handleTimeout(ctx, exec)
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

	if completed {
		r.handleCompletion(ctx, exec, ExecutionStatus(newStatus))
		return
	}

	if err := r.store.UpdateStatus(ctx, exec.ExecutionID, ExecutionStatus(newStatus), "", 0); err != nil {
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

// handleTimeout deletes the ResourceBundle FIRST, then marks as timed_out.
// If RB deletion fails, status stays pending/running so the reconciler retries.
func (r *Reconciler) handleTimeout(ctx context.Context, exec *Execution) {
	timeout := r.timeoutForExecution(exec)
	createdAt, _ := time.Parse(time.RFC3339, exec.CreatedAt)

	r.logger.Warn("execution exceeded timeout, cleaning up",
		"execution_id", exec.ExecutionID,
		"age", time.Since(createdAt).String(),
		"timeout", timeout.String(),
	)

	if err := r.deleteResourceBundle(ctx, exec); err != nil {
		return
	}

	now := time.Now().UTC()
	duration := int(now.Sub(createdAt).Seconds())
	if err := r.store.UpdateStatus(ctx, exec.ExecutionID, StatusTimedOut, now.Format(time.RFC3339), duration); err != nil {
		r.logger.Error("resource bundle deleted but failed to update status to timed_out — will not retry RB deletion",
			"execution_id", exec.ExecutionID,
			"error", err,
		)
		return
	}

	r.logger.Info("execution marked as timed_out",
		"execution_id", exec.ExecutionID,
		"duration_seconds", duration,
	)
}

// handleCompletion processes a terminal execution status.
// On success: deletes RB first (race-safe), then updates status.
// On failure: updates status only — leaves RB/Job in place for log inspection
// (Job's ttlSecondsAfterFinished handles eventual GC).
func (r *Reconciler) handleCompletion(ctx context.Context, exec *Execution, terminalStatus ExecutionStatus) {
	if terminalStatus == StatusSucceeded {
		if err := r.deleteResourceBundle(ctx, exec); err != nil {
			return
		}
	}

	now := time.Now().UTC()
	var duration int
	createdAt, err := time.Parse(time.RFC3339, exec.CreatedAt)
	if err == nil {
		duration = int(now.Sub(createdAt).Seconds())
	}

	if err := r.store.UpdateStatus(ctx, exec.ExecutionID, terminalStatus, now.Format(time.RFC3339), duration); err != nil {
		r.logger.Error("failed to update terminal status",
			"execution_id", exec.ExecutionID,
			"terminal_status", string(terminalStatus),
			"error", err,
		)
		return
	}

	r.logger.Info("execution completed",
		"execution_id", exec.ExecutionID,
		"status", string(terminalStatus),
		"duration_seconds", duration,
	)

	if terminalStatus == StatusFailed {
		r.logger.Info("failed job preserved for log inspection (TTL will GC)",
			"execution_id", exec.ExecutionID,
			"manifest_work", exec.ManifestWorkName,
		)
	}
}

// deleteResourceBundle removes the RB from Maestro. Returns nil on success or
// if the RB is already gone (idempotent). Returns error if deletion actually fails.
func (r *Reconciler) deleteResourceBundle(ctx context.Context, exec *Execution) error {
	err := r.maestroClient.DeleteManifestWork(ctx, exec.TargetCluster, exec.ManifestWorkName)
	if err != nil {
		if maestro.IsNotFound(err) {
			r.logger.Debug("resource bundle already deleted",
				"execution_id", exec.ExecutionID,
				"manifest_work", exec.ManifestWorkName,
			)
			return nil
		}
		r.logger.Error("failed to delete resource bundle — will retry next reconcile",
			"execution_id", exec.ExecutionID,
			"manifest_work", exec.ManifestWorkName,
			"error", err,
		)
		return err
	}

	r.logger.Info("resource bundle deleted",
		"execution_id", exec.ExecutionID,
		"manifest_work", exec.ManifestWorkName,
	)
	return nil
}

func (r *Reconciler) timeoutForExecution(exec *Execution) time.Duration {
	if r.registry != nil {
		if tmpl, ok := r.registry.Get(exec.Action); ok && tmpl.TimeoutSeconds > 0 {
			return time.Duration(tmpl.TimeoutSeconds) * time.Second
		}
	}
	return time.Duration(r.jobConfig.ExecutionTimeoutSeconds) * time.Second
}

func (r *Reconciler) isTimedOut(exec *Execution) bool {
	createdAt, err := time.Parse(time.RFC3339, exec.CreatedAt)
	if err != nil {
		return false
	}
	return time.Since(createdAt) > r.timeoutForExecution(exec)
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
