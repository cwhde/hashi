// Package httpapi holds the HTTP handlers. Feature handlers implement the generated OpenAPI server.
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"
)

// Check is one readiness dependency. It returns nil when healthy.
type Check func(ctx context.Context) error

// Health serves /healthz (process alive) and /readyz (dependencies OK and not shutting down).
type Health struct {
	checks   map[string]Check
	draining atomic.Bool
}

// NewHealth returns a Health with the named readiness checks.
func NewHealth(checks map[string]Check) *Health { return &Health{checks: checks} }

// Drain makes /readyz fail so load balancers stop sending traffic before shutdown.
func (h *Health) Drain() { h.draining.Store(true) }

type healthBody struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

func (h *Health) Healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthBody{Status: "ok"})
}

func (h *Health) Readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	body := healthBody{Status: "ok", Checks: map[string]string{}}
	code := http.StatusOK
	if h.draining.Load() {
		body.Checks["shutdown"] = "draining"
		body.Status, code = "unavailable", http.StatusServiceUnavailable
	}
	for name, c := range h.checks {
		// Check errors are our own messages (schema version, ping); they carry no secrets.
		if err := c(ctx); err != nil {
			body.Checks[name] = err.Error()
			body.Status, code = "unavailable", http.StatusServiceUnavailable
		} else {
			body.Checks[name] = "ok"
		}
	}
	writeJSON(w, code, body)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
