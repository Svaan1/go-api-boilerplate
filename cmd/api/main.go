// Command api starts the HTTP API and owns process signals.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/svaan1/go-api-boilerplate/internal/app"
	"github.com/svaan1/go-api-boilerplate/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("api startup failed", "error", err)
		os.Exit(1)
	}
	logger := cfg.Logger()
	if err := run(cfg, logger); err != nil {
		logger.Error("api stopped", "error", err)
		os.Exit(1)
	}
}

func run(cfg config.Config, logger *slog.Logger) error {
	application, err := app.New(cfg, logger)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return application.Run(ctx)
}
