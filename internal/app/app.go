// Package app owns application composition and resource lifecycle.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/svaan1/go-api-boilerplate/internal/auth"
	"github.com/svaan1/go-api-boilerplate/internal/config"
	"github.com/svaan1/go-api-boilerplate/internal/database"
	"github.com/svaan1/go-api-boilerplate/internal/httpapi"
	"github.com/svaan1/go-api-boilerplate/internal/observability"
	"github.com/svaan1/go-api-boilerplate/internal/ratelimit"
)

// App owns HTTP serving and its bounded shutdown.
type App struct {
	pool            *pgxpool.Pool
	server          *http.Server
	shutdownTimeout time.Duration
	logger          *slog.Logger
}

// New constructs process resources without starting background work.
func New(cfg config.Config, logger *slog.Logger) (*App, error) {
	verifier, err := auth.NewHMACVerifier(cfg.Auth)
	if err != nil {
		return nil, fmt.Errorf("constructing verifier: %w", err)
	}
	limiter := ratelimit.New(cfg.RateLimit, nil)
	pool, err := database.Open(context.Background(), cfg.Database, logger)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	handler, err := httpapi.NewRouter(cfg, httpapi.Dependencies{
		Logger: logger, Database: pool, Pool: pool, Verifier: verifier,
		Limiter: limiter, Metrics: observability.New(),
	})
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("constructing router: %w", err)
	}
	return &App{
		server: &http.Server{
			Addr: cfg.HTTP.Address, Handler: handler,
			ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
			ReadTimeout:       cfg.HTTP.ReadTimeout, WriteTimeout: cfg.HTTP.WriteTimeout,
			IdleTimeout: cfg.HTTP.IdleTimeout, MaxHeaderBytes: cfg.HTTP.MaxHeaderBytes,
		},
		shutdownTimeout: cfg.Shutdown.Timeout,
		logger:          logger,
	}, nil
}

// Run serves until context cancellation or unexpected listener failure.
func (a *App) Run(ctx context.Context) error {
	defer a.pool.Close()
	listener, err := net.Listen("tcp", a.server.Addr)
	if err != nil {
		return fmt.Errorf("listening: %w", err)
	}
	served := make(chan error, 1)
	go func() { served <- a.server.Serve(listener) }()
	var runErr error
	isServed := false
	select {
	case err := <-served:
		isServed = true
		if !errors.Is(err, http.ErrServerClosed) {
			runErr = fmt.Errorf("serving http: %w", err)
		}
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout)
	defer cancel()
	if err := a.server.Shutdown(shutdownCtx); err != nil {
		runErr = errors.Join(runErr, fmt.Errorf("shutting down http: %w", err))
		if closeErr := a.server.Close(); closeErr != nil {
			runErr = errors.Join(runErr, fmt.Errorf("closing http: %w", closeErr))
		}
	}
	if !isServed {
		if err := <-served; err != nil && !errors.Is(err, http.ErrServerClosed) {
			runErr = errors.Join(runErr, fmt.Errorf("serving http: %w", err))
		}
	}
	a.logger.Info("http stopped")
	return runErr
}
