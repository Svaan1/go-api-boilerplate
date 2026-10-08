package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/svaan1/go-api-boilerplate/internal/platform/config"
)

// Server serves one handler with configured limits and bounded shutdown.
type Server struct {
	server          *http.Server
	shutdownTimeout time.Duration
}

// NewServer applies HTTP limits and shutdown timeout without listening.
func NewServer(cfg config.Config, handler http.Handler) *Server {
	return &Server{
		server: &http.Server{
			Addr: cfg.HTTP.Address, Handler: handler,
			ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
			ReadTimeout:       cfg.HTTP.ReadTimeout, WriteTimeout: cfg.HTTP.WriteTimeout,
			IdleTimeout: cfg.HTTP.IdleTimeout, MaxHeaderBytes: cfg.HTTP.MaxHeaderBytes,
		},
		shutdownTimeout: cfg.Shutdown.Timeout,
	}
}

// Run serves until context cancellation or unexpected listener failure, then
// drains connections within the shutdown timeout.
func (s *Server) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return fmt.Errorf("listening: %w", err)
	}
	served := make(chan error, 1)
	go func() { served <- s.server.Serve(listener) }()
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
	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()
	if err := s.server.Shutdown(shutdownCtx); err != nil {
		runErr = errors.Join(runErr, fmt.Errorf("shutting down http: %w", err))
		if closeErr := s.server.Close(); closeErr != nil {
			runErr = errors.Join(runErr, fmt.Errorf("closing http: %w", closeErr))
		}
	}
	if !isServed {
		if err := <-served; err != nil && !errors.Is(err, http.ErrServerClosed) {
			runErr = errors.Join(runErr, fmt.Errorf("serving http: %w", err))
		}
	}
	return runErr
}
