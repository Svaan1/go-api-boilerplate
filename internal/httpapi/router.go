// Package httpapi owns public HTTP routes and their transport policy.
package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/svaan1/go-api-boilerplate/internal/auth"
	"github.com/svaan1/go-api-boilerplate/internal/config"
	"github.com/svaan1/go-api-boilerplate/internal/observability"
	"github.com/svaan1/go-api-boilerplate/internal/plugin"
	"github.com/svaan1/go-api-boilerplate/internal/problem"
	"github.com/svaan1/go-api-boilerplate/internal/ratelimit"
)

// Pinger provides bounded dependency readiness without exposing internals.
type Pinger interface{ Ping(context.Context) error }

// Dependencies supplies concrete process-owned transport capabilities.
type Dependencies struct {
	Logger   *slog.Logger
	Database Pinger
	Pool     *pgxpool.Pool
	Verifier auth.Verifier
	Limiter  ratelimit.Limiter
	Metrics  *observability.Metrics
}

type matchedRouteKey struct{}

// NewRouter registers public endpoints and a stable API not-found response.
func NewRouter(cfg config.Config, deps Dependencies) (http.Handler, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeStatus(w, http.StatusOK, "ok")
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if deps.Database == nil {
			writeStatus(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), cfg.Shutdown.ReadinessTimeout)
		defer cancel()
		if deps.Database.Ping(ctx) != nil {
			deps.Logger.Warn("readiness unavailable", "request_id", RequestID(r))
			writeStatus(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
		writeStatus(w, http.StatusOK, "ok")
	})
	if err := plugin.Register(mux, cfg.App.Plugins, plugin.Dependencies{
		Logger: deps.Logger, Pool: deps.Pool,
	}); err != nil {
		return nil, err
	}
	if cfg.Observability.Metrics && deps.Metrics != nil {
		mux.Handle("GET "+cfg.Observability.MetricsPath, deps.Metrics.Handler())
	}
	mux.HandleFunc("/v1/", func(w http.ResponseWriter, r *http.Request) {
		problem.Write(w, r, http.StatusNotFound, "resource not found", nil)
	})
	dispatch := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		matched := r.Context().Value(matchedRouteKey{}).(http.Handler)
		matched.ServeHTTP(w, r)
	})
	handler := auth.Authenticate(deps.Verifier)(dispatch)
	var onRejected func(*http.Request)
	if deps.Metrics != nil {
		onRejected = deps.Metrics.RateLimitRejected
	}
	handler = RateLimit(handler, deps.Limiter, onRejected)
	handler = ClientIdentity(cfg)(handler)
	handler = LimitBody(handler, cfg.HTTP.MaxBodyBytes)
	handler = CORS(cfg)(handler)
	handler = SecurityHeaders(cfg)(handler)
	handler = RecoverPanic(handler, deps.Logger)
	if deps.Metrics != nil {
		handler = deps.Metrics.Wrap(handler)
	}
	handler = RequestLogging(handler, deps.Logger)
	handler = WithRequestID(handler)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		matched, pattern := mux.Handler(r)
		r.Pattern = pattern
		handler.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), matchedRouteKey{}, matched)))
	}), nil
}

func writeStatus(w http.ResponseWriter, code int, status string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(struct {
		Status    string    `json:"status"`
		Timestamp time.Time `json:"timestamp"`
	}{Status: status, Timestamp: time.Now().UTC()})
}
