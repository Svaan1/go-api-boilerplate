package app

import (
	"net/http"

	"github.com/svaan1/go-api-boilerplate/internal/features/example"
	examplepostgres "github.com/svaan1/go-api-boilerplate/internal/features/example/adapters/postgres"
	exampleapp "github.com/svaan1/go-api-boilerplate/internal/features/example/application"
	"github.com/svaan1/go-api-boilerplate/internal/features/system"
	"github.com/svaan1/go-api-boilerplate/internal/platform/clock"
	"github.com/svaan1/go-api-boilerplate/internal/platform/config"
	"github.com/svaan1/go-api-boilerplate/internal/platform/database"
	"github.com/svaan1/go-api-boilerplate/internal/platform/httpserver"
	"github.com/svaan1/go-api-boilerplate/internal/platform/idgen"
)

// Platform services satisfy feature ports structurally; assert it here, where
// both sides are visible.
var (
	_ exampleapp.Clock       = clock.System{}
	_ exampleapp.IDGenerator = idgen.UUIDv7{}
)

// modules lists every feature served by the API. Adding a feature means
// adding one constructor call here.
func modules(cfg config.Config, p platform) []httpserver.Module {
	return []httpserver.Module{
		system.New(system.Dependencies{
			Logger:           p.logger,
			HealthCheckers:   p.healthCheckers,
			ReadinessTimeout: cfg.Shutdown.ReadinessTimeout,
			Metrics:          metricsHandler(cfg, p),
			MetricsPath:      cfg.Observability.MetricsPath,
		}),
		example.New(example.Dependencies{
			Logger:     p.logger,
			Repository: examplepostgres.NewRepository(p.pool),
			Transactor: database.NewTxManager(p.pool),
			Clock:      clock.System{},
			IDs:        idgen.UUIDv7{},
		}),
	}
}

func metricsHandler(cfg config.Config, p platform) http.Handler {
	if !cfg.Observability.Metrics || p.metrics == nil {
		return nil
	}
	return p.metrics.Handler()
}
