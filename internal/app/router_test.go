package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	systemapp "github.com/svaan1/go-api-boilerplate/internal/features/system/application"
	"github.com/svaan1/go-api-boilerplate/internal/platform/auth"
	"github.com/svaan1/go-api-boilerplate/internal/platform/config"
	"github.com/svaan1/go-api-boilerplate/internal/platform/observability"
	"github.com/svaan1/go-api-boilerplate/internal/platform/ratelimit"
)

type probe struct{ err error }

func (probe) Name() string                  { return "probe" }
func (p probe) Check(context.Context) error { return p.err }

func ready() []systemapp.HealthChecker { return []systemapp.HealthChecker{probe{}} }

const (
	stackSecret   = "01234567890123456789012345678901"
	stackIssuer   = "router-stack-tests"
	stackAudience = "router-stack-client"
)

func stackConfig() config.Config {
	return config.Config{
		HTTP:          config.HTTP{MaxBodyBytes: 32},
		Auth:          config.Auth{HMACSecret: stackSecret, Issuer: stackIssuer, Audience: stackAudience, Algorithm: "HS256"},
		CORS:          config.CORS{Origins: []string{"https://client.example"}, Methods: []string{"GET", "POST"}, Headers: []string{"Authorization", "Content-Type"}},
		Observability: config.Observability{Metrics: true, MetricsPath: "/metrics"},
		Shutdown:      config.Shutdown{ReadinessTimeout: time.Second},
	}
}

func stackLogger(output io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func signedIdentity(t *testing.T, roles, permissions []string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"iss": stackIssuer, "aud": stackAudience, "sub": "subject-private-42",
		"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
		"roles": roles, "permissions": permissions,
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(stackSecret))
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func newStackRouter(t *testing.T, cfg config.Config, checkers []systemapp.HealthChecker, limiter ratelimit.Limiter, metrics *observability.Metrics, logOutput io.Writer) http.Handler {
	t.Helper()
	if limiter == nil {
		limiter = ratelimit.New(config.RateLimit{RequestsPerSecond: 100, Burst: 100, MaxClients: 100, IdleTTL: time.Minute, CleanupInterval: time.Minute}, nil)
	}
	verifier, err := auth.NewHMACVerifier(cfg.Auth)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newHandler(cfg, platform{
		logger: stackLogger(logOutput), verifier: verifier,
		limiter: limiter, metrics: metrics, healthCheckers: checkers,
	})
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func perform(handler http.Handler, method, target, remote string, headers http.Header, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.RemoteAddr = remote
	request.Header = headers.Clone()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestRouterStackHealthReadinessAndIdentity(t *testing.T) {
	for _, test := range []struct {
		name     string
		checkers []systemapp.HealthChecker
		path     string
		status   int
	}{
		{name: "health stays live", path: "/healthz", status: http.StatusOK},
		{name: "readiness reports missing database", path: "/readyz", status: http.StatusServiceUnavailable},
		{name: "readiness hides database failure", checkers: []systemapp.HealthChecker{probe{err: errors.New("private database detail")}}, path: "/readyz", status: http.StatusServiceUnavailable},
		{name: "readiness succeeds", checkers: ready(), path: "/readyz", status: http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			router := newStackRouter(t, stackConfig(), test.checkers, nil, nil, &logs)
			recorder := perform(router, http.MethodGet, test.path, "192.0.2.9:4321", make(http.Header), "")
			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, test.status, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), `"timestamp"`) || strings.Contains(recorder.Body.String(), "private database detail") {
				t.Fatalf("unexpected health response: %s", recorder.Body.String())
			}
		})
	}

	router := newStackRouter(t, stackConfig(), ready(), nil, nil, io.Discard)
	anonymous := perform(router, http.MethodGet, "/v1/me", "192.0.2.10:4321", make(http.Header), "")
	if anonymous.Code != http.StatusUnauthorized || anonymous.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Fatalf("anonymous /v1/me = %d, WWW-Authenticate=%q", anonymous.Code, anonymous.Header().Get("WWW-Authenticate"))
	}

	token := signedIdentity(t, []string{"reader"}, []string{"profile:read"})
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+token)
	response := perform(router, http.MethodGet, "/v1/me", "192.0.2.11:4321", headers, "")
	if response.Code != http.StatusOK {
		t.Fatalf("authenticated /v1/me = %d; body=%s", response.Code, response.Body.String())
	}
	var identity map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &identity); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"subject": "subject-private-42", "roles": []any{"reader"}, "permissions": []any{"profile:read"}}
	if !equalJSON(identity, want) {
		t.Fatalf("identity = %#v, want %#v", identity, want)
	}
}

