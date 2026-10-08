// Package http serves liveness, readiness, metrics, and the caller's identity.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/svaan1/go-api-boilerplate/internal/features/system/application"
	"github.com/svaan1/go-api-boilerplate/internal/platform/auth"
	"github.com/svaan1/go-api-boilerplate/internal/platform/httpserver"
	"github.com/svaan1/go-api-boilerplate/internal/platform/httpx"
)

// Config holds the handler's collaborators. A nil Metrics handler leaves the
// metrics route unregistered.
type Config struct {
	Logger           *slog.Logger
	Readiness        *application.Readiness
	ReadinessTimeout time.Duration
	Metrics          http.Handler
	MetricsPath      string
}

// Handler serves system endpoints.
type Handler struct {
	cfg Config
}

// NewHandler constructs system endpoints.
func NewHandler(cfg Config) *Handler {
	return &Handler{cfg: cfg}
}

// RegisterRoutes installs system routes on mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("GET /readyz", h.ready)
	mux.Handle("GET /v1/me", auth.RequireAuthenticated(http.HandlerFunc(h.me)))
	if h.cfg.Metrics != nil {
		mux.Handle("GET "+h.cfg.MetricsPath, h.cfg.Metrics)
	}
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeStatus(w, http.StatusOK, "ok")
}

func (h *Handler) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.ReadinessTimeout)
	defer cancel()
	if err := h.cfg.Readiness.Check(ctx); err != nil {
		// Log only the checker name: dependency errors may carry credentials.
		if checkErr := (*application.CheckError)(nil); errors.As(err, &checkErr) {
			h.cfg.Logger.Warn("readiness unavailable",
				"request_id", httpserver.RequestID(r), "check", checkErr.Name)
		}
		writeStatus(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	writeStatus(w, http.StatusOK, "ok")
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.FromContext(r.Context())
	if !ok {
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "authentication required", nil)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(identity)
}

func writeStatus(w http.ResponseWriter, code int, status string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(struct {
		Status    string    `json:"status"`
		Timestamp time.Time `json:"timestamp"`
	}{Status: status, Timestamp: time.Now().UTC()})
}
