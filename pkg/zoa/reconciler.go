package zoa

import (
	"context"
	"log/slog"
	"time"

	"github.com/openshift/rosa-regional-platform-api/pkg/clients/maestro"
)

// Reconciler periodically checks pending/running TA executions and updates their
// status by inspecting Maestro ResourceBundle feedback.
type Reconciler struct {
	store         ExecutionStore
	maestroClient maestro.ClientInterface
	logger        *slog.Logger
	interval      time.Duration
}

// NewReconciler creates a new ZOA status reconciler.
func NewReconciler(store ExecutionStore, maestroClient maestro.ClientInterface, interval time.Duration, logger *slog.Logger) *Reconciler {
	return &Reconciler{
		store:         store,
		maestroClient: maestroClient,
		logger:        logger,
		interval:      interval,
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
	if exec.ManifestWorkName == "" {
		return
	}

	bundle, err := r.maestroClient.GetResourceBundle(ctx, exec.ManifestWorkName)
	if err != nil {
		r.logger.Error("failed to query maestro for execution status",
			"execution_id", exec.ExecutionID,
			"resource_bundle_id", exec.ManifestWorkName,
			"error", err,
		)
		return
	}

	if bundle == nil || bundle.Status == nil {
		return
	}

	newStatus, completed := r.parseJobStatus(bundle.Status)
	if newStatus == "" {
		return
	}

	// Only update if status actually changed
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
	}
}

// parseJobStatus extracts the Job completion status from ResourceBundle status feedback.
// Returns the new status string and whether the job is terminal.
func (r *Reconciler) parseJobStatus(status map[string]interface{}) (string, bool) {
	resourceStatus, ok := status["resourceStatus"].([]interface{})
	if !ok {
		return "", false
	}

	for _, rs := range resourceStatus {
		rsMap, ok := rs.(map[string]interface{})
		if !ok {
			continue
		}

		feedback, ok := rsMap["statusFeedback"].(map[string]interface{})
		if !ok {
			continue
		}

		values, ok := feedback["values"].([]interface{})
		if !ok {
			continue
		}

		for _, v := range values {
			vMap, ok := v.(map[string]interface{})
			if !ok {
				continue
			}

			name, _ := vMap["name"].(string)
			fieldValue, _ := vMap["fieldValue"].(map[string]interface{})
			if fieldValue == nil {
				continue
			}

			intVal, _ := fieldValue["integer"].(float64)

			switch name {
			case "succeeded":
				if intVal > 0 {
					return string(StatusSucceeded), true
				}
			case "failed":
				if intVal > 0 {
					return string(StatusFailed), true
				}
			}
		}
	}

	// Check conditions for "running" state
	for _, rs := range resourceStatus {
		rsMap, ok := rs.(map[string]interface{})
		if !ok {
			continue
		}
		conditions, ok := rsMap["conditions"].([]interface{})
		if !ok {
			continue
		}
		for _, c := range conditions {
			cMap, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			if cMap["type"] == "Applied" && cMap["status"] == "True" {
				return string(StatusRunning), false
			}
		}
	}

	return "", false
}
