// Package system serves process health, readiness, metrics, and the
// authenticated caller's identity.
package system

import (
	"log/slog"
	"net/http"
	"time"

	systemhttp "github.com/svaan1/go-api-boilerplate/internal/features/system/adapters/http"
	"github.com/svaan1/go-api-boilerplate/internal/features/system/application"
	"github.com/svaan1/go-api-boilerplate/internal/platform/httpserver"
)

// Dependencies are supplied by the composition root.
type Dependencies struct {
	Logger *slog.Logger
	// HealthCheckers gate readiness; with none registered readiness fails closed.
	HealthCheckers   []application.HealthChecker
	ReadinessTimeout time.Duration
	// Metrics, when non-nil, is served at MetricsPath.
	Metrics     http.Handler
	MetricsPath string
}

// Module is the system feature.
type Module struct {
	handler *systemhttp.Handler
}

var _ httpserver.Module = (*Module)(nil)

// New wires system use cases and HTTP adapter.
func New(deps Dependencies) *Module {
	return &Module{handler: systemhttp.NewHandler(systemhttp.Config{
		Logger:           deps.Logger,
		Readiness:        application.NewReadiness(deps.HealthCheckers...),
		ReadinessTimeout: deps.ReadinessTimeout,
		Metrics:          deps.Metrics,
		MetricsPath:      deps.MetricsPath,
	})}
}

// Name implements [httpserver.Module].
func (*Module) Name() string { return "system" }

// RegisterRoutes implements [httpserver.Module].
func (m *Module) RegisterRoutes(mux *http.ServeMux) error {
	m.handler.RegisterRoutes(mux)
	return nil
}
