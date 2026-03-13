package handlers

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
)

// HealthHandler handles health check endpoints
type HealthHandler struct {
	ready *atomic.Bool
}

// NewHealthHandler creates a new HealthHandler
func NewHealthHandler() *HealthHandler {
	ready := &atomic.Bool{}
	ready.Store(true)
	return &HealthHandler{
		ready: ready,
	}
}

// SetReady sets the readiness state
func (h *HealthHandler) SetReady(ready bool) {
	h.ready.Store(ready)
}

// Liveness handles GET /live
func (h *HealthHandler) Liveness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Readiness handles GET /ready
func (h *HealthHandler) Readiness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if !h.ready.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "unavailable"})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
