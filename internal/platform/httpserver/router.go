// Package httpserver owns the HTTP router, middleware chain, and server
// lifecycle. Feature routes arrive through [Module] registration.
package httpserver

import (
	"log/slog"
	"net/http"

	"github.com/svaan1/go-api-boilerplate/internal/platform/auth"
	"github.com/svaan1/go-api-boilerplate/internal/platform/config"
	"github.com/svaan1/go-api-boilerplate/internal/platform/httpx"
	"github.com/svaan1/go-api-boilerplate/internal/platform/observability"
	"github.com/svaan1/go-api-boilerplate/internal/platform/ratelimit"
)

// Dependencies supplies concrete process-owned transport capabilities.
type Dependencies struct {
	Logger   *slog.Logger
	Verifier auth.Verifier
	Limiter  ratelimit.Limiter
	Metrics  *observability.Metrics
}

// NewRouter registers module routes plus a stable API not-found response and
// wraps them in the transport middleware chain.
func NewRouter(cfg config.Config, deps Dependencies, modules []Module) (http.Handler, error) {
	mux := http.NewServeMux()
	if err := registerModules(mux, modules); err != nil {
		return nil, err
	}
	mux.HandleFunc("/v1/", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteProblem(w, r, http.StatusNotFound, "resource not found", nil)
	})
	// Dispatch through ServeMux.ServeHTTP, not the handler returned by
	// mux.Handler: only ServeHTTP populates Request.PathValue wildcards.
	handler := auth.Authenticate(deps.Verifier)(mux)
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
	// Resolve the route pattern before the middleware chain so logging,
	// metrics, and rate limiting see the normalized pattern, never raw paths.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pattern := mux.Handler(r)
		r.Pattern = pattern
		handler.ServeHTTP(w, r)
	}), nil
}