func equalJSON(got, want map[string]any) bool {
	gotBytes, _ := json.Marshal(got)
	wantBytes, _ := json.Marshal(want)
	return bytes.Equal(gotBytes, wantBytes)
}

func TestRouterStackRequestIDAndCORS(t *testing.T) {
	router := newStackRouter(t, stackConfig(), ready(), nil, nil, io.Discard)
	for _, test := range []struct {
		name string
		id   string
		want string
	}{
		{name: "valid ID preserved", id: "trace-123", want: "trace-123"},
		{name: "control character replaced", id: "bad\nid", want: "generated"},
	} {
		t.Run(test.name, func(t *testing.T) {
			headers := make(http.Header)
			headers.Set("X-Request-ID", test.id)
			response := perform(router, http.MethodGet, "/healthz", "192.0.2.12:4321", headers, "")
			got := response.Header().Get("X-Request-ID")
			if test.want == "generated" {
				if got == "" || got == test.id || len(got) != 32 {
					t.Fatalf("invalid request ID was not replaced: %q", got)
				}
			} else if got != test.want {
				t.Fatalf("request ID = %q, want %q", got, test.want)
			}
		})
	}

	headers := make(http.Header)
	headers.Set("Origin", "https://client.example")
	headers.Set("Access-Control-Request-Method", "POST")
	headers.Set("Access-Control-Request-Headers", "Authorization, Content-Type")
	preflight := perform(router, http.MethodOptions, "/v1/me", "192.0.2.13:4321", headers, "")
	if preflight.Code != http.StatusNoContent || preflight.Header().Get("Access-Control-Allow-Origin") != "https://client.example" || preflight.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Fatalf("preflight response = %d headers=%v body=%s", preflight.Code, preflight.Header(), preflight.Body.String())
	}
	if !strings.Contains(strings.Join(preflight.Header().Values("Vary"), ","), "Origin") {
		t.Fatalf("preflight missing Vary: Origin: %v", preflight.Header())
	}
}

type recordedLimiter struct {
	clients []string
	calls   int
}

func (l *recordedLimiter) Allow(_ context.Context, client string) (ratelimit.Decision, error) {
	l.clients = append(l.clients, client)
	l.calls++
	return ratelimit.Decision{Allowed: l.calls == 1, Limit: 1, Remaining: 0, RetryAfter: time.Second}, nil
}

func TestRouterStackRateLimitIgnoresUntrustedForwardedAddress(t *testing.T) {
	limiter := &recordedLimiter{}
	router := newStackRouter(t, stackConfig(), ready(), limiter, nil, io.Discard)
	for i := 0; i < 2; i++ {
		headers := make(http.Header)
		headers.Set("X-Forwarded-For", "198.51.100.200")
		response := perform(router, http.MethodGet, "/healthz", "203.0.113.8:1234", headers, "")
		if i == 1 {
			if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") != "1" || response.Header().Get("RateLimit-Limit") != "1" || response.Header().Get("RateLimit-Remaining") != "0" || response.Header().Get("RateLimit-Reset") != "1" {
				t.Fatalf("rate-limit response = %d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
			}
		}
	}
	if len(limiter.clients) != 2 || limiter.clients[0] != "203.0.113.8" || limiter.clients[1] != "203.0.113.8" {
		t.Fatalf("limiter client identities = %v; untrusted X-Forwarded-For must be ignored", limiter.clients)
	}
}

func TestRouterStackMetricsUseNormalizedRoutes(t *testing.T) {
	metrics := observability.New()
	router := newStackRouter(t, stackConfig(), ready(), nil, metrics, io.Discard)
	token := signedIdentity(t, []string{"member"}, []string{"profile:read"})
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+token)
	response := perform(router, http.MethodGet, "/v1/me?private_query=do-not-export", "198.51.100.77:5432", headers, "")
	if response.Code != http.StatusOK {
		t.Fatalf("GET /v1/me = %d; body=%s", response.Code, response.Body.String())
	}
	scrape := perform(router, http.MethodGet, "/metrics", "198.51.100.77:5432", make(http.Header), "")
	body := scrape.Body.String()
	if !strings.Contains(body, `route="GET /v1/me"`) || !strings.Contains(body, `status="200"`) {
		t.Fatalf("metrics lack normalized /v1/me request label: %s", body)
	}
	for _, private := range []string{"private_query", "do-not-export", "198.51.100.77", "subject-private-42", token} {
		if strings.Contains(body, private) {
			t.Fatalf("metrics expose private value %q", private)
		}
	}
}
