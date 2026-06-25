package zoa

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/openshift/rosa-regional-platform-api/pkg/clients/fleetdb"
	hyperfleetv1alpha1 "github.com/typeid/hyperfleet-operator/api/v1alpha1"
)

// Reconciler periodically checks pending/running TA executions and updates their
// status by inspecting HyperFleetManifest CR status on fleet-db.
// On terminal states (succeeded, failed, timeout), it deletes the HyperFleetManifest
// BEFORE updating status, preventing stale CRs if the status update were to fail.
type Reconciler struct {
	store     ExecutionStore
	registry  *TemplateRegistry
	fleetDB   *fleetdb.Client
	jobConfig *JobConfig
	logger    *slog.Logger
	interval  time.Duration
}

func NewReconciler(
	store ExecutionStore,
	registry *TemplateRegistry,
	fleetDB *fleetdb.Client,
	jobConfig *JobConfig,
	interval time.Duration,
	logger *slog.Logger,
) *Reconciler {
	return &Reconciler{
		store:     store,
		registry:  registry,
		fleetDB:   fleetDB,
		jobConfig: jobConfig,
		logger:    logger,
		interval:  interval,
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

	hfm, err := r.fleetDB.GetManifest(ctx, JobNamespace, exec.ManifestWorkName)
	if err != nil {
		if fleetdb.IsNotFound(err) {
			return
		}
		r.logger.Error("failed to get manifest from fleet-db",
			"execution_id", exec.ExecutionID,
			"manifest", exec.ManifestWorkName,
			"target_cluster", exec.TargetCluster,
			"error", err,
		)
		return
	}

	if hfm == nil {
		return
	}

	result := r.parseManifestStatus(hfm, exec.ExecutionID)

	if result.fullyCompleted() {
		r.handleCompletion(ctx, exec, result)
		return
	}

	if result.applied && exec.Status == StatusPending {
		if err := r.store.UpdateStatus(ctx, exec.ExecutionID, StatusRunning, "", 0); err != nil {
			r.logger.Error("failed to update execution status to running",
				"execution_id", exec.ExecutionID,
				"error", err,
			)
			return
		}
		r.logger.Info("execution status updated",
			"execution_id", exec.ExecutionID,
			"status", "running",
		)
	}
}

// handleTimeout deletes the HyperFleetManifest FIRST, then marks as timed_out.
// If deletion fails, status stays pending/running so the reconciler retries.
func (r *Reconciler) handleTimeout(ctx context.Context, exec *Execution) {
	timeout := r.timeoutForExecution(exec)
	createdAt, _ := time.Parse(time.RFC3339, exec.CreatedAt)

	r.logger.Warn("execution exceeded timeout, cleaning up",
		"execution_id", exec.ExecutionID,
		"age", time.Since(createdAt).String(),
		"timeout", timeout.String(),
	)

	if err := r.deleteManifest(ctx, exec); err != nil {
		return
	}

	now := time.Now().UTC()
	duration := int(now.Sub(createdAt).Seconds())
	if err := r.store.UpdateStatus(ctx, exec.ExecutionID, StatusTimedOut, now.Format(time.RFC3339), duration); err != nil {
		r.logger.Error("manifest deleted but failed to update status to timed_out — will not retry deletion",
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

// handleCompletion deletes the HyperFleetManifest FIRST, then updates terminal status.
// Computes durations from Job timestamps reported via HFM resource statuses.
func (r *Reconciler) handleCompletion(ctx context.Context, exec *Execution, result *jobResult) {
	if err := r.deleteManifest(ctx, exec); err != nil {
		return
	}

	now := time.Now().UTC()
	createdAt, _ := time.Parse(time.RFC3339, exec.CreatedAt)
	totalDuration := int(now.Sub(createdAt).Seconds())

	runnerSeconds := result.computeRunnerSeconds()
	uploadSeconds := result.computeUploadSeconds()

	status := result.taStatus()
	if status == "" {
		status = StatusFailed
	}
	outputStatus := result.outputStatus()

	if err := r.store.UpdateCompletion(ctx, exec.ExecutionID, status, now.Format(time.RFC3339), totalDuration, runnerSeconds, uploadSeconds, outputStatus); err != nil {
		r.logger.Error("manifest deleted but failed to update terminal status",
			"execution_id", exec.ExecutionID,
			"terminal_status", string(status),
			"error", err,
		)
		return
	}

	r.logger.Info("execution completed",
		"execution_id", exec.ExecutionID,
		"status", string(status),
		"output_status", string(outputStatus),
		"duration_seconds", totalDuration,
		"runner_seconds", runnerSeconds,
		"upload_seconds", uploadSeconds,
	)
}

// deleteManifest removes the HyperFleetManifest from fleet-db. Returns nil on success or
// if the CR is already gone (idempotent). Returns error if deletion actually fails.
func (r *Reconciler) deleteManifest(ctx context.Context, exec *Execution) error {
	err := r.fleetDB.DeleteManifest(ctx, JobNamespace, exec.ManifestWorkName)
	if err != nil {
		if fleetdb.IsNotFound(err) {
			r.logger.Debug("manifest already deleted",
				"execution_id", exec.ExecutionID,
				"manifest", exec.ManifestWorkName,
			)
			return nil
		}
		r.logger.Error("failed to delete manifest — will retry next reconcile",
			"execution_id", exec.ExecutionID,
			"manifest", exec.ManifestWorkName,
			"error", err,
		)
		return err
	}

	r.logger.Info("manifest deleted",
		"execution_id", exec.ExecutionID,
		"manifest", exec.ManifestWorkName,
	)
	return nil
}

const dispatchBuffer = 120 // seconds — covers DynamoDB desire propagation + pod scheduling + image pull

func (r *Reconciler) timeoutForExecution(exec *Execution) time.Duration {
	execTimeout := r.jobConfig.ExecutionTimeoutSeconds
	if r.registry != nil {
		if tmpl, ok := r.registry.Get(exec.Action); ok && tmpl.TimeoutSeconds > 0 {
			execTimeout = tmpl.TimeoutSeconds
		}
	}
	uploadTimeout := r.jobConfig.UploadTimeoutSeconds
	if uploadTimeout == 0 {
		uploadTimeout = 120
	}
	return time.Duration(execTimeout+uploadTimeout+dispatchBuffer) * time.Second
}

func (r *Reconciler) isTimedOut(exec *Execution) bool {
	createdAt, err := time.Parse(time.RFC3339, exec.CreatedAt)
	if err != nil {
		return false
	}
	return time.Since(createdAt) > r.timeoutForExecution(exec)
}

// jobResult holds parsed completion info from HyperFleetManifest resource statuses.
type jobResult struct {
	taSucceeded          bool
	taFailed             bool
	uploadSucceeded      bool
	uploadFailed         bool
	applied              bool
	runnerStartTime      string
	runnerCompletionTime string
	uploadCompletionTime string
}

func (jr *jobResult) taCompleted() bool {
	return jr.taSucceeded || jr.taFailed
}

func (jr *jobResult) uploadCompleted() bool {
	return jr.uploadSucceeded || jr.uploadFailed
}

func (jr *jobResult) fullyCompleted() bool {
	return jr.taCompleted() && jr.uploadCompleted()
}

func (jr *jobResult) taStatus() ExecutionStatus {
	if jr.taSucceeded {
		return StatusSucceeded
	}
	if jr.taFailed {
		return StatusFailed
	}
	return ""
}

func (jr *jobResult) outputStatus() OutputStatus {
	if jr.uploadSucceeded {
		return OutputStatusUploaded
	}
	if jr.uploadFailed {
		return OutputStatusFailed
	}
	return OutputStatusPending
}

func (jr *jobResult) computeRunnerSeconds() int {
	start, err1 := time.Parse(time.RFC3339, jr.runnerStartTime)
	end, err2 := time.Parse(time.RFC3339, jr.runnerCompletionTime)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(end.Sub(start).Seconds())
}

func (jr *jobResult) computeUploadSeconds() int {
	runnerEnd, err1 := time.Parse(time.RFC3339, jr.runnerCompletionTime)
	uploadEnd, err2 := time.Parse(time.RFC3339, jr.uploadCompletionTime)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(uploadEnd.Sub(runnerEnd).Seconds())
}

// partialJobStatus mirrors the subset of batch/v1 Job.Status we need.
// ResourceStatus.Status contains the .status sub-object directly (not the full Job).
type partialJobStatus struct {
	Succeeded      int32  `json:"succeeded,omitempty"`
	Failed         int32  `json:"failed,omitempty"`
	StartTime      string `json:"startTime,omitempty"`
	CompletionTime string `json:"completionTime,omitempty"`
}

// parseManifestStatus extracts Job status from the HFM's resource statuses.
// Runner job name = "zoa-{execID}", upload job name = "zoa-{execID}-upload".
func (r *Reconciler) parseManifestStatus(hfm *hyperfleetv1alpha1.HyperFleetManifest, execID string) *jobResult {
	result := &jobResult{}

	runnerJobName := "zoa-" + execID
	uploadJobName := "zoa-" + execID + "-upload"

	for _, rs := range hfm.Status.ResourceStatuses {
		if rs.Resource != "jobs" {
			continue
		}

		if len(rs.Status.Raw) == 0 {
			continue
		}

		var job partialJobStatus
		if err := json.Unmarshal(rs.Status.Raw, &job); err != nil {
			r.logger.Debug("failed to unmarshal job status from resource status",
				"name", rs.Name,
				"error", err,
			)
			continue
		}

		switch rs.Name {
		case runnerJobName:
			if job.Succeeded > 0 {
				result.taSucceeded = true
			}
			if job.Failed > 0 {
				result.taFailed = true
			}
			if job.StartTime != "" {
				result.runnerStartTime = job.StartTime
			}
			if job.CompletionTime != "" {
				result.runnerCompletionTime = job.CompletionTime
			}
		case uploadJobName:
			if job.Succeeded > 0 {
				result.uploadSucceeded = true
			}
			if job.Failed > 0 {
				result.uploadFailed = true
			}
			if job.CompletionTime != "" {
				result.uploadCompletionTime = job.CompletionTime
			}
		}
	}

	if result.taCompleted() || result.uploadCompleted() {
		return result
	}

	if hfm.Status.Phase == hyperfleetv1alpha1.ManifestPhaseApplied {
		result.applied = true
	}

	return result
}
