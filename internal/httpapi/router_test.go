package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/svaan1/go-api-boilerplate/internal/config"
	"github.com/svaan1/go-api-boilerplate/internal/ratelimit"
)

type probe struct{ err error }

func (p probe) Ping(context.Context) error { return p.err }

func TestHealthReadiness(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		p          Pinger
		want       int
	}{
		{name: "liveness without database", path: "/healthz", want: 200},
		{name: "readiness without database", path: "/readyz", want: 503},
		{name: "readiness database down", path: "/readyz", p: probe{err: errors.New("connection refused")}, want: 503},
		{name: "readiness database healthy", path: "/readyz", p: probe{}, want: 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := ratelimit.New(config.RateLimit{RequestsPerSecond: 10, Burst: 20, MaxClients: 100, IdleTTL: time.Minute, CleanupInterval: time.Minute}, nil)
			router, err := NewRouter(config.Config{Shutdown: config.Shutdown{ReadinessTimeout: time.Second}}, Dependencies{
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Database: tc.p, Limiter: limits,
			})
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if w.Code != tc.want {
				t.Fatalf("status %d want %d", w.Code, tc.want)
			}
			if !strings.Contains(w.Body.String(), `"timestamp"`) || strings.Contains(w.Body.String(), "connection refused") {
				t.Fatalf("unsafe health response: %s", w.Body.String())
			}
		})
	}
}

func TestReadinessDoesNotLogDatabaseCredentials(t *testing.T) {
	var logs bytes.Buffer
	connectionURL := "postgres://user:private@127.0.0.1:5432/api"
	limits := ratelimit.New(config.RateLimit{
		RequestsPerSecond: 10, Burst: 2, MaxClients: 2,
		IdleTTL: time.Minute, CleanupInterval: time.Minute,
	}, nil)
	router, err := NewRouter(config.Config{Shutdown: config.Shutdown{ReadinessTimeout: time.Second}},
		Dependencies{
			Logger:   slog.New(slog.NewTextHandler(&logs, nil)),
			Database: probe{err: errors.New(connectionURL)}, Limiter: limits,
		})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusServiceUnavailable ||
		strings.Contains(logs.String(), connectionURL) || strings.Contains(response.Body.String(), connectionURL) {
		t.Fatalf("readiness exposed credentials: status=%d body=%s logs=%s",
			response.Code, response.Body.String(), logs.String())
	}
}
