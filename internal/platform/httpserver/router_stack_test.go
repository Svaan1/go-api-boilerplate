package httpserver

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/svaan1/go-api-boilerplate/internal/platform/auth"
	"github.com/svaan1/go-api-boilerplate/internal/platform/config"
	"github.com/svaan1/go-api-boilerplate/internal/platform/httpx"
	"github.com/svaan1/go-api-boilerplate/internal/platform/ratelimit"
)

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

func perform(handler http.Handler, method, target, remote string, headers http.Header, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.RemoteAddr = remote
	request.Header = headers.Clone()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestRouterStackFixtureGuardsBodyLimitAndPanicRecovery(t *testing.T) {
	var logs bytes.Buffer
	cfg := stackConfig()
	cfg.HTTP.MaxBodyBytes = 8
	mux := http.NewServeMux()
	mux.Handle("GET /fixture/role", auth.RequireRole("admin", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })))
	mux.Handle("GET /fixture/permission", auth.RequirePermission("write:secret", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })))
	mux.HandleFunc("POST /fixture/decode", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Value string `json:"value"`
		}
		if err := httpx.DecodeJSON(w, r, &input); err != nil {
			var decodeError *httpx.DecodeError
			if errors.As(err, &decodeError) {
				httpx.WriteProblem(w, r, decodeError.Status, decodeError.Message, nil)
				return
			}
			httpx.WriteProblem(w, r, http.StatusBadRequest, "invalid request", nil)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /fixture/panic", func(http.ResponseWriter, *http.Request) { panic("private panic credential") })
	verifier, err := auth.NewHMACVerifier(cfg.Auth)
	if err != nil {
		t.Fatal(err)
	}
	handler := auth.Authenticate(verifier)(mux)
	handler = RateLimit(handler, ratelimit.New(config.RateLimit{RequestsPerSecond: 100, Burst: 100, MaxClients: 100, IdleTTL: time.Minute, CleanupInterval: time.Minute}, nil), nil)
	handler = ClientIdentity(cfg)(handler)
	handler = LimitBody(handler, cfg.HTTP.MaxBodyBytes)
	handler = CORS(cfg)(handler)
	handler = SecurityHeaders(cfg)(handler)
	handler = RecoverPanic(handler, stackLogger(&logs))
	handler = RequestLogging(handler, stackLogger(&logs))
	handler = WithRequestID(handler)
	stack := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pattern := mux.Handler(r)
		r.Pattern = pattern
		handler.ServeHTTP(w, r)
	})

	for _, test := range []struct {
		name   string
		path   string
		claims string
		status int
	}{
		{name: "role denied", path: "/fixture/role", claims: signedIdentity(t, []string{"member"}, []string{"write:secret"}), status: http.StatusForbidden},
		{name: "permission denied", path: "/fixture/permission", claims: signedIdentity(t, []string{"admin"}, []string{"profile:read"}), status: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			headers := make(http.Header)
			headers.Set("Authorization", "Bearer "+test.claims)
			response := perform(stack, http.MethodGet, test.path, "192.0.2.19:5432", headers, "")
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.status, response.Body.String())
			}
		})
	}

	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	oversized := perform(stack, http.MethodPost, "/fixture/decode", "192.0.2.20:5432", headers, `{"value":"payload-exceeds-limit"}`)
	if oversized.Code != http.StatusRequestEntityTooLarge || !strings.Contains(oversized.Body.String(), "request body is too large") {
		t.Fatalf("oversized JSON response = %d body=%s", oversized.Code, oversized.Body.String())
	}
	if strings.Contains(oversized.Body.String(), "payload-exceeds-limit") {
		t.Fatalf("oversized response leaked request body: %s", oversized.Body.String())
	}

	panicToken := signedIdentity(t, []string{"member"}, []string{"profile:read"})
	panicHeaders := make(http.Header)
	panicHeaders.Set("Authorization", "Bearer "+panicToken)
	panicResponse := perform(stack, http.MethodGet, "/fixture/panic", "192.0.2.21:5432", panicHeaders, "")
	if panicResponse.Code != http.StatusInternalServerError || strings.Contains(panicResponse.Body.String(), "private panic credential") {
		t.Fatalf("panic response not redacted: status=%d body=%s", panicResponse.Code, panicResponse.Body.String())
	}
	if !strings.Contains(logs.String(), "http panic") || !strings.Contains(logs.String(), "goroutine") || strings.Contains(logs.String(), "private panic credential") || strings.Contains(logs.String(), panicToken) || strings.Contains(logs.String(), "subject-private-42") {
		t.Fatalf("panic log missing stack or leaking sensitive data: %s", logs.String())
	}
}
