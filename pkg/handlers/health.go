package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync/atomic"
)

// HealthHandler handles health check endpoints
type HealthHandler struct {
	ready  *atomic.Bool
	logger *slog.Logger
}

// NewHealthHandler creates a new HealthHandler
func NewHealthHandler(logger *slog.Logger) *HealthHandler {
	ready := &atomic.Bool{}
	ready.Store(true)
	return &HealthHandler{
		ready:  ready,
		logger: logger,
	}
}

// SetReady sets the readiness state
func (h *HealthHandler) SetReady(ready bool) {
	h.ready.Store(ready)
}

// Liveness handles GET /live
func (h *HealthHandler) Liveness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

// Readiness handles GET /ready
func (h *HealthHandler) Readiness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if !h.ready.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "unavailable"}); err != nil {
			h.logger.Error("failed to encode response", "error", err)
		}
		return
	}

	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}
