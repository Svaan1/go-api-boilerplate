// Package app is the composition root: it builds platform resources, wires
// feature modules, and owns process resource lifecycle.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	systemapp "github.com/svaan1/go-api-boilerplate/internal/features/system/application"
	"github.com/svaan1/go-api-boilerplate/internal/platform/auth"
	"github.com/svaan1/go-api-boilerplate/internal/platform/config"
	"github.com/svaan1/go-api-boilerplate/internal/platform/database"
	"github.com/svaan1/go-api-boilerplate/internal/platform/httpserver"
	"github.com/svaan1/go-api-boilerplate/internal/platform/observability"
	"github.com/svaan1/go-api-boilerplate/internal/platform/ratelimit"
)

var _ systemapp.HealthChecker = database.HealthCheck{}

// App owns HTTP serving, the database pool, and their bounded shutdown.
type App struct {
	pool   *pgxpool.Pool
	server *httpserver.Server
	logger *slog.Logger
}

// New constructs process resources without starting background work.
func New(cfg config.Config, logger *slog.Logger) (*App, error) {
	verifier, err := auth.NewHMACVerifier(cfg.Auth)
	if err != nil {
		return nil, fmt.Errorf("constructing verifier: %w", err)
	}
	pool, err := database.Open(context.Background(), cfg.Database, logger)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	handler, err := newHandler(cfg, platform{
		logger:         logger,
		verifier:       verifier,
		limiter:        ratelimit.New(cfg.RateLimit, nil),
		metrics:        observability.New(),
		pool:           pool,
		healthCheckers: []systemapp.HealthChecker{database.NewHealthCheck(pool)},
	})
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("constructing router: %w", err)
	}
	return &App{pool: pool, server: httpserver.NewServer(cfg, handler), logger: logger}, nil
}

// Run serves until context cancellation or unexpected listener failure, then
// closes the database pool after HTTP has drained.
func (a *App) Run(ctx context.Context) error {
	defer a.pool.Close()
	err := a.server.Run(ctx)
	a.logger.Info("http stopped")
	return err
}

// platform carries process-owned resources that modules and the router share.
type platform struct {
	logger         *slog.Logger
	verifier       auth.Verifier
	limiter        ratelimit.Limiter
	metrics        *observability.Metrics
	pool           *pgxpool.Pool
	healthCheckers []systemapp.HealthChecker
}

func newHandler(cfg config.Config, p platform) (http.Handler, error) {
	return httpserver.NewRouter(cfg, httpserver.Dependencies{
		Logger: p.logger, Verifier: p.verifier, Limiter: p.limiter, Metrics: p.metrics,
	}, modules(cfg, p))
}
