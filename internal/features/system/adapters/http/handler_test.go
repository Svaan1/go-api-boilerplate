package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/svaan1/go-api-boilerplate/internal/features/system/application"
	"github.com/svaan1/go-api-boilerplate/internal/platform/auth"
)

type probe struct{ err error }

func (probe) Name() string                  { return "probe" }
func (p probe) Check(context.Context) error { return p.err }

func newMux(logger *slog.Logger, checkers ...application.HealthChecker) *http.ServeMux {
	mux := http.NewServeMux()
	NewHandler(Config{
		Logger:           logger,
		Readiness:        application.NewReadiness(checkers...),
		ReadinessTimeout: time.Second,
	}).RegisterRoutes(mux)
	return mux
}

func TestHealthReadiness(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		checkers   []application.HealthChecker
		want       int
	}{
		{name: "liveness without checkers", path: "/healthz", want: 200},
		{name: "readiness without checkers", path: "/readyz", want: 503},
		{name: "readiness dependency down", path: "/readyz", checkers: []application.HealthChecker{probe{err: errors.New("connection refused")}}, want: 503},
		{name: "readiness second dependency down", path: "/readyz", checkers: []application.HealthChecker{probe{}, probe{err: errors.New("connection refused")}}, want: 503},
		{name: "readiness dependency healthy", path: "/readyz", checkers: []application.HealthChecker{probe{}}, want: 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux := newMux(slog.New(slog.NewTextHandler(io.Discard, nil)), tc.checkers...)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
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
	mux := newMux(slog.New(slog.NewTextHandler(&logs, nil)), probe{err: errors.New(connectionURL)})
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusServiceUnavailable ||
		strings.Contains(logs.String(), connectionURL) || strings.Contains(response.Body.String(), connectionURL) {
		t.Fatalf("readiness exposed credentials: status=%d body=%s logs=%s",
			response.Code, response.Body.String(), logs.String())
	}
	if !strings.Contains(logs.String(), "check=probe") {
		t.Fatalf("readiness log does not name failed check: %s", logs.String())
	}
}

type testVerifier struct{ identity auth.Identity }

func (v testVerifier) Verify(context.Context, string) (auth.Identity, error) {
	return v.identity, nil
}

func TestMeRequiresAuthenticationAndReturnsIdentity(t *testing.T) {
	identity := auth.Identity{
		Subject:     "subject-1",
		Roles:       []string{"admin"},
		Permissions: []string{"profile:read"},
	}
	handler := auth.Authenticate(testVerifier{identity: identity})(newMux(slog.New(slog.NewTextHandler(io.Discard, nil))))

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/v1/me", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want %d", unauthorized.Code, http.StatusUnauthorized)
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	request.Header.Set("Authorization", "Bearer valid")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("authenticated status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding identity: %v", err)
	}
	want := map[string]any{
		"subject":     "subject-1",
		"roles":       []any{"admin"},
		"permissions": []any{"profile:read"},
	}
	if !reflect.DeepEqual(body, want) {
		t.Fatalf("identity JSON = %#v, want %#v", body, want)
	}
}

func TestMetricsRouteRegisteredOnlyWhenHandlerProvided(t *testing.T) {
	metrics := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	for _, tc := range []struct {
		name    string
		handler http.Handler
		want    int
	}{
		{name: "enabled", handler: metrics, want: http.StatusTeapot},
		{name: "disabled", want: http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			NewHandler(Config{Readiness: application.NewReadiness(), Metrics: tc.handler, MetricsPath: "/internal/metrics"}).RegisterRoutes(mux)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/internal/metrics", nil))
			if w.Code != tc.want {
				t.Fatalf("status %d want %d", w.Code, tc.want)
			}
		})
	}
}
